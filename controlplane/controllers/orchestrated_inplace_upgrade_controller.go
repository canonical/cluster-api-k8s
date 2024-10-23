package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
	"github.com/canonical/cluster-api-k8s/pkg/trace"
	"github.com/canonical/cluster-api-k8s/pkg/upgrade/inplace"
)

// OrchestratedInPlaceUpgradeController reconciles a CK8sControlPlane object and manages the in-place upgrades.
type OrchestratedInPlaceUpgradeController struct {
	scheme        *runtime.Scheme
	recorder      record.EventRecorder
	machineGetter inplace.MachineGetter

	client.Client
	Log  logr.Logger
	lock inplace.UpgradeLock
}

// OrchestratedInPlaceUpgradeScope is a struct that holds the context of the upgrade process.
type OrchestratedInPlaceUpgradeScope struct {
	cluster          *clusterv1.Cluster
	ck8sControlPlane *controlplanev1.CK8sControlPlane
	ck8sPatcher      inplace.Patcher
	upgradeTo        string
	ownedMachines    collections.Machines
}

// SetupWithManager sets up the controller with the Manager.
func (r *OrchestratedInPlaceUpgradeController) SetupWithManager(mgr ctrl.Manager) error {
	r.scheme = mgr.GetScheme()
	r.recorder = mgr.GetEventRecorderFor("ck8s-cp-orchestrated-inplace-upgrade-controller")
	r.machineGetter = &ck8s.Management{
		Client: r.Client,
	}
	r.lock = inplace.NewUpgradeLock(r.Client)

	if err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.CK8sControlPlane{}).
		Owns(&clusterv1.Machine{}).
		Complete(r); err != nil {
		return fmt.Errorf("failed to get new controller builder: %w", err)
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create;delete;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// Reconcile handles the reconciliation of a CK8sControlPlane object.
func (r *OrchestratedInPlaceUpgradeController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	traceID := trace.NewID()
	log := r.Log.WithValues("orchestrated_inplace_upgrade", req.NamespacedName, "trace_id", traceID)
	log.V(1).Info("Reconciliation started...")

	ck8sCP := &controlplanev1.CK8sControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, ck8sCP); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("CK8sControlPlane resource not found. Ignoring since the object must be deleted.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get CK8sControlPlane: %w", err)
	}

	if inplace.GetUpgradeInstructions(ck8sCP) == "" {
		log.V(1).Info("CK8sControlPlane has no upgrade instructions, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if isDeleted(ck8sCP) {
		log.V(1).Info("CK8sControlPlane is being deleted, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if !r.machinesAreReady(ck8sCP) {
		log.V(1).Info("Machines are not ready, requeuing...")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	scope, err := r.createScope(ctx, ck8sCP)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create scope: %w", err)
	}

	upgradingMachine, err := r.lock.IsLocked(ctx, scope.cluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check if upgrade is locked: %w", err)
	}

	// Upgrade is locked and a machine is already upgrading
	if upgradingMachine != nil {
		// NOTE(Hue): Maybe none of the `upgrade-to` and `release` annotations are set on the machine.
		// If that's the case, the machine will never get upgraded.
		// We consider this a stale lock and unlock the upgrade process.
		if inplace.GetUpgradeInstructions(upgradingMachine) != scope.upgradeTo {
			log.V(1).Info("Machine does not have expected upgrade instructions, unlocking...", "machine", upgradingMachine.Name)
			if err := r.lock.Unlock(ctx, scope.cluster); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to unlock upgrade: %w", err)
			}
			return ctrl.Result{Requeue: true}, nil
		}

		if inplace.IsUpgraded(upgradingMachine, scope.upgradeTo) {
			if err := r.lock.Unlock(ctx, scope.cluster); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to unlock upgrade: %w", err)
			}

			return ctrl.Result{Requeue: true}, nil
		}

		if inplace.IsMachineUpgradeFailed(upgradingMachine) {
			log.Info("Machine upgrade failed for machine, requeuing...", "machine", upgradingMachine.Name)
			if err := r.markUpgradeFailed(ctx, scope, upgradingMachine); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to mark upgrade as failed: %w", err)
			}

			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		log.V(1).Info("Upgrade is locked, a machine is upgrading, requeuing...", "machine", upgradingMachine.Name)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Check if there are machines to upgrade
	var upgradedMachines int
	for _, m := range scope.ownedMachines {
		if inplace.IsUpgraded(m, scope.upgradeTo) {
			log.V(1).Info("Machine is already upgraded", "machine", m.Name)
			upgradedMachines++
			continue
		}

		if isDeleted(m) {
			log.V(1).Info("Machine is being deleted, requeuing...", "machine", m.Name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Lock the process for the machine and start the upgrade
		if err := r.lock.Lock(ctx, scope.cluster, m); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to lock upgrade for machine %q: %w", m.Name, err)
		}

		if err := r.markMachineToUpgrade(ctx, scope, m); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark machine to upgrade: %w", err)
		}

		log.V(1).Info("Machine marked for upgrade", "machine", m.Name)

		if err := r.markUpgradeInProgress(ctx, scope, m); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark upgrade as in-progress: %w", err)
		}

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if upgradedMachines == len(scope.ownedMachines) {
		if err := r.markUpgradeDone(ctx, scope); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark upgrade as done: %w", err)
		}

		log.V(1).Info("All machines are upgraded")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// markUpgradeInProgress annotates the CK8sControlPlane with in-place upgrade in-progress.
func (r *OrchestratedInPlaceUpgradeController) markUpgradeInProgress(ctx context.Context, scope *OrchestratedInPlaceUpgradeScope, upgradingMachine *clusterv1.Machine) error {
	if err := inplace.MarkUpgradeInProgress(ctx, scope.ck8sControlPlane, scope.upgradeTo, scope.ck8sPatcher); err != nil {
		return fmt.Errorf("failed to mark object with upgrade in-progress: %w", err)
	}

	r.recorder.Eventf(
		scope.ck8sControlPlane,
		corev1.EventTypeNormal,
		bootstrapv1.InPlaceUpgradeInProgressEvent,
		"In-place upgrade is in-progress for %q",
		upgradingMachine.Name,
	)
	return nil
}

// markUpgradeDone annotates the CK8sControlPlane with in-place upgrade done.
func (r *OrchestratedInPlaceUpgradeController) markUpgradeDone(ctx context.Context, scope *OrchestratedInPlaceUpgradeScope) error {
	if err := inplace.MarkUpgradeDone(ctx, scope.ck8sControlPlane, scope.upgradeTo, scope.ck8sPatcher); err != nil {
		return fmt.Errorf("failed to mark object with upgrade done: %w", err)
	}

	r.recorder.Eventf(
		scope.ck8sControlPlane,
		corev1.EventTypeNormal,
		bootstrapv1.InPlaceUpgradeDoneEvent,
		"In-place upgrade is done",
	)
	return nil
}

// markUpgradeFailed annotates the CK8sControlPlane with in-place upgrade failed.
func (r *OrchestratedInPlaceUpgradeController) markUpgradeFailed(ctx context.Context, scope *OrchestratedInPlaceUpgradeScope, failedM *clusterv1.Machine) error {
	if err := inplace.MarkUpgradeFailed(ctx, scope.ck8sControlPlane, scope.ck8sPatcher); err != nil {
		return fmt.Errorf("failed to mark object with upgrade failed: %w", err)
	}

	r.recorder.Eventf(
		scope.ck8sControlPlane,
		corev1.EventTypeWarning,
		bootstrapv1.InPlaceUpgradeFailedEvent,
		"In-place upgrade failed for machine %q.",
		failedM.Name,
	)
	return nil
}

// createScope creates a new OrchestratedInPlaceUpgradeScope.
func (r *OrchestratedInPlaceUpgradeController) createScope(ctx context.Context, ck8sCP *controlplanev1.CK8sControlPlane) (*OrchestratedInPlaceUpgradeScope, error) {
	patchHelper, err := patch.NewHelper(ck8sCP, r.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create new patch helper: %w", err)
	}

	cluster, err := util.GetOwnerCluster(ctx, r.Client, ck8sCP.ObjectMeta)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}

	ownedMachines, err := r.getControlPlaneMachines(ctx, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get owned machines: %w", err)
	}

	return &OrchestratedInPlaceUpgradeScope{
		cluster:          cluster,
		ck8sControlPlane: ck8sCP,
		upgradeTo:        inplace.GetUpgradeInstructions(ck8sCP),
		ownedMachines:    ownedMachines,
		ck8sPatcher:      patchHelper,
	}, nil
}

// getControlPlaneMachines gets the control plane machines of the cluster.
func (r *OrchestratedInPlaceUpgradeController) getControlPlaneMachines(ctx context.Context, cluster *clusterv1.Cluster) (collections.Machines, error) {
	ownedMachines, err := r.machineGetter.GetMachinesForCluster(ctx, client.ObjectKeyFromObject(cluster), collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster machines: %w", err)
	}

	return ownedMachines, nil
}

// markMachineToUpgrade marks the machine to upgrade.
func (r *OrchestratedInPlaceUpgradeController) markMachineToUpgrade(ctx context.Context, scope *OrchestratedInPlaceUpgradeScope, m *clusterv1.Machine) error {
	if err := inplace.MarkMachineToUpgrade(ctx, m, scope.upgradeTo, r.Client); err != nil {
		return fmt.Errorf("failed to mark machine to inplace upgrade: %w", err)
	}

	r.recorder.Eventf(
		scope.ck8sControlPlane,
		corev1.EventTypeNormal,
		bootstrapv1.InPlaceUpgradeInProgressEvent,
		"Machine %q is upgrading to %q",
		m.Name,
		scope.upgradeTo,
	)

	return nil
}

func (r *OrchestratedInPlaceUpgradeController) machinesAreReady(ck8sCP *controlplanev1.CK8sControlPlane) bool {
	if ck8sCP == nil || ck8sCP.Spec.Replicas == nil {
		return false
	}
	return ck8sCP.Status.ReadyReplicas == *ck8sCP.Spec.Replicas
}

// isDeleted returns true if the object is being deleted.
func isDeleted(obj client.Object) bool {
	return !obj.GetDeletionTimestamp().IsZero()
}
