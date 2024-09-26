package controllers

import (
	"context"
	"fmt"
	"time"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
	utiltime "github.com/canonical/cluster-api-k8s/pkg/time"
	"github.com/canonical/cluster-api-k8s/pkg/token"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bsutil "sigs.k8s.io/cluster-api/bootstrap/util"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CertificatesReconciler reconciles a Machine object
type CertificatesReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder

	K8sdDialTimeout time.Duration

	managementCluster ck8s.ManagementCluster
}

// SetupWithManager sets up the controller with the Manager.
func (r *CertificatesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if _, err := ctrl.NewControllerManagedBy(mgr).For(&clusterv1.Machine{}).Build(r); err != nil {
		return err
	}

	r.Scheme = mgr.GetScheme()
	r.recorder = mgr.GetEventRecorderFor("ck8s-certificates-controller")

	if r.managementCluster == nil {
		r.managementCluster = &ck8s.Management{
			Client:          r.Client,
			K8sdDialTimeout: r.K8sdDialTimeout,
		}
	}
	return nil
}

type CertificatesScope struct {
	Cluster  *clusterv1.Cluster
	Config   *bootstrapv1.CK8sConfig
	Log      logr.Logger
	Machine  *clusterv1.Machine
	Patcher  *patch.Helper
	Workload *ck8s.Workload
}

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=ck8sconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=ck8sconfigs/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=exp.cluster.x-k8s.io,resources=machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete
func (r *CertificatesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace, "machine", req.Name)

	m := &clusterv1.Machine{}
	if err := r.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if !m.ObjectMeta.DeletionTimestamp.IsZero() {
		// Machine is being deleted, return early.
		return ctrl.Result{}, nil
	}

	mAnnotations := m.GetAnnotations()

	var refreshCertificates, updateExpiryDate bool
	_, refreshCertificates = mAnnotations[bootstrapv1.CertificatesRefreshAnnotation]
	_, updateExpiryDate = mAnnotations[bootstrapv1.MachineCertificatesExpiryDateAnnotation]
	if !refreshCertificates && updateExpiryDate {
		// No need to refresh certificates or update expiry date, return early.
		return ctrl.Result{}, nil
	}

	// Look up the Cluster Config
	config := &bootstrapv1.CK8sConfig{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}, config); err != nil {
		return ctrl.Result{}, err
	}

	// Look up for the owner
	configOwner, err := bsutil.GetConfigOwner(ctx, r.Client, config)
	if err != nil {
		log.Error(err, "Failed to get config owner")
		return ctrl.Result{}, err
	}
	if configOwner == nil {
		return ctrl.Result{}, nil
	}

	cluster, err := util.GetClusterByName(ctx, r.Client, m.GetNamespace(), m.Spec.ClusterName)

	microclusterPort := config.Spec.ControlPlaneConfig.GetMicroclusterPort()
	workload, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster), microclusterPort)
	if err != nil {
		return ctrl.Result{}, err
	}

	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create patch helper for machine: %w", err)
	}

	scope := &CertificatesScope{
		Log:      log,
		Machine:  m,
		Config:   config,
		Cluster:  cluster,
		Patcher:  patchHelper,
		Workload: workload,
	}

	if refreshCertificates {
		if configOwner.IsControlPlaneMachine() {
			return r.refreshControlPlaneCertificates(ctx, scope)
		} else {
			return ctrl.Result{}, fmt.Errorf("Worker nodes are not supported yet")
		}
	}

	if !updateExpiryDate {
		return r.updateExpiryDateAnnotation(ctx, scope)
	}

	return ctrl.Result{}, nil
}

func (r *CertificatesReconciler) refreshControlPlaneCertificates(ctx context.Context, scope *CertificatesScope) (reconcile.Result, error) {
	nodeToken, err := token.LookupNodeToken(ctx, r.Client, util.ObjectKey(scope.Cluster), scope.Machine.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	mAnnotations := scope.Machine.GetAnnotations()

	refreshAnnotation, ok := mAnnotations[bootstrapv1.CertificatesRefreshAnnotation]
	if !ok {
		return ctrl.Result{}, nil
	}

	seconds, err := utiltime.TTLToSeconds(refreshAnnotation)
	scope.Log.Info("Refreshing Certificates", "TTL", refreshAnnotation, "seconds", seconds)
	if err != nil {
		scope.Log.Error(err, "Failed to parse TTL annotation")
		return ctrl.Result{}, err
	}

	controlPlaneConfig := scope.Config.Spec.ControlPlaneConfig
	controlPlaneEndpoint := scope.Cluster.Spec.ControlPlaneEndpoint.Host
	extraSANs := append(controlPlaneConfig.ExtraSANs, controlPlaneEndpoint)

	expirySecondsUnix, err := scope.Workload.RefreshCertificates(ctx, scope.Machine, *nodeToken, seconds, extraSANs)
	if err != nil {
		scope.Log.Error(err, "Failed to refresh certificates")
		r.recorder.Eventf(scope.Machine, corev1.EventTypeWarning, bootstrapv1.CertificatesRefreshFailedEvent, "Failed to refresh certificates: %v", err)
		return ctrl.Result{}, err
	}

	scope.Log.Info("Certificates refreshed", "expiry", expirySecondsUnix)

	expiryTime := time.Unix(int64(expirySecondsUnix), 0)

	delete(mAnnotations, bootstrapv1.CertificatesRefreshAnnotation)
	mAnnotations[bootstrapv1.MachineCertificatesExpiryDateAnnotation] = expiryTime.Format(time.RFC3339)
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.Patcher.Patch(ctx, scope.Machine); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	r.recorder.Eventf(scope.Machine, corev1.EventTypeNormal, bootstrapv1.CertificatesRefreshDoneEvent, "Certificates refreshed, will expire at %s", expiryTime)

	return ctrl.Result{}, nil
}

func (r *CertificatesReconciler) updateExpiryDateAnnotation(ctx context.Context, scope *CertificatesScope) (reconcile.Result, error) {
	nodeToken, err := token.LookupNodeToken(ctx, r.Client, util.ObjectKey(scope.Cluster), scope.Machine.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	mAnnotations := scope.Machine.GetAnnotations()

	expiryDateString, err := scope.Workload.GetCertificatesExpiryDate(ctx, scope.Machine, *nodeToken)
	if err != nil {
		scope.Log.Error(err, "Failed to get certificates expiry date")
		return ctrl.Result{}, err
	}

	mAnnotations[bootstrapv1.MachineCertificatesExpiryDateAnnotation] = expiryDateString
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.Patcher.Patch(ctx, scope.Machine); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	return ctrl.Result{}, nil
}
