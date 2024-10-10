package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
	"github.com/canonical/cluster-api-k8s/pkg/token"
)

// InPlaceUpgradeReconciler reconciles machines and performs in-place upgrades based on annotations.
type InPlaceUpgradeReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder

	K8sdDialTimeout time.Duration

	managementCluster ck8s.ManagementCluster
}

func (r *InPlaceUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if _, err := ctrl.NewControllerManagedBy(mgr).For(&clusterv1.Machine{}).Build(r); err != nil {
		return fmt.Errorf("failed setting up with a controller manager: %w", err)
	}

	r.Scheme = mgr.GetScheme()
	r.recorder = mgr.GetEventRecorderFor("ck8s-in-place-upgrade-controller")

	if r.managementCluster == nil {
		r.managementCluster = &ck8s.Management{
			Client:          r.Client,
			K8sdDialTimeout: r.K8sdDialTimeout,
		}
	}
	return nil
}

type UpgradeScope struct {
	Log             logr.Logger
	Cluster         *clusterv1.Cluster
	WorkloadCluster *ck8s.Workload
	Machine         *clusterv1.Machine
	PatchHelper     *patch.Helper
	UpgradeOption   string
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=ck8sconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=ck8sconfigs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch

func (r *InPlaceUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("namespace", req.Namespace, "machine", req.Name)

	m := &clusterv1.Machine{}
	if err := r.Client.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	mAnnotations := m.GetAnnotations()

	upgradeOption, ok := mAnnotations[bootstrapv1.InPlaceUpgradeToAnnotation]
	if !ok {
		// In-place upgrade to annotation not found, ignoring...
		return ctrl.Result{}, nil
	}

	// Lookup the cluster the machine belongs to
	cluster, err := util.GetClusterByName(ctx, r.Client, m.Namespace, m.Spec.ClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Get the workload cluster for the machine
	workloadCluster, err := r.getWorkloadClusterForMachine(ctx, util.ObjectKey(cluster), m)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get workload cluster for machine: %w", err)
	}

	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create patch helper for machine: %w", err)
	}

	scope := &UpgradeScope{
		Log:             log,
		Cluster:         cluster,
		WorkloadCluster: workloadCluster,
		Machine:         m,
		PatchHelper:     patchHelper,
		UpgradeOption:   upgradeOption,
	}

	changeID, hasChangeIDAnnotation := mAnnotations[bootstrapv1.InPlaceUpgradeChangeIDAnnotation]

	if hasChangeIDAnnotation {
		// Still handling an in-progress upgrade since change ID is present
		upgradeStatus, ok := mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation]
		if !ok {
			log.Info("Found in-place upgrade change ID without status, marking as failed")
			if err := r.markUpgradeFailed(ctx, scope, "missing in-place upgrade status"); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to mark in place upgrade status: %w", err)
			}
		}

		switch upgradeStatus {
		case bootstrapv1.InPlaceUpgradeInProgressStatus:
			return r.handleUpgradeInProgress(ctx, scope, changeID)
		case bootstrapv1.InPlaceUpgradeDoneStatus:
			return r.handleUpgradeDone(ctx, scope)
		case bootstrapv1.InPlaceUpgradeFailedStatus:
			return r.handleUpgradeFailed(ctx, scope)
		default:
			log.Info("Found invalid in-place upgrade status, marking as failed")
			if err := r.markUpgradeFailed(ctx, scope, "invalid in-place upgrade status"); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to mark in place upgrade status: %w", err)
			}
		}

		return ctrl.Result{}, nil
	}

	// Starting a new upgrade or retrying a failed one
	return r.handleUpgradeRequest(ctx, scope)
}

func (r *InPlaceUpgradeReconciler) getWorkloadClusterForMachine(ctx context.Context, clusterKey types.NamespacedName, m *clusterv1.Machine) (*ck8s.Workload, error) {
	// Lookup the ck8s config used by the machine
	config := &bootstrapv1.CK8sConfig{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}, config); err != nil {
		return nil, err
	}

	microclusterPort := config.Spec.ControlPlaneConfig.GetMicroclusterPort()
	return r.managementCluster.GetWorkloadCluster(ctx, clusterKey, microclusterPort)
}

func (r *InPlaceUpgradeReconciler) markUpgradeInProgress(ctx context.Context, scope *UpgradeScope, changeID string) error {
	mAnnotations := scope.Machine.GetAnnotations()

	mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeInProgressStatus
	mAnnotations[bootstrapv1.InPlaceUpgradeChangeIDAnnotation] = changeID
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.PatchHelper.Patch(ctx, scope.Machine); err != nil {
		return fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	r.recorder.Eventf(scope.Machine, corev1.EventTypeNormal, bootstrapv1.InPlaceUpgradeInProgressEvent, "Performing in place upgrade with %s", scope.UpgradeOption)
	return nil
}

func (r *InPlaceUpgradeReconciler) markUpgradeDone(ctx context.Context, scope *UpgradeScope) error {
	mAnnotations := scope.Machine.GetAnnotations()

	mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeDoneStatus
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.PatchHelper.Patch(ctx, scope.Machine); err != nil {
		return fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	r.recorder.Eventf(scope.Machine, corev1.EventTypeNormal, bootstrapv1.InPlaceUpgradeDoneEvent, "Successfully performed in place upgrade with %s", scope.UpgradeOption)
	return nil
}

func (r *InPlaceUpgradeReconciler) markUpgradeFailed(ctx context.Context, scope *UpgradeScope, failure string) error {
	mAnnotations := scope.Machine.GetAnnotations()

	mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeFailedStatus
	// NOTE(Hue): Add an annotation here to indicate that the upgrade failed
	// and we are not going to retry it.
	mAnnotations[bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation] = time.Now().Format(time.RFC1123Z)
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.PatchHelper.Patch(ctx, scope.Machine); err != nil {
		return fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	r.recorder.Eventf(scope.Machine, corev1.EventTypeWarning, bootstrapv1.InPlaceUpgradeFailedEvent, "Failed to perform in place upgrade with %s: %s", scope.UpgradeOption, failure)
	return nil
}

func (r *InPlaceUpgradeReconciler) handleUpgradeRequest(ctx context.Context, scope *UpgradeScope) (reconcile.Result, error) {
	// Lookup the node token for the machine
	nodeToken, err := token.LookupNodeToken(ctx, r.Client, util.ObjectKey(scope.Cluster), scope.Machine.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to lookup node token: %w", err)
	}

	mAnnotations := scope.Machine.GetAnnotations()

	delete(mAnnotations, bootstrapv1.InPlaceUpgradeStatusAnnotation)
	delete(mAnnotations, bootstrapv1.InPlaceUpgradeChangeIDAnnotation)
	delete(mAnnotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.PatchHelper.Patch(ctx, scope.Machine); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	// Perform the in-place upgrade through snap refresh
	changeID, err := scope.WorkloadCluster.RefreshMachine(ctx, scope.Machine, *nodeToken, scope.UpgradeOption)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to refresh machine: %w", err)
	}

	// Set in place upgrade status to in progress
	if err := r.markUpgradeInProgress(ctx, scope, changeID); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to mark in place upgrade status: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *InPlaceUpgradeReconciler) handleUpgradeInProgress(ctx context.Context, scope *UpgradeScope, changeID string) (reconcile.Result, error) {
	// Lookup the node token for the machine
	nodeToken, err := token.LookupNodeToken(ctx, r.Client, util.ObjectKey(scope.Cluster), scope.Machine.Name)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to lookup node token: %w", err)
	}

	status, err := scope.WorkloadCluster.GetRefreshStatusForMachine(ctx, scope.Machine, *nodeToken, changeID)
	if err != nil {
		scope.Log.Info("Failed to get refresh status for machine", "error", err)
		return ctrl.Result{}, fmt.Errorf("failed to get refresh status for machine: %w", err)
	}

	if !status.Completed {
		scope.Log.Info("In-place upgrade still in progress, requeuing...")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	switch status.Status {
	case "Done":
		scope.Log.Info("In-place upgrade completed successfully")
		if err := r.markUpgradeDone(ctx, scope); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark in place upgrade status: %w", err)
		}
	case "Error":
		scope.Log.Info("In-place upgrade failed", "error", status.ErrorMessage)
		if err := r.markUpgradeFailed(ctx, scope, status.ErrorMessage); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark in place upgrade status: %w", err)
		}
	default:
		scope.Log.Info("Found invalid refresh status, marking as failed")
		if err := r.markUpgradeFailed(ctx, scope, "invalid refresh status"); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark in place upgrade status: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

func (r *InPlaceUpgradeReconciler) handleUpgradeDone(ctx context.Context, scope *UpgradeScope) (reconcile.Result, error) {
	mAnnotations := scope.Machine.GetAnnotations()

	delete(mAnnotations, bootstrapv1.InPlaceUpgradeToAnnotation)
	delete(mAnnotations, bootstrapv1.InPlaceUpgradeChangeIDAnnotation)
	delete(mAnnotations, bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation)
	mAnnotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation] = scope.UpgradeOption
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.PatchHelper.Patch(ctx, scope.Machine); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	return ctrl.Result{}, nil
}

func (r *InPlaceUpgradeReconciler) handleUpgradeFailed(ctx context.Context, scope *UpgradeScope) (reconcile.Result, error) {
	mAnnotations := scope.Machine.GetAnnotations()

	// NOTE(Hue): We don't remove the `LastFailedAttemptAt` annotation here
	// because we want to know if the upgrade failed at some point in the `MachineDeploymentReconciler`.
	// This function triggers a retry by removing the `Status` and `ChangeID` annotations,
	// but the `LastFailedAttemptAt` lets us descriminiate between a retry and a fresh upgrade.
	// Overall, we don't remove the `LastFailedAttemptAt` annotation in the `InPlaceUpgradeReconciler`.
	// That's the responsibility of the `MachineDeploymentReconciler`.

	delete(mAnnotations, bootstrapv1.InPlaceUpgradeStatusAnnotation)
	delete(mAnnotations, bootstrapv1.InPlaceUpgradeChangeIDAnnotation)
	scope.Machine.SetAnnotations(mAnnotations)
	if err := scope.PatchHelper.Patch(ctx, scope.Machine); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to patch machine annotations: %w", err)
	}

	return ctrl.Result{}, nil
}
