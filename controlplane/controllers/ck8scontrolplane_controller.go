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
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
	"github.com/canonical/cluster-api-k8s/pkg/kubeconfig"
	"github.com/canonical/cluster-api-k8s/pkg/secret"
	"github.com/canonical/cluster-api-k8s/pkg/token"
)

// CK8sControlPlaneReconciler reconciles a CK8sControlPlane object.
type CK8sControlPlaneReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	controller controller.Controller
	recorder   record.EventRecorder

	K8sdDialTimeout time.Duration

	managementCluster         ck8s.ManagementCluster
	managementClusterUncached ck8s.ManagementCluster
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

func (r *CK8sControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "ck8sControlPlane", req.Name)

	// Fetch the CK8sControlPlane instance.
	kcp := &controlplanev1.CK8sControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, kcp); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kcp.ObjectMeta)
	if err != nil {
		logger.Error(err, "Failed to retrieve owner Cluster from the API Server")
		return reconcile.Result{}, err
	}
	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{Requeue: true}, nil
	}
	logger = logger.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, kcp) {
		logger.Info("Reconciliation is paused for this object")
		return reconcile.Result{}, nil
	}

	// Wait for the cluster infrastructure to be ready before creating machines
	if !cluster.Status.InfrastructureReady {
		return reconcile.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(kcp, r.Client)
	if err != nil {
		logger.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(kcp, controlplanev1.CK8sControlPlaneFinalizer) {
		controllerutil.AddFinalizer(kcp, controlplanev1.CK8sControlPlaneFinalizer)

		// patch and return right away instead of reusing the main defer,
		// because the main defer may take too much time to get cluster status
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		if err := patchHelper.Patch(ctx, kcp, patchOpts...); err != nil {
			logger.Error(err, "Failed to patch CK8sControlPlane to add finalizer")
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	var res ctrl.Result
	if !kcp.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		res, err = r.reconcileDelete(ctx, cluster, kcp)
	} else {
		// Handle normal reconciliation loop.
		res, err = r.reconcile(ctx, cluster, kcp)
	}

	// Always attempt to update status.
	if requeue, updateErr := r.updateStatus(ctx, kcp, cluster); updateErr != nil {
		if requeue && res.IsZero() {
			res.RequeueAfter = 10 * time.Second
		}
		var connFailure *ck8s.RemoteClusterConnectionError
		if errors.As(updateErr, &connFailure) {
			logger.Info("Could not connect to workload cluster to fetch status", "updateErr", updateErr.Error())
		} else {
			logger.Error(updateErr, "Failed to update CK8sControlPlane Status")
			err = kerrors.NewAggregate([]error{err, updateErr})
		}
	}

	// Always attempt to Patch the CK8sControlPlane object and status after each reconciliation.
	if patchErr := patchCK8sControlPlane(ctx, patchHelper, kcp); patchErr != nil {
		logger.Error(patchErr, "Failed to patch CK8sControlPlane")
		err = kerrors.NewAggregate([]error{err, patchErr})
	}

	// TODO: remove this as soon as we have a proper remote cluster cache in place.
	// Make KCP to requeue in case status is not ready, so we can check for node status without waiting for a full resync (by default 10 minutes).
	// Only requeue if we are not going in exponential backoff due to error, or if we are not already re-queueing, or if the object has a deletion timestamp.
	if err == nil && !res.Requeue && !(res.RequeueAfter > 0) && kcp.ObjectMeta.DeletionTimestamp.IsZero() {
		if !kcp.Status.Ready {
			res = ctrl.Result{RequeueAfter: 20 * time.Second}
		}
	}

	return res, err
}

// reconcileDelete handles CK8sControlPlane deletion.
// The implementation does not take non-control plane workloads into consideration. This may or may not change in the future.
// Please see https://github.com/kubernetes-sigs/cluster-api/issues/2064.
func (r *CK8sControlPlaneReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.CK8sControlPlane) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", kcp.Namespace, "CK8sControlPlane", kcp.Name, "cluster", cluster.Name)
	logger.Info("Reconcile CK8sControlPlane deletion")

	// Gets all machines, not just control plane machines.
	allMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		return reconcile.Result{}, err
	}
	ownedMachines := allMachines.Filter(collections.OwnedMachines(kcp))

	// If no control plane machines remain, remove the finalizer
	if len(ownedMachines) == 0 {
		controllerutil.RemoveFinalizer(kcp, controlplanev1.CK8sControlPlaneFinalizer)
		return reconcile.Result{}, nil
	}

	controlPlane, err := ck8s.NewControlPlane(ctx, r.Client, cluster, kcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")
		return reconcile.Result{}, err
	}

	// Updates conditions reporting the status of static pods
	// NOTE: Ignoring failures given that we are deleting
	if err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil {
		logger.Info("failed to reconcile conditions", "error", err.Error())
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	// However, during delete we are hiding the counter (1 of x) because it does not make sense given that
	// all the machines are deleted in parallel.
	conditions.SetAggregate(kcp, controlplanev1.MachinesReadyCondition, ownedMachines.ConditionGetters(), conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

	// Verify that only control plane machines remain
	if len(allMachines) != len(ownedMachines) {
		logger.Info("Waiting for worker nodes to be deleted first")
		conditions.MarkFalse(kcp, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Waiting for worker nodes to be deleted first")
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Delete control plane machines in parallel
	machinesToDelete := ownedMachines.Filter(collections.Not(collections.HasDeletionTimestamp))
	var errs []error
	for i := range machinesToDelete {
		m := machinesToDelete[i]
		logger := logger.WithValues("machine", m)
		if err := r.Client.Delete(ctx, machinesToDelete[i]); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to cleanup owned machine")
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedDelete",
			"Failed to delete control plane Machines for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		return reconcile.Result{}, err
	}
	conditions.MarkFalse(kcp, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

func patchCK8sControlPlane(ctx context.Context, patchHelper *patch.Helper, kcp *controlplanev1.CK8sControlPlane) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(kcp,
		conditions.WithConditions(
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			controlplanev1.CertificatesAvailableCondition,
			controlplanev1.TokenAvailableCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		kcp,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			controlplanev1.CertificatesAvailableCondition,
			controlplanev1.TokenAvailableCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}

func (r *CK8sControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, log *logr.Logger) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.CK8sControlPlane{}).
		Owns(&clusterv1.Machine{}).
		//	WithOptions(options).
		//	WithEventFilter(predicates.ResourceNotPaused(r.Log)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToCK8sControlPlane),
			builder.WithPredicates(
				predicates.All(mgr.GetScheme(), ctrl.LoggerFrom(ctx),
					predicates.ClusterPausedTransitionsOrInfrastructureReady(mgr.GetScheme(), r.Log),
				),
			),
		).
		Build(r)
	if err != nil {
		return fmt.Errorf("failed setting up with a controller manager: %w", err)
	}

	r.Scheme = mgr.GetScheme()
	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("ck8s-control-plane-controller")

	if r.managementCluster == nil {
		r.managementCluster = &ck8s.Management{
			Client:          r.Client,
			K8sdDialTimeout: r.K8sdDialTimeout,
		}
	}

	if r.managementClusterUncached == nil {
		r.managementClusterUncached = &ck8s.Management{
			Client:          mgr.GetClient(),
			K8sdDialTimeout: r.K8sdDialTimeout,
		}
	}

	return nil
}

// ClusterToCK8sControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for CK8sControlPlane based on updates to a Cluster.
func (r *CK8sControlPlaneReconciler) ClusterToCK8sControlPlane(_ context.Context, o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "CK8sControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
}

// updateStatus is called after every reconcilitation loop in a defer statement to always make sure we have the
// resource status subresourcs up-to-date.
func (r *CK8sControlPlaneReconciler) updateStatus(ctx context.Context, kcp *controlplanev1.CK8sControlPlane, cluster *clusterv1.Cluster) (bool, error) {
	selector := collections.ControlPlaneSelectorForCluster(cluster.Name)
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	kcp.Status.Selector = selector.String()

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster), collections.OwnedMachines(kcp))
	if err != nil {
		return false, fmt.Errorf("failed to get list of owned machines: %w", err)
	}

	logger := r.Log.WithValues("namespace", kcp.Namespace, "CK8sControlPlane", kcp.Name, "cluster", cluster.Name)
	controlPlane, err := ck8s.NewControlPlane(ctx, r.Client, cluster, kcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")
		return false, err
	}
	kcp.Status.UpdatedReplicas = int32(len(controlPlane.UpToDateMachines()))

	replicas := int32(len(ownedMachines))
	desiredReplicas := *kcp.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	kcp.Status.Replicas = replicas
	kcp.Status.ReadyReplicas = 0
	kcp.Status.UnavailableReplicas = replicas

	lowestVersion := ownedMachines.LowestVersion()
	if lowestVersion != nil {
		kcp.Status.Version = lowestVersion
	}

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !kcp.DeletionTimestamp.IsZero() {
		return false, nil
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		conditions.MarkFalse(kcp, controlplanev1.ResizedCondition, controlplanev1.ScalingUpReason, clusterv1.ConditionSeverityWarning, "Scaling up control plane to %d replicas (actual %d)", desiredReplicas, replicas)
	// We are scaling down
	case replicas > desiredReplicas:
		conditions.MarkFalse(kcp, controlplanev1.ResizedCondition, controlplanev1.ScalingDownReason, clusterv1.ConditionSeverityWarning, "Scaling down control plane to %d replicas (actual %d)", desiredReplicas, replicas)
	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		readyMachines := ownedMachines.Filter(collections.IsReady())
		if int32(len(readyMachines)) == replicas {
			conditions.MarkTrue(kcp, controlplanev1.ResizedCondition)
		}
	}

	microclusterPort := kcp.Spec.CK8sConfigSpec.ControlPlaneConfig.GetMicroclusterPort()
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster), microclusterPort)
	if err != nil {
		return false, fmt.Errorf("failed to create remote cluster client: %w", err)
	}
	status, err := workloadCluster.ClusterStatus(ctx)
	if err != nil {
		// we will requeue if HasK8sdConfigMap hasn't been set yet.
		return !status.HasK8sdConfigMap, err
	}

	logger.Info("ClusterStatus", "workload", status)

	kcp.Status.ReadyReplicas = status.ReadyNodes
	kcp.Status.UnavailableReplicas = replicas - status.ReadyNodes

	// NOTE(neoaggelos): We consider the control plane to be initialized iff the k8sd-config exists
	if status.HasK8sdConfigMap {
		kcp.Status.Initialized = true
	}

	if kcp.Status.ReadyReplicas > 0 {
		kcp.Status.Ready = true
		conditions.MarkTrue(kcp, controlplanev1.AvailableCondition)
	}

	// Surface lastRemediation data in status.
	// LastRemediation is the remediation currently in progress, in any, or the
	// most recent of the remediation we are keeping track on machines.
	var lastRemediation *RemediationData

	if v, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		remediationData, err := RemediationDataFromAnnotation(v)
		if err != nil {
			return false, err
		}
		lastRemediation = remediationData
	} else {
		for _, m := range controlPlane.Machines.UnsortedList() {
			if v, ok := m.Annotations[controlplanev1.RemediationForAnnotation]; ok {
				remediationData, err := RemediationDataFromAnnotation(v)
				if err != nil {
					return false, err
				}
				if lastRemediation == nil || lastRemediation.Timestamp.Time.Before(remediationData.Timestamp.Time) {
					lastRemediation = remediationData
				}
			}
		}
	}

	if lastRemediation != nil {
		controlPlane.KCP.Status.LastRemediation = lastRemediation.ToStatus()
	}

	return false, nil
}

// reconcile handles CK8sControlPlane reconciliation.
func (r *CK8sControlPlaneReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.CK8sControlPlane) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", kcp.Namespace, "CK8sControlPlane", kcp.Name, "cluster", cluster.Name)
	logger.Info("Reconcile CK8sControlPlane")

	// Make sure to reconcile the external infrastructure reference.
	if err := r.reconcileExternalReference(ctx, cluster, kcp.Spec.MachineTemplate.InfrastructureRef); err != nil {
		return reconcile.Result{}, err
	}

	certificates := secret.NewCertificatesForInitialControlPlane(&kcp.Spec.CK8sConfigSpec)
	controllerRef := metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane"))
	if err := certificates.LookupOrGenerate(ctx, r.Client, util.ObjectKey(cluster), *controllerRef); err != nil {
		logger.Error(err, "unable to lookup or create cluster certificates")
		conditions.MarkFalse(kcp, controlplanev1.CertificatesAvailableCondition, controlplanev1.CertificatesGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return reconcile.Result{}, err
	}
	conditions.MarkTrue(kcp, controlplanev1.CertificatesAvailableCondition)

	if err := token.Reconcile(ctx, r.Client, client.ObjectKeyFromObject(cluster), kcp); err != nil {
		conditions.MarkFalse(kcp, controlplanev1.TokenAvailableCondition, controlplanev1.TokenGenerationFailedReason, clusterv1.ConditionSeverityWarning, "%s", err.Error())
		return reconcile.Result{}, err
	}
	conditions.MarkTrue(kcp, controlplanev1.TokenAvailableCondition)

	// If ControlPlaneEndpoint is not set, return early
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		logger.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return reconcile.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if result, err := r.reconcileKubeconfig(ctx, util.ObjectKey(cluster), cluster.Spec.ControlPlaneEndpoint, kcp); err != nil {
		logger.Error(err, "failed to reconcile Kubeconfig")
		return result, err
	}

	controlPlaneMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, util.ObjectKey(cluster), collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		logger.Error(err, "failed to retrieve control plane machines for cluster")
		return reconcile.Result{}, err
	}

	adoptableMachines := controlPlaneMachines.Filter(collections.AdoptableControlPlaneMachines(cluster.Name))
	if len(adoptableMachines) > 0 {
		// We adopt the Machines and then wait for the update event for the ownership reference to re-queue them so the cache is up-to-date
		// err = r.adoptMachines(ctx, kcp, adoptableMachines, cluster)
		return reconcile.Result{}, err
	}

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(kcp))
	if len(ownedMachines) != len(controlPlaneMachines) {
		logger.Info("Not all control plane machines are owned by this CK8sControlPlane, refusing to operate in mixed management mode")
		return reconcile.Result{}, nil
	}

	controlPlane, err := ck8s.NewControlPlane(ctx, r.Client, cluster, kcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")
		return reconcile.Result{}, err
	}

	if err := r.syncMachines(ctx, kcp, controlPlane); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to sync Machines: %w", err)
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(controlPlane.KCP, controlplanev1.MachinesReadyCondition, ownedMachines.ConditionGetters(), conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

	// Updates conditions reporting the status of static pods
	// NOTE: Conditions reporting KCP operation progress like e.g. Resized or SpecUpToDate are inlined with the rest of the execution.
	if err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile unhealthy machines by triggering deletion and requeue if it is considered safe to remediate,
	// otherwise continue with the other KCP operations.
	if result, err := r.reconcileUnhealthyMachines(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Control plane machines rollout due to configuration changes (e.g. upgrades) takes precedence over other operations.
	needRollout := controlPlane.MachinesNeedingRollout()
	switch {
	case len(needRollout) > 0:
		logger.Info("Rolling out Control Plane machines", "needRollout", needRollout.Names())
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition, controlplanev1.RollingUpdateInProgressReason, clusterv1.ConditionSeverityWarning, "Rolling %d replicas with outdated spec (%d replicas up to date)", len(needRollout), len(controlPlane.Machines)-len(needRollout))
		return r.upgradeControlPlane(ctx, cluster, kcp, controlPlane, needRollout)
	default:
		// make sure last upgrade operation is marked as completed.
		// NOTE: we are checking the condition already exists in order to avoid to set this condition at the first
		// reconciliation/before a rolling upgrade actually starts.
		if conditions.Has(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition) {
			conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition)
		}
	}

	// If we've made it this far, we can assume that all ownedMachines are up to date
	numMachines := len(ownedMachines)
	desiredReplicas := int(*kcp.Spec.Replicas)

	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// Create new Machine w/ init
		logger.Info("Initializing control plane", "Desired", desiredReplicas, "Existing", numMachines)
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.AvailableCondition, controlplanev1.WaitingForCK8sServerReason, clusterv1.ConditionSeverityInfo, "")
		return r.initializeControlPlane(ctx, cluster, kcp, controlPlane)
	// We are scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		// Create a new Machine w/ join
		logger.Info("Scaling up control plane", "Desired", desiredReplicas, "Existing", numMachines)
		return r.scaleUpControlPlane(ctx, cluster, kcp, controlPlane)
	// We are scaling down
	case numMachines > desiredReplicas:
		logger.Info("Scaling down control plane", "Desired", desiredReplicas, "Existing", numMachines)
		// The last parameter (i.e. machines needing to be rolled out) should always be empty here.
		return r.scaleDownControlPlane(ctx, cluster, kcp, controlPlane, collections.Machines{})
	}

	return reconcile.Result{}, nil
}

func (r *CK8sControlPlaneReconciler) reconcileExternalReference(ctx context.Context, cluster *clusterv1.Cluster, ref corev1.ObjectReference) error {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	obj, err := external.Get(ctx, r.Client, &ref)
	if err != nil {
		return err
	}

	// Note: We intentionally do not handle checking for the paused label on an external template reference

	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return err
	}

	obj.SetOwnerReferences(util.EnsureOwnerRef(obj.GetOwnerReferences(), metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))

	return patchHelper.Patch(ctx, obj)
}

func (r *CK8sControlPlaneReconciler) reconcileKubeconfig(ctx context.Context, clusterName client.ObjectKey, endpoint clusterv1.APIEndpoint, kcp *controlplanev1.CK8sControlPlane) (ctrl.Result, error) {
	if endpoint.IsZero() {
		return reconcile.Result{}, nil
	}

	controllerOwnerRef := *metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane"))
	configSecret, err := secret.GetFromNamespacedName(ctx, r.Client, clusterName, secret.Kubeconfig)
	switch {
	case apierrors.IsNotFound(err):
		createErr := kubeconfig.CreateSecretWithOwner(
			ctx,
			r.Client,
			clusterName,
			endpoint.String(),
			controllerOwnerRef,
		)
		if errors.Is(createErr, kubeconfig.ErrDependentCertificateNotFound) {
			return ctrl.Result{RequeueAfter: dependentCertRequeueAfter}, nil
		}
		// always return if we have just created in order to skip rotation checks
		return reconcile.Result{}, createErr

	case err != nil:
		return reconcile.Result{}, fmt.Errorf("failed to retrieve kubeconfig Secret: %w", err)
	}

	// only do rotation on owned secrets
	if !util.IsControlledBy(configSecret, kcp) {
		return reconcile.Result{}, nil
	}

	/**
	// TODO rotation
	needsRotation, err := kubeconfig.NeedsClientCertRotation(configSecret, certs.ClientCertificateRenewalDuration)
	if err != nil {
		return err
	}

	if needsRotation {
		r.Log.Info("rotating kubeconfig secret")
		if err := kubeconfig.RegenerateSecret(ctx, r.Client, configSecret); err != nil {
			return fmt.Errorf("failed to regenerate kubeconfig")
		}
	}
	**/

	return reconcile.Result{}, nil
}

// reconcileControlPlaneConditions is responsible of reconciling conditions reporting the status of static pods.
func (r *CK8sControlPlaneReconciler) reconcileControlPlaneConditions(ctx context.Context, controlPlane *ck8s.ControlPlane) error {
	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !controlPlane.KCP.Status.Initialized {
		return nil
	}

	microclusterPort := controlPlane.KCP.Spec.CK8sConfigSpec.ControlPlaneConfig.GetMicroclusterPort()
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster), microclusterPort)
	if err != nil {
		return fmt.Errorf("cannot get remote client to workload cluster: %w", err)
	}

	// Update conditions status
	workloadCluster.UpdateAgentConditions(ctx, controlPlane)

	// Patch machines with the updated conditions.
	if err := controlPlane.PatchMachines(ctx); err != nil {
		return err
	}

	// KCP will be patched at the end of Reconcile to reflect updated conditions, so we can return now.
	return nil
}

func (r *CK8sControlPlaneReconciler) syncMachines(ctx context.Context, kcp *controlplanev1.CK8sControlPlane, controlPlane *ck8s.ControlPlane) error {
	for machineName := range controlPlane.Machines {
		m := controlPlane.Machines[machineName]
		// If the machine is already being deleted, we don't need to update it.
		if !m.DeletionTimestamp.IsZero() {
			continue
		}

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			return fmt.Errorf("failed to create patch helper for machine: %w", err)
		}

		// Create a new map if machine has no annotations.
		if m.Annotations == nil {
			m.Annotations = map[string]string{}
		}

		// Set annotations
		// Add the annotations from the MachineTemplate.
		for k, v := range kcp.Spec.MachineTemplate.ObjectMeta.Annotations {
			m.Annotations[k] = v
		}

		if err := patchHelper.Patch(ctx, m); err != nil {
			return fmt.Errorf("failed to patch machine annotations: %w", err)
		}

		controlPlane.Machines[machineName] = m
	}
	return nil
}

func (r *CK8sControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.CK8sControlPlane,
	controlPlane *ck8s.ControlPlane,
	machinesRequireUpgrade collections.Machines,
) (ctrl.Result, error) {
	// TODO: handle reconciliation of etcd members and kubeadm config in case they get out of sync with cluster

	/**
	logger := controlPlane.Logger()
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))

	if err != nil {
		logger.Error(err, "failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))
		return reconcile.Result{}, err
	}

	parsedVersion, err := semver.ParseTolerant(kcp.Spec.Version)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf(err, "failed to parse kubernetes version %q", kcp.Spec.Version)
	}


	if kcp.Spec.CK8sConfigSpec.ClusterConfiguration != nil {
		imageRepository := kcp.Spec.CK8sConfigSpec.ClusterConfiguration.ImageRepository
		if err := workloadCluster.UpdateImageRepositoryInKubeadmConfigMap(ctx, imageRepository); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update the image repository in the kubeadm config map")
		}
	}

	if kcp.Spec.CK8sConfigSpec.ClusterConfiguration != nil && kcp.Spec.CK8sConfigSpec.ClusterConfiguration.Etcd.Local != nil {
		meta := kcp.Spec.CK8sConfigSpec.ClusterConfiguration.Etcd.Local.ImageMeta
		if err := workloadCluster.UpdateEtcdVersionInKubeadmConfigMap(ctx, meta.ImageRepository, meta.ImageTag); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update the etcd version in the kubeadm config map")
		}
	}

	if err := workloadCluster.UpdateKubeletConfigMap(ctx, parsedVersion); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to upgrade kubelet config map")
	}
	**/

	if controlPlane.Machines.Len() <= int(*kcp.Spec.Replicas) {
		// scaleUp ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
		return r.scaleUpControlPlane(ctx, cluster, kcp, controlPlane)
	}
	return r.scaleDownControlPlane(ctx, cluster, kcp, controlPlane, machinesRequireUpgrade)
}
