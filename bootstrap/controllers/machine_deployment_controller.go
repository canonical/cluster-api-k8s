package controllers

import (
	"context"
	"fmt"
	"slices"
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
	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
	"github.com/canonical/cluster-api-k8s/pkg/trace"
)

// MachineGetter is an interface that defines the methods a MachineDeploymentReconciler uses to get machines.
type MachineGetter interface {
	GetMachinesForCluster(ctx context.Context, cluster client.ObjectKey, filters ...collections.Func) (collections.Machines, error)
}

// MachineDeploymentReconciler reconciles a MachineDeployment object and manages the in-place upgrades.
type MachineDeploymentReconciler struct {
	scheme        *runtime.Scheme
	recorder      record.EventRecorder
	machineGetter MachineGetter

	client.Client
	Log logr.Logger
}

// MachineDeploymentUpgradeScope is a struct that holds the context of the upgrade process.
type MachineDeploymentUpgradeScope struct {
	MachineDeployment *clusterv1.MachineDeployment
	PatchHelper       *patch.Helper
	UpgradeTo         string
	OwnedMachines     []*clusterv1.Machine
}

// NewMachineDeploymentReconciler creates a new MachineDeploymentReconciler.
func (r *MachineDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.scheme = mgr.GetScheme()
	r.recorder = mgr.GetEventRecorderFor("ck8s-machine-deployment-controller")

	if r.machineGetter == nil {
		r.machineGetter = &ck8s.Management{
			Client: r.Client,
		}
	}

	// NOTE(Hue): Initially, I tried to go with comprehensive predicates but there was two problems with that:
	// 1. It was not really understandable and mantainable.
	// 2. Sometimes the reconciliation was not getting triggered when it should have, debugging this
	// through the predicates was a nightmare.
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.MachineDeployment{}).
		Owns(&clusterv1.Machine{}).
		Complete(r); err != nil {
		return fmt.Errorf("failed to get new controller builder: %w", err)
	}

	return nil
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets;machinesets/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments;machinedeployments/status,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles the reconciliation of a MachineDeployment object.
func (r *MachineDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	traceID := trace.NewID()
	log := r.Log.WithValues("machine_deployment", req.NamespacedName, "trace_id", traceID)
	log.V(1).Info("Reconciliation started...")

	machineDeployment := &clusterv1.MachineDeployment{}
	if err := r.Get(ctx, req.NamespacedName, machineDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("MachineDeployment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get MachineDeployment: %w", err)
	}

	if r.getUpgradeInstructions(machineDeployment) == "" {
		log.V(1).Info("MachineDeployment has no upgrade instructions, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	if !machineDeployment.DeletionTimestamp.IsZero() {
		log.V(1).Info("MachineDeployment is being deleted, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	ownedMachines, err := r.getOwnedMachines(ctx, machineDeployment)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get owned machines: %w", err)
	}

	scope, err := r.createScope(machineDeployment, ownedMachines)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create scope: %w", err)
	}

	// Starting the upgrade process
	var upgradedMachines int
	for _, m := range ownedMachines {
		if r.isMachineUpgraded(scope, m) {
			log.V(1).Info("Machine is already upgraded", "machine", m.Name)
			upgradedMachines++
			continue
		}

		if !m.DeletionTimestamp.IsZero() {
			log.V(1).Info("Machine is being deleted, requeuing...", "machine", m.Name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		if r.isMachineUpgradeFailed(m) {
			log.Info("Machine upgrade failed for machine, requeuing...", "machine", m.Name)
			if err := r.markUpgradeFailed(ctx, scope, m); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to mark upgrade as failed: %w", err)
			}

			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		if r.isMachineUpgrading(m) {
			log.V(1).Info("Machine is upgrading, requeuing...", "machine", m.Name)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		// Machine is not upgraded, mark it for upgrade
		if err := r.markMachineToUpgrade(ctx, scope, m); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark machine to upgrade: %w", err)
		}

		log.V(1).Info("Machine marked for upgrade", "machine", m.Name)

		if err := r.markUpgradeInProgress(ctx, scope, m); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark upgrade as in-progress: %w", err)
		}

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if upgradedMachines == len(ownedMachines) {
		if err := r.markUpgradeDone(ctx, scope); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to mark upgrade as done: %w", err)
		}

		log.V(1).Info("All machines are upgraded")
		return ctrl.Result{}, nil
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// markUpgradeInProgress marks the MachineDeployment as in-place upgrade in-progress.
func (r *MachineDeploymentReconciler) markUpgradeInProgress(ctx context.Context, scope *MachineDeploymentUpgradeScope, upgradingMachine *clusterv1.Machine) error {
	mdAnnotations := scope.MachineDeployment.Annotations
	if mdAnnotations == nil {
		mdAnnotations = make(map[string]string)
	}

	// clean up
	delete(mdAnnotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)

	mdAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeInProgressStatus
	mdAnnotations[bootstrapv1.InPlaceUpgradeToAnnotation] = scope.UpgradeTo

	scope.MachineDeployment.SetAnnotations(mdAnnotations)

	if err := scope.PatchHelper.Patch(ctx, scope.MachineDeployment); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	r.recorder.Eventf(
		scope.MachineDeployment,
		corev1.EventTypeNormal,
		bootstrapv1.InPlaceUpgradeInProgressEvent,
		"In-place upgrade is in-progress for %q",
		upgradingMachine.Name,
	)
	return nil
}

// markUpgradeDone marks the MachineDeployment as in-place upgrade done.
func (r *MachineDeploymentReconciler) markUpgradeDone(ctx context.Context, scope *MachineDeploymentUpgradeScope) error {
	annotations := scope.MachineDeployment.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// clean up
	delete(annotations, bootstrapv1.InPlaceUpgradeToAnnotation)

	annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeDoneStatus
	annotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation] = scope.UpgradeTo

	scope.MachineDeployment.SetAnnotations(annotations)

	if err := scope.PatchHelper.Patch(ctx, scope.MachineDeployment); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	r.recorder.Eventf(
		scope.MachineDeployment,
		corev1.EventTypeNormal,
		bootstrapv1.InPlaceUpgradeDoneEvent,
		"In-place upgrade is done",
	)
	return nil
}

// markUpgradeFailed marks the MachineDeployment as in-place upgrade failed.
func (r *MachineDeploymentReconciler) markUpgradeFailed(ctx context.Context, scope *MachineDeploymentUpgradeScope, failedM *clusterv1.Machine) error {
	annotations := scope.MachineDeployment.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// clean up
	delete(annotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)

	annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeFailedStatus
	scope.MachineDeployment.SetAnnotations(annotations)

	if err := scope.PatchHelper.Patch(ctx, scope.MachineDeployment); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	r.recorder.Eventf(
		scope.MachineDeployment,
		corev1.EventTypeWarning,
		bootstrapv1.InPlaceUpgradeFailedEvent,
		"In-place upgrade failed for machine %q.",
		failedM.Name,
	)
	return nil
}

// createScope creates a new MachineDeploymentUpgradeScope.
func (r *MachineDeploymentReconciler) createScope(md *clusterv1.MachineDeployment, ownedMachines []*clusterv1.Machine) (*MachineDeploymentUpgradeScope, error) {
	patchHelper, err := patch.NewHelper(md, r.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create new patch helper: %w", err)
	}

	return &MachineDeploymentUpgradeScope{
		MachineDeployment: md,
		UpgradeTo:         r.getUpgradeInstructions(md),
		OwnedMachines:     ownedMachines,
		PatchHelper:       patchHelper,
	}, nil
}

// getCluster gets the Cluster object for the MachineDeployment.
func (r *MachineDeploymentReconciler) getCluster(ctx context.Context, md *clusterv1.MachineDeployment) (*clusterv1.Cluster, error) {
	cluster := &clusterv1.Cluster{}
	clusterKey := client.ObjectKey{
		Namespace: md.Namespace,
		Name:      md.Spec.ClusterName,
	}
	if err := r.Get(ctx, clusterKey, cluster); err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	return cluster, nil
}

// getOwnedMachines gets the machines owned by the MachineDeployment.
func (r *MachineDeploymentReconciler) getOwnedMachines(ctx context.Context, md *clusterv1.MachineDeployment) ([]*clusterv1.Machine, error) {
	cluster, err := r.getCluster(ctx, md)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}

	// NOTE(Hue): The machines are not owned by the MachineDeployment directly, but by the MachineSet.
	var (
		msList   clusterv1.MachineSetList
		selector = map[string]string{
			clusterv1.ClusterNameLabel:           cluster.Name,
			clusterv1.MachineDeploymentNameLabel: md.Name,
		}
	)
	if err := r.List(ctx, &msList, client.InNamespace(cluster.Namespace), client.MatchingLabels(selector)); err != nil {
		return nil, fmt.Errorf("failed to get MachineSetList: %w", err)
	}

	var (
		ms    clusterv1.MachineSet
		found bool
	)
	// NOTE(Hue): The nosec is due to a false positive: https://stackoverflow.com/questions/62446118/implicit-memory-aliasing-in-for-loop
	for _, _ms := range msList.Items { // #nosec G601
		if util.IsOwnedByObject(&_ms, md) {
			ms = _ms
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("failed to find MachineSet owned by MachineDeployment %q", md.Name)
	}

	ownedMachinesCollection, err := r.machineGetter.GetMachinesForCluster(ctx, client.ObjectKeyFromObject(cluster), collections.OwnedMachines(&ms))
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster machines: %w", err)
	}

	ownedMachines := make([]*clusterv1.Machine, len(ownedMachinesCollection))
	i := 0
	for _, m := range ownedMachinesCollection {
		ownedMachines[i] = m
		i++
	}

	// NOTE(Hue): Sorting machines by their UID to make sure we have a deterministic order.
	// This is to (kind of) make sure we upgrade the machines in the same order every time.
	// Meaning that if in the previous reconciliation we annotated a machine with upgrade-to,
	// In the next reconciliation we will make sure that upgrade was successful before moving
	// to the next machine.
	// This is not the most robust way to do this, but it's good enough.
	// A better way to do this might be to use some kind of lock (via a secret or something),
	// similar to control plane init lock.
	slices.SortStableFunc(ownedMachines, func(m1, m2 *clusterv1.Machine) int {
		switch {
		case m1.UID < m2.UID:
			return -1
		case m1.UID == m2.UID:
			return 0
		default:
			return 1
		}
	})

	return ownedMachines, nil
}

// isMachineUpgraded checks if the machine is already upgraded.
func (r *MachineDeploymentReconciler) isMachineUpgraded(scope *MachineDeploymentUpgradeScope, m *clusterv1.Machine) bool {
	mUpgradeRelease := m.Annotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation]
	return mUpgradeRelease == scope.UpgradeTo
}

// isMachineUpgrading checks if the machine is upgrading.
func (r *MachineDeploymentReconciler) isMachineUpgrading(m *clusterv1.Machine) bool {
	return m.Annotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] == bootstrapv1.InPlaceUpgradeInProgressStatus ||
		m.Annotations[bootstrapv1.InPlaceUpgradeToAnnotation] != ""
}

// isMachineUpgradeFailed checks if the machine upgrade failed.
func (r *MachineDeploymentReconciler) isMachineUpgradeFailed(m *clusterv1.Machine) bool {
	return m.Annotations[bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation] != ""
}

// markMachineToUpgrade marks the machine to upgrade.
func (r *MachineDeploymentReconciler) markMachineToUpgrade(ctx context.Context, scope *MachineDeploymentUpgradeScope, m *clusterv1.Machine) error {
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return fmt.Errorf("failed to create new patch helper: %w", err)
	}

	if m.Annotations == nil {
		m.Annotations = make(map[string]string)
	}

	// clean up
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeStatusAnnotation)
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeChangeIDAnnotation)
	delete(m.Annotations, bootstrapv1.InPlaceUpgradeLastFailedAttemptAtAnnotation)

	m.Annotations[bootstrapv1.InPlaceUpgradeToAnnotation] = scope.UpgradeTo

	if err := patchHelper.Patch(ctx, m); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	r.recorder.Eventf(
		scope.MachineDeployment,
		corev1.EventTypeNormal,
		bootstrapv1.InPlaceUpgradeInProgressEvent,
		"Machine %q is upgrading to %q",
		m.Name,
		scope.UpgradeTo,
	)

	return nil
}

func (r *MachineDeploymentReconciler) getUpgradeInstructions(md *clusterv1.MachineDeployment) string {
	// NOTE(Hue): The reason we are checking the `release` annotation as well is that we want to make sure
	// we upgrade the new machines that joined after the initial upgrade process.
	// The `upgrade-to` overwrites the `release` annotation, because we might have both in case
	// the user decides to do another in-place upgrade after a successful one.
	upgradeTo := md.Annotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation]
	if to, ok := md.Annotations[bootstrapv1.InPlaceUpgradeToAnnotation]; ok {
		upgradeTo = to
	}

	return upgradeTo
}
