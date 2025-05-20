/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bsutil "sigs.k8s.io/cluster-api/bootstrap/util"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	kubeyaml "sigs.k8s.io/yaml"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
	ck8serrors "github.com/canonical/cluster-api-k8s/pkg/errors"
	"github.com/canonical/cluster-api-k8s/pkg/locking"
	"github.com/canonical/cluster-api-k8s/pkg/secret"
	"github.com/canonical/cluster-api-k8s/pkg/token"
)

// InitLocker is a lock that is used around control plane init.
type InitLocker interface {
	Lock(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) bool
	Unlock(ctx context.Context, cluster *clusterv1.Cluster) bool
}

// CK8sConfigReconciler reconciles a CK8sConfig object.
type CK8sConfigReconciler struct {
	client.Client
	Log          logr.Logger
	CK8sInitLock InitLocker
	Scheme       *runtime.Scheme

	K8sdDialTimeout   time.Duration
	managementCluster ck8s.ManagementCluster
}

type Scope struct {
	logr.Logger
	Config      *bootstrapv1.CK8sConfig
	ConfigOwner *bsutil.ConfigOwner
	Cluster     *clusterv1.Cluster
}

var (
	ErrInvalidRef   = errors.New("invalid reference")
	ErrFailedUnlock = errors.New("failed to unlock the init lock")
)

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=ck8sconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=ck8sconfigs/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=exp.cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *CK8sConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ reconcile.Result, rerr error) {
	log := r.Log.WithValues("ck8sconfig", req.NamespacedName)

	// Lookup the ck8s config
	config := &bootstrapv1.CK8sConfig{}
	if err := r.Client.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get config")
		return ctrl.Result{}, err
	}

	// Look up the owner of this KubeConfig if there is one
	configOwner, err := bsutil.GetConfigOwner(ctx, r.Client, config)
	if apierrors.IsNotFound(err) {
		// Could not find the owner yet, this is not an error and will rereconcile when the owner gets set.
		return ctrl.Result{}, nil
	}
	if err != nil {
		log.Error(err, "Failed to get owner")
		return ctrl.Result{}, err
	}
	if configOwner == nil {
		return ctrl.Result{}, nil
	}

	log = log.WithValues("kind", configOwner.GetKind(), "version", configOwner.GetResourceVersion(), "name", configOwner.GetName())

	// Lookup the cluster the config owner is associated with
	cluster, err := util.GetClusterByName(ctx, r.Client, configOwner.GetNamespace(), configOwner.ClusterName())
	if err != nil {
		if errors.Is(err, util.ErrNoCluster) {
			log.Info(fmt.Sprintf("%s does not belong to a cluster yet, waiting until it's part of a cluster", configOwner.GetKind()))
			return ctrl.Result{}, nil
		}

		if apierrors.IsNotFound(err) {
			log.Info("Cluster does not exist yet, waiting until it is created")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Could not get cluster with metadata")
		return ctrl.Result{}, err
	}

	if annotations.IsPaused(cluster, config) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	scope := &Scope{
		Logger:      log,
		Config:      config,
		ConfigOwner: configOwner,
		Cluster:     cluster,
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(config, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Attempt to Patch the CK8sConfig object and status after each reconciliation if no error occurs.
	defer func() {
		// always update the readyCondition; the summary is represented using the "1 of x completed" notation.

		conditions.SetSummary(config,
			conditions.WithConditions(
				bootstrapv1.DataSecretAvailableCondition,
				bootstrapv1.CertificatesAvailableCondition,
			),
		)

		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if rerr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, config, patchOpts...); err != nil {
			log.Error(err, "Failed to patch config", "rerr", rerr)
			if rerr == nil {
				rerr = err
			}
		}
	}()

	switch {
	// Wait for the infrastructure to be ready.
	case !cluster.Status.InfrastructureReady:
		log.Info("Cluster infrastructure is not ready, waiting")
		conditions.MarkFalse(config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	// Reconcile status for machines that already have a secret reference, but our status isn't up to date.
	// This case solves the pivoting scenario (or a backup restore) which doesn't preserve the status subresource on objects.
	case configOwner.DataSecretName() != nil && (!config.Status.Ready || config.Status.DataSecretName == nil):
		config.Status.Ready = true
		config.Status.DataSecretName = configOwner.DataSecretName()
		conditions.MarkTrue(config, bootstrapv1.DataSecretAvailableCondition)
		return ctrl.Result{}, nil
	// Status is ready means a config has been generated.
	case config.Status.Ready:
		// In any other case just return as the config is already generated and need not be generated again.
		return ctrl.Result{}, nil
	}

	// Note: can't use IsFalse here because we need to handle the absence of the condition as well as false.
	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		return r.handleClusterNotInitialized(ctx, scope)
	}

	// Every other case it's a join scenario
	// Nb. in this case ClusterConfiguration and InitConfiguration should not be defined by users, but in case of misconfigurations, CABPK simply ignore them

	// Unlock any locks that might have been set during init process
	r.CK8sInitLock.Unlock(ctx, cluster)

	// it's a control plane join
	if configOwner.IsControlPlaneMachine() {
		if err := r.joinControlplane(ctx, scope); err != nil {
			log.Error(err, "Encountered error during joinControlplane")
			return ck8serrors.RequeueOnK8sdProxyError(err)
		}
		return reconcile.Result{}, nil
	}

	// It's a worker join
	if err := r.joinWorker(ctx, scope); err != nil {
		log.Error(err, "Encountered error during joinWorker")
		return ck8serrors.RequeueOnK8sdProxyError(err)
	}
	return reconcile.Result{}, nil
}

func (r *CK8sConfigReconciler) joinControlplane(ctx context.Context, scope *Scope) error {
	machine := &clusterv1.Machine{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(scope.ConfigOwner.Object, machine); err != nil {
		return fmt.Errorf("cannot convert %s to Machine: %w", scope.ConfigOwner.GetKind(), err)
	}

	// injects into config.Version values from top level object
	r.reconcileTopLevelObjectSettings(scope.Cluster, machine, scope.Config)

	nodeToken, err := token.GenerateAndStoreNodeToken(ctx, r.Client, client.ObjectKeyFromObject(scope.Cluster), machine.Name)
	if err != nil {
		return fmt.Errorf("failed to generate node token: %w", err)
	}

	microclusterPort := scope.Config.Spec.ControlPlaneConfig.GetMicroclusterPort()
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(scope.Cluster), microclusterPort)
	if err != nil {
		return fmt.Errorf("failed to create remote cluster client: %w", err)
	}

	joinToken, err := workloadCluster.NewControlPlaneJoinToken(ctx, scope.Config.Name)
	if err != nil {
		return fmt.Errorf("failed to request join token: %w", err)
	}

	controlPlaneConfig := scope.Config.Spec.ControlPlaneConfig
	// Adding the join token name to the extra SANs is required because the token name
	// and kubelet name diverge in the CAPI context.
	// See https://github.com/canonical/k8s-snap/pull/629 for more details.
	controlPlaneConfig.ExtraSANs = append(controlPlaneConfig.ExtraSANs, scope.Config.Name)

	configStruct := ck8s.GenerateJoinControlPlaneConfig(ck8s.JoinControlPlaneConfig{
		ControlPlaneEndpoint: scope.Cluster.Spec.ControlPlaneEndpoint.Host,
		ControlPlaneConfig:   controlPlaneConfig,

		ExtraKubeProxyArgs:  scope.Config.Spec.ExtraKubeProxyArgs,
		ExtraKubeletArgs:    scope.Config.Spec.ExtraKubeletArgs,
		ExtraContainerdArgs: scope.Config.Spec.ExtraContainerdArgs,
	})
	joinConfig, err := kubeyaml.Marshal(configStruct)
	if err != nil {
		return err
	}

	files, err := r.resolveFiles(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return err
	}

	snapInstallData, err := r.getSnapInstallDataFromSpec(scope.Config.Spec)
	if err != nil {
		return fmt.Errorf("failed to get snap install data from spec: %w", err)
	}

	// If the machine has an in-place upgrade annotation, use it to set the snap install data
	inPlaceInstallData := r.resolveInPlaceUpgradeRelease(machine)
	if inPlaceInstallData != nil {
		snapInstallData = inPlaceInstallData
	}

	// log snapinstalldata
	scope.Info("SnapInstallData Spec", "Option", scope.Config.Spec.Channel, "Value", scope.Config.Spec.Revision, "LocalPath", scope.Config.Spec.LocalPath)
	scope.Info("SnapInstallData", "Option", snapInstallData.Option, "Value", snapInstallData.Value)

	input := cloudinit.JoinControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			BootCommands:         scope.Config.Spec.BootCommands,
			PreRunCommands:       scope.Config.Spec.PreRunCommands,
			PostRunCommands:      scope.Config.Spec.PostRunCommands,
			AdditionalUserData:   scope.Config.Spec.AdditionalUserData,
			KubernetesVersion:    scope.Config.Spec.Version,
			SnapInstallData:      snapInstallData,
			ExtraFiles:           cloudinit.FilesFromAPI(files),
			ConfigFileContents:   string(joinConfig),
			MicroclusterAddress:  scope.Config.Spec.ControlPlaneConfig.MicroclusterAddress,
			MicroclusterPort:     microclusterPort,
			HTTPProxy:            scope.Config.Spec.HTTPProxy,
			HTTPSProxy:           scope.Config.Spec.HTTPSProxy,
			NoProxy:              scope.Config.Spec.NoProxy,
			AirGapped:            scope.Config.Spec.AirGapped,
			SnapstoreProxyScheme: scope.Config.Spec.SnapstoreProxyScheme,
			SnapstoreProxyDomain: scope.Config.Spec.SnapstoreProxyDomain,
			SnapstoreProxyID:     scope.Config.Spec.SnapstoreProxyID,
			NodeName:             scope.Config.Spec.NodeName,
			NodeToken:            *nodeToken,
		},
		JoinToken: joinToken,
	}
	cloudConfig, err := cloudinit.NewJoinControlPlane(input)
	if err != nil {
		return err
	}
	cloudInitData, err := cloudinit.GenerateCloudConfig(cloudConfig)
	if err != nil {
		return fmt.Errorf("failed to generate cloud-init: %w", err)
	}

	if err := r.storeBootstrapData(ctx, scope, cloudInitData); err != nil {
		scope.Error(err, "Failed to store bootstrap data")
		return err
	}
	return nil
}

func (r *CK8sConfigReconciler) joinWorker(ctx context.Context, scope *Scope) error {
	machine := &clusterv1.Machine{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(scope.ConfigOwner.Object, machine); err != nil {
		return fmt.Errorf("cannot convert %s to Machine: %w", scope.ConfigOwner.GetKind(), err)
	}

	// injects into config.Version values from top level object
	r.reconcileTopLevelObjectSettings(scope.Cluster, machine, scope.Config)

	authToken, err := token.Lookup(ctx, r.Client, client.ObjectKeyFromObject(scope.Cluster))
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return err
	}

	if authToken == nil {
		return fmt.Errorf("auth token not yet generated")
	}

	nodeToken, err := token.GenerateAndStoreNodeToken(ctx, r.Client, client.ObjectKeyFromObject(scope.Cluster), machine.Name)
	if err != nil {
		return fmt.Errorf("failed to generate node token: %w", err)
	}

	microclusterPort := scope.Config.Spec.ControlPlaneConfig.GetMicroclusterPort()
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(scope.Cluster), microclusterPort)
	if err != nil {
		return fmt.Errorf("failed to create remote cluster client: %w", err)
	}

	joinToken, err := workloadCluster.NewWorkerJoinToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to request join token: %w", err)
	}

	configStruct := ck8s.GenerateJoinWorkerConfig(ck8s.JoinWorkerConfig{
		ExtraKubeProxyArgs:         scope.Config.Spec.ExtraKubeProxyArgs,
		ExtraKubeletArgs:           scope.Config.Spec.ExtraKubeletArgs,
		ExtraContainerdArgs:        scope.Config.Spec.ExtraContainerdArgs,
		ExtraK8sAPIServerProxyArgs: scope.Config.Spec.ExtraK8sAPIServerProxyArgs,
	})
	joinConfig, err := kubeyaml.Marshal(configStruct)
	if err != nil {
		return err
	}

	files, err := r.resolveFiles(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return err
	}

	snapInstallData, err := r.getSnapInstallDataFromSpec(scope.Config.Spec)
	if err != nil {
		return fmt.Errorf("failed to get snap install data from spec: %w", err)
	}

	// If the machine has an in-place upgrade annotation, use it to set the snap install data
	inPlaceInstallData := r.resolveInPlaceUpgradeRelease(machine)
	if inPlaceInstallData != nil {
		snapInstallData = inPlaceInstallData
	}

	input := cloudinit.JoinWorkerInput{
		BaseUserData: cloudinit.BaseUserData{
			BootCommands:         scope.Config.Spec.BootCommands,
			PreRunCommands:       scope.Config.Spec.PreRunCommands,
			PostRunCommands:      scope.Config.Spec.PostRunCommands,
			AdditionalUserData:   scope.Config.Spec.AdditionalUserData,
			KubernetesVersion:    scope.Config.Spec.Version,
			SnapInstallData:      snapInstallData,
			ExtraFiles:           cloudinit.FilesFromAPI(files),
			ConfigFileContents:   string(joinConfig),
			MicroclusterAddress:  scope.Config.Spec.ControlPlaneConfig.MicroclusterAddress,
			MicroclusterPort:     microclusterPort,
			HTTPProxy:            scope.Config.Spec.HTTPProxy,
			HTTPSProxy:           scope.Config.Spec.HTTPSProxy,
			NoProxy:              scope.Config.Spec.NoProxy,
			AirGapped:            scope.Config.Spec.AirGapped,
			SnapstoreProxyScheme: scope.Config.Spec.SnapstoreProxyScheme,
			SnapstoreProxyDomain: scope.Config.Spec.SnapstoreProxyDomain,
			SnapstoreProxyID:     scope.Config.Spec.SnapstoreProxyID,
			NodeName:             scope.Config.Spec.NodeName,
			NodeToken:            *nodeToken,
		},
		JoinToken: joinToken,
	}
	cloudConfig, err := cloudinit.NewJoinWorker(input)
	if err != nil {
		return err
	}
	cloudInitData, err := cloudinit.GenerateCloudConfig(cloudConfig)
	if err != nil {
		return fmt.Errorf("failed to generate cloud-init: %w", err)
	}

	if err := r.storeBootstrapData(ctx, scope, cloudInitData); err != nil {
		scope.Error(err, "Failed to store bootstrap data")
		return err
	}

	return nil
}

// resolveUserBootstrapConfig returns the bootstrap configuration provided by the user.
// It can resolve string content, a reference to a secret, or an empty string if no configuration was provided.
func (r *CK8sConfigReconciler) resolveUserBootstrapConfig(ctx context.Context, cfg *bootstrapv1.CK8sConfig) (string, error) {
	// User did not provide a bootstrap configuration
	if cfg.Spec.BootstrapConfig == nil {
		return "", nil
	}

	// User provided a bootstrap configuration through content
	if cfg.Spec.BootstrapConfig.Content != "" {
		return cfg.Spec.BootstrapConfig.Content, nil
	}

	// User referenced a secret for the bootstrap configuration
	if cfg.Spec.BootstrapConfig.ContentFrom == nil {
		return "", nil
	}

	data, err := r.resolveSecretFileContent(ctx, cfg.Namespace, *cfg.Spec.BootstrapConfig.ContentFrom)
	if err != nil {
		return "", fmt.Errorf("failed to read bootstrap configuration from secret %q: %w", cfg.Spec.BootstrapConfig.ContentFrom.Secret.Name, err)
	}

	return string(data), nil
}

// resolveFiles maps .Spec.Files into cloudinit.Files, resolving any object references
// along the way.
func (r *CK8sConfigReconciler) resolveFiles(ctx context.Context, cfg *bootstrapv1.CK8sConfig) ([]bootstrapv1.File, error) {
	collected := make([]bootstrapv1.File, 0, len(cfg.Spec.Files))

	for i := range cfg.Spec.Files {
		in := cfg.Spec.Files[i]
		if in.ContentFrom != nil {
			data, err := r.resolveSecretFileContent(ctx, cfg.Namespace, *in.ContentFrom)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve file source: %w", err)
			}
			in.ContentFrom = nil
			in.Content = string(data)
		}
		collected = append(collected, in)
	}

	return collected, nil
}

func (r *CK8sConfigReconciler) resolveInPlaceUpgradeRelease(machine *clusterv1.Machine) *cloudinit.SnapInstallData {
	mAnnotations := machine.GetAnnotations()

	if mAnnotations == nil {
		return nil
	}

	val, ok := mAnnotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation]
	if !ok {
		return nil
	}

	optionKv := strings.Split(val, "=")

	if len(optionKv) != 2 {
		r.Log.Info("Invalid in-place upgrade release annotation, ignoring", "annotation", val)
		return nil
	}

	switch optionKv[0] {
	case "channel":
		return &cloudinit.SnapInstallData{
			Option: cloudinit.InstallOptionChannel,
			Value:  optionKv[1],
		}
	case "revision":
		return &cloudinit.SnapInstallData{
			Option: cloudinit.InstallOptionRevision,
			Value:  optionKv[1],
		}
	case "localPath":
		return &cloudinit.SnapInstallData{
			Option: cloudinit.InstallOptionLocalPath,
			Value:  optionKv[1],
		}
	default:
		r.Log.Info("Unknown in-place upgrade release option, ignoring", "option", optionKv[0])
	}

	return nil
}

func (r *CK8sConfigReconciler) getSnapInstallDataFromSpec(spec bootstrapv1.CK8sConfigSpec) (*cloudinit.SnapInstallData, error) {
	// Ensure that exactly one option is set
	count := 0
	if spec.Channel != "" {
		count++
	}
	if spec.Revision != "" {
		count++
	}
	if spec.LocalPath != "" {
		count++
	}
	if count > 1 {
		return nil, fmt.Errorf("only one of Channel, Revision, or LocalPath can be set, but multiple were provided")
	}

	switch {
	case spec.Channel != "":
		return &cloudinit.SnapInstallData{
			Option: cloudinit.InstallOptionChannel,
			Value:  spec.Channel,
		}, nil
	case spec.Revision != "":
		return &cloudinit.SnapInstallData{
			Option: cloudinit.InstallOptionRevision,
			Value:  spec.Revision,
		}, nil
	case spec.LocalPath != "":
		return &cloudinit.SnapInstallData{
			Option: cloudinit.InstallOptionLocalPath,
			Value:  spec.LocalPath,
		}, nil
	default:
		return &cloudinit.SnapInstallData{}, nil
	}
}

// resolveSecretFileContent returns file content fetched from a referenced secret object.
func (r *CK8sConfigReconciler) resolveSecretFileContent(ctx context.Context, ns string, source bootstrapv1.FileSource) ([]byte, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ns, Name: source.Secret.Name}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("secret not found %s: %w", key, err)
		}
		return nil, fmt.Errorf("failed to retrieve Secret %q: %w", key, err)
	}
	data, ok := secret.Data[source.Secret.Key]
	if !ok {
		return nil, fmt.Errorf("secret references non-existent secret key %q: %w", source.Secret.Key, ErrInvalidRef)
	}
	return data, nil
}

// resolveSecretFileContent returns file content fetched from a referenced secret object.
func (r *CK8sConfigReconciler) resolveSecretReference(ctx context.Context, ns string, secretRef bootstrapv1.SecretRef) ([]byte, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ns, Name: secretRef.Name}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("secret not found %s: %w", key, err)
		}
		return nil, fmt.Errorf("failed to retrieve Secret %q: %w", key, err)
	}
	data, ok := secret.Data[secretRef.Key]
	if !ok {
		return nil, fmt.Errorf("secret references non-existent secret key %q: %w", secretRef.Key, ErrInvalidRef)
	}
	return data, nil
}

func (r *CK8sConfigReconciler) handleClusterNotInitialized(ctx context.Context, scope *Scope) (_ ctrl.Result, reterr error) {
	// initialize the DataSecretAvailableCondition if missing.
	// this is required in order to avoid the condition's LastTransitionTime to flicker in case of errors surfacing
	// using the DataSecretGeneratedFailedReason
	if conditions.GetReason(scope.Config, bootstrapv1.DataSecretAvailableCondition) != bootstrapv1.DataSecretGenerationFailedReason {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, clusterv1.WaitingForControlPlaneAvailableReason, clusterv1.ConditionSeverityInfo, "")
	}

	// if it's NOT a control plane machine, requeue
	if !scope.ConfigOwner.IsControlPlaneMachine() {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	machine := &clusterv1.Machine{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(scope.ConfigOwner.Object, machine); err != nil {
		return ctrl.Result{}, fmt.Errorf("cannot convert %s to Machine: %w", scope.ConfigOwner.GetKind(), err)
	}

	// acquire the init lock so that only the first machine configured
	// as control plane get processed here
	// if not the first, requeue

	if !r.CK8sInitLock.Lock(ctx, scope.Cluster, machine) {
		scope.Info("A control plane is already being initialized, requeing until control plane is ready")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	defer func() {
		if reterr != nil {
			if !r.CK8sInitLock.Unlock(ctx, scope.Cluster) {
				reterr = kerrors.NewAggregate([]error{reterr, ErrFailedUnlock})
			}
		}
	}()

	scope.Info("Creating BootstrapData for the init control plane")

	// injects into config.ClusterConfiguration values from top level object
	r.reconcileTopLevelObjectSettings(scope.Cluster, machine, scope.Config)

	certificates := secret.NewCertificatesForInitialControlPlane(&scope.Config.Spec)
	err := certificates.LookupOrGenerate(
		ctx,
		r.Client,
		util.ObjectKey(scope.Cluster),
		*metav1.NewControllerRef(scope.Config, bootstrapv1.GroupVersion.WithKind("CK8sConfig")),
	)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.CertificatesAvailableCondition, bootstrapv1.CertificatesGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(scope.Config, bootstrapv1.CertificatesAvailableCondition)

	authToken, err := token.Lookup(ctx, r.Client, client.ObjectKeyFromObject(scope.Cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	nodeToken, err := token.GenerateAndStoreNodeToken(ctx, r.Client, client.ObjectKeyFromObject(scope.Cluster), machine.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to generate node token: %w", err)
	}

	clusterInitConfig := ck8s.InitControlPlaneConfig{
		ControlPlaneEndpoint:  scope.Cluster.Spec.ControlPlaneEndpoint.Host,
		ControlPlaneConfig:    scope.Config.Spec.ControlPlaneConfig,
		PopulatedCertificates: certificates,
		InitConfig:            scope.Config.Spec.InitConfig,

		ClusterNetwork: scope.Cluster.Spec.ClusterNetwork,

		ExtraKubeProxyArgs:         scope.Config.Spec.ExtraKubeProxyArgs,
		ExtraKubeletArgs:           scope.Config.Spec.ExtraKubeletArgs,
		ExtraContainerdArgs:        scope.Config.Spec.ExtraContainerdArgs,
		ExtraK8sAPIServerProxyArgs: scope.Config.Spec.ExtraK8sAPIServerProxyArgs,
	}

	if !scope.Config.Spec.IsEtcdManaged() {
		clusterInitConfig.DatastoreType = scope.Config.Spec.ControlPlaneConfig.DatastoreType

		datastoreServers, err := r.resolveSecretReference(ctx, scope.Config.Namespace, scope.Config.Spec.ControlPlaneConfig.DatastoreServersSecretRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		clusterInitConfig.DatastoreServers = strings.Split(string(datastoreServers), ",")
	}

	configStruct, err := ck8s.GenerateInitControlPlaneConfig(clusterInitConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	initConfig, err := kubeyaml.Marshal(configStruct)
	if err != nil {
		return ctrl.Result{}, err
	}

	files, err := r.resolveFiles(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return ctrl.Result{}, err
	}

	userSuppliedBootstrapConfig, err := r.resolveUserBootstrapConfig(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return ctrl.Result{}, err
	}

	microclusterPort := scope.Config.Spec.ControlPlaneConfig.GetMicroclusterPort()
	ds, err := ck8s.RenderK8sdProxyDaemonSetManifest(ck8s.K8sdProxyDaemonSetInput{K8sdPort: microclusterPort})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to render k8sd-proxy daemonset: %w", err)
	}

	snapInstallData, err := r.getSnapInstallDataFromSpec(scope.Config.Spec)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.SnapInstallDataValidatedCondition, bootstrapv1.SnapInstallValidationFailedReason, clusterv1.ConditionSeverityError, "%s", err.Error())
		return ctrl.Result{Requeue: true}, fmt.Errorf("failed to get snap install data from spec: %w", err)
	}

	cpinput := cloudinit.InitControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			BootCommands:         scope.Config.Spec.BootCommands,
			PreRunCommands:       scope.Config.Spec.PreRunCommands,
			PostRunCommands:      scope.Config.Spec.PostRunCommands,
			AdditionalUserData:   scope.Config.Spec.AdditionalUserData,
			KubernetesVersion:    scope.Config.Spec.Version,
			BootstrapConfig:      userSuppliedBootstrapConfig,
			SnapInstallData:      snapInstallData,
			ExtraFiles:           cloudinit.FilesFromAPI(files),
			ConfigFileContents:   string(initConfig),
			MicroclusterAddress:  scope.Config.Spec.ControlPlaneConfig.MicroclusterAddress,
			MicroclusterPort:     microclusterPort,
			NodeName:             scope.Config.Spec.NodeName,
			HTTPProxy:            scope.Config.Spec.HTTPProxy,
			HTTPSProxy:           scope.Config.Spec.HTTPSProxy,
			NoProxy:              scope.Config.Spec.NoProxy,
			AirGapped:            scope.Config.Spec.AirGapped,
			SnapstoreProxyScheme: scope.Config.Spec.SnapstoreProxyScheme,
			SnapstoreProxyDomain: scope.Config.Spec.SnapstoreProxyDomain,
			SnapstoreProxyID:     scope.Config.Spec.SnapstoreProxyID,
			NodeToken:            *nodeToken,
		},
		AuthToken:          *authToken,
		K8sdProxyDaemonSet: string(ds),
	}

	cloudConfig, err := cloudinit.NewInitControlPlane(cpinput)
	if err != nil {
		return ctrl.Result{}, err
	}

	cloudInitData, err := cloudinit.GenerateCloudConfig(cloudConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to generate cloud-init: %w", err)
	}

	if err := r.storeBootstrapData(ctx, scope, cloudInitData); err != nil {
		scope.Error(err, "Failed to store bootstrap data")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CK8sConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.CK8sInitLock == nil {
		r.CK8sInitLock = locking.NewControlPlaneInitMutex(mgr.GetClient())
	}

	if r.managementCluster == nil {
		r.managementCluster = &ck8s.Management{
			Client:          r.Client,
			K8sdDialTimeout: r.K8sdDialTimeout,
		}
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1.CK8sConfig{}).
		Complete(r)
}

// storeBootstrapData creates a new secret with the data passed in as input,
// sets the reference in the configuration status and ready to true.
func (r *CK8sConfigReconciler) storeBootstrapData(ctx context.Context, scope *Scope, data []byte) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.Config.Name,
			Namespace: scope.Config.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: scope.Cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "CK8sConfig",
					Name:       scope.Config.Name,
					UID:        scope.Config.UID,
					Controller: ptr.To[bool](true),
				},
			},
		},
		Data: map[string][]byte{
			"value": data,
		},
		Type: clusterv1.ClusterSecretType,
	}

	// as secret creation and scope.Config status patch are not atomic operations
	// it is possible that secret creation happens but the config.Status patches are not applied
	if err := r.Client.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create bootstrap data secret for CK8sConfig %s/%s: %w", scope.Config.Namespace, scope.Config.Name, err)
		}
		r.Log.Info("bootstrap data secret for CK8sConfig already exists, updating", "secret", secret.Name, "CK8sConfig", scope.Config.Name)
		if err := r.Client.Update(ctx, secret); err != nil {
			return fmt.Errorf("failed to update bootstrap data secret for CK8sConfig %s/%s: %w", scope.Config.Namespace, scope.Config.Name, err)
		}
	}

	scope.Config.Status.DataSecretName = ptr.To[string](secret.Name)
	scope.Config.Status.Ready = true
	conditions.MarkTrue(scope.Config, bootstrapv1.DataSecretAvailableCondition)
	return nil
}

func (r *CK8sConfigReconciler) reconcileTopLevelObjectSettings(_ *clusterv1.Cluster, machine *clusterv1.Machine, config *bootstrapv1.CK8sConfig) {
	log := r.Log.WithValues("ck8sconfig", fmt.Sprintf("%s/%s", config.Namespace, config.Name))

	// If there are no Version settings defined in Config, use Version from machine, if defined
	if config.Spec.Version == "" && machine.Spec.Version != nil {
		config.Spec.Version = *machine.Spec.Version
		log.Info("Altering Config", "Version", config.Spec.Version)
	}
}
