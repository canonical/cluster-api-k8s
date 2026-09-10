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
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta3"
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

func (r *CK8sControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "ck8sControlPlane", req.Name)

	logger.Info("CK8sControlPlaneReconciler reconcile request received")

	// Fetch the CK8sControlPlane instance.
	kcp := &controlplanev1.CK8sControlPlane{}
	if err := r.Get(ctx, req.NamespacedName, kcp); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to retrieve CK8sControlPlane: Not Found")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to retrieve CK8sControlPlane")
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, kcp.ObjectMeta)
	if err != nil {
		// It should be an issue to be investigated if the controller get the NotFound status.
		// So, it should return the error.
		return ctrl.Result{}, pkgerrors.Wrapf(err, "failed to retrieve owner Cluster")
	}
	if cluster == nil {
		logger.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("cluster", cluster.Name)

	if annotations.IsPaused(cluster, kcp) {
		logger.Info("Reconciliation is paused for this object")
		return reconcile.Result{}, nil
	}

	// Wait for the cluster infrastructure to be ready before creating machines
	if !ptr.Deref(cluster.Status.Initialization.InfrastructureProvisioned, false) {
		logger.Info("Cluster infrastructure is not ready. Requeuing CK8sControlPlane")
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
		patchOpts := make([]patch.Option, 0, 1)
		patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		if err := patchHelper.Patch(ctx, kcp, patchOpts...); err != nil {
			logger.Error(err, "Failed to patch CK8sControlPlane to add finalizer")
			return reconcile.Result{}, err
		}
		logger.Info("Added finalizer and patched WithStatusObservedGeneration for CK8sControlPlane")
		return reconcile.Result{}, nil
	}

	defer func() {
		// Always attempt to update status.
		if updateErr := r.updateStatus(ctx, kcp, cluster); updateErr != nil {
			var connFailure *ck8s.RemoteClusterConnectionError
			if errors.As(updateErr, &connFailure) {
				logger.Info("Could not connect to workload cluster to fetch status", "updateErr", updateErr.Error())
			} else {
				logger.Error(updateErr, "Failed to update CK8sControlPlane Status")
				reterr = kerrors.NewAggregate([]error{reterr, updateErr})
			}
		}

		// Always attempt to Patch the CK8sControlPlane object and status after each reconciliation.
		if patchErr := patchCK8sControlPlane(ctx, patchHelper, kcp); patchErr != nil {
			logger.Error(patchErr, "Failed to patch CK8sControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, patchErr})
		}

		// TODO: remove this as soon as we have a proper remote cluster cache in place.
		// Make KCP to requeue in case status is not ready, so we can check for node status without waiting for a full resync (by default 10 minutes).
		// Only requeue if we are not going in exponential backoff due to error, or if we are not already re-queueing, or if the object has a deletion timestamp.
		logger.Info("Checking if to requeueing CK8sControlPlane")
		if reterr == nil && !res.Requeue && res.RequeueAfter <= 0 && kcp.DeletionTimestamp.IsZero() {
			logger.Info("Checking if to requeueing CK8sControlPlane for not ready status")

			if !conditions.IsTrue(kcp, string(controlplanev1.AvailableCondition)) {
				logger.Info("Requeueing CK8sControlPlane for not ready status", "requeueAfter", 20*time.Second)
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}
		}
	}()

	if !kcp.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		return r.reconcileDelete(ctx, cluster, kcp)
	} else {
		// Handle normal reconciliation loop.
		return r.reconcile(ctx, cluster, kcp)
	}
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
	ownedMachines := allMachines.Filter(collections.OwnedMachines(kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane").GroupKind()))

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

	// NOTE: Aggregate machine conditions are not set here because conditions.SetAggregate
	// is not available with the current conditions utility API.

	// Verify that only control plane machines remain
	if len(allMachines) != len(ownedMachines) {
		logger.Info("Waiting for worker nodes to be deleted first")
		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.ResizedCondition),
			Status:  metav1.ConditionFalse,
			Reason:  "WaitingForWorkerDeletion",
			Message: fmt.Sprintf("Waiting for worker nodes to be deleted first, %d remaining", len(allMachines)-len(ownedMachines)),
		})
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Delete control plane machines in parallel
	machinesToDelete := ownedMachines.Filter(collections.Not(collections.HasDeletionTimestamp))
	var errs []error
	for i := range machinesToDelete {
		m := machinesToDelete[i]
		logger := logger.WithValues("machine", m)
		if err := r.Delete(ctx, machinesToDelete[i]); err != nil && !apierrors.IsNotFound(err) {
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
	conditions.Set(kcp, metav1.Condition{
		Type:    string(controlplanev1.ResizedCondition),
		Status:  metav1.ConditionFalse,
		Reason:  "DeletingControlPlaneMachines",
		Message: fmt.Sprintf("Deleting control plane machines, %d remaining", len(ownedMachines)-len(machinesToDelete)),
	})
	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

func patchCK8sControlPlane(ctx context.Context, patchHelper *patch.Helper, kcp *controlplanev1.CK8sControlPlane) error {
	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		kcp,
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			string(controlplanev1.MachinesReadyCondition),
			string(controlplanev1.MachinesSpecUpToDateCondition),
			string(controlplanev1.ResizedCondition),
			string(controlplanev1.AvailableCondition),
			string(controlplanev1.CertificatesAvailableCondition),
			string(controlplanev1.TokenAvailableCondition),
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
	if controlPlaneRef.Name != "" && controlPlaneRef.Kind == "CK8sControlPlane" {
		return []ctrl.Request{{
			NamespacedName: client.ObjectKey{
				Namespace: c.Namespace,
				Name:      controlPlaneRef.Name,
			},
		}}
	}

	return nil
}
func setReplicas(_ context.Context, kcp *controlplanev1.CK8sControlPlane, machines collections.Machines) {
	var readyReplicas, availableReplicas, upToDateReplicas int32
	for _, machine := range machines {
		if conditions.IsTrue(machine, clusterv1.MachineReadyCondition) {
			readyReplicas++
		}
		if conditions.IsTrue(machine, clusterv1.MachineAvailableCondition) {
			availableReplicas++
		}
		if conditions.IsTrue(machine, clusterv1.MachineUpToDateCondition) {
			upToDateReplicas++
		}
	}

	kcp.Status.Replicas = int32(len(machines))
	kcp.Status.ReadyReplicas = ptr.To(readyReplicas)
	kcp.Status.AvailableReplicas = ptr.To(availableReplicas)
	kcp.Status.UpToDateReplicas = ptr.To(upToDateReplicas)
}

// updateStatus is called after every reconcilitation loop in a defer statement to always make sure we have the
// resource status subresourcs up-to-date.
func (r *CK8sControlPlaneReconciler) updateStatus(ctx context.Context, kcp *controlplanev1.CK8sControlPlane, cluster *clusterv1.Cluster) error {
	selector := collections.ControlPlaneSelectorForCluster(cluster.Name)
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	kcp.Status.Selector = selector.String()

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, util.ObjectKey(cluster), collections.OwnedMachines(kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane").GroupKind()))
	if err != nil {
		return fmt.Errorf("failed to get list of owned machines: %w", err)
	}

	logger := r.Log.WithValues("namespace", kcp.Namespace, "CK8sControlPlane", kcp.Name, "cluster", cluster.Name)
	controlPlane, err := ck8s.NewControlPlane(ctx, r.Client, cluster, kcp, ownedMachines)
	if err != nil {
		logger.Error(err, "failed to initialize control plane")
		return err
	}
	replicas := int32(len(ownedMachines))
	desiredReplicas := *kcp.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	kcp.Status.Replicas = replicas

	lowestVersion := ownedMachines.LowestVersion()
	if lowestVersion != "" {
		kcp.Status.Version = &lowestVersion
	}

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !kcp.DeletionTimestamp.IsZero() {
		return nil
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.ResizedCondition),
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.ScalingUpReason,
			Message: fmt.Sprintf("Scaling up control plane from %d to %d replicas", replicas, desiredReplicas),
		})
	// We are scaling down
	case replicas > desiredReplicas:
		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.ResizedCondition),
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.ScalingDownReason,
			Message: fmt.Sprintf("Scaling down control plane from %d to %d replicas", replicas, desiredReplicas),
		})
	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		readyMachines := ownedMachines.Filter(collections.IsReady())
		if int32(len(readyMachines)) == replicas {
			conditions.Set(kcp, metav1.Condition{
				Type:    string(controlplanev1.ResizedCondition),
				Status:  metav1.ConditionTrue,
				Reason:  "ScalingCompleted",
				Message: "Successfully resized control plane",
			})
		}
	}

	microclusterPort := kcp.Spec.CK8sConfigSpec.ControlPlaneConfig.GetMicroclusterPort()
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster), microclusterPort)
	if err != nil {
		return fmt.Errorf("failed to create remote cluster client: %w", err)
	}
	status, err := workloadCluster.ClusterStatus(ctx)
	if err != nil {
		return err
	}

	logger.Info("ClusterStatus", "workload", status)
	setReplicas(ctx, kcp, ownedMachines)
	setInitializedCondition(ctx, kcp)
	enableDefaultNetwork := kcp.Spec.CK8sConfigSpec.InitConfig.GetEnableDefaultNetwork()

	// NOTE(neoaggelos): We consider the control plane to be initialized if the k8sd-config exists.
	// When enableDefaultNetwork is false (user-managed CNI), k8sd-config may not appear until CNI
	// is installed, so we fall back to API-server accessibility (ClusterStatus succeeded + replicas
	// exist) to break the initialization deadlock and allow the MAAS controller to proceed.
	if status.HasK8sdConfigMap || (!enableDefaultNetwork && replicas > 0) {
		kcp.Status.Initialization.ControlPlaneInitialized = ptr.To(true)
	}

	// When default network is disabled, nodes remain NotReady until the external CNI is
	// installed by user. Mark the control plane Ready/Available as soon as
	// it is initialized so that ControlPlaneInitializedCondition propagates to the Cluster object
	// and the CAPI ClusterCacheTracker can establish a remote connection.
	// Nodes will transition to Ready once CNI is applied, at which point ReadyReplicas > 0 and
	// the normal path also satisfies this condition.
	if kcp.Status.ReadyReplicas != nil && *kcp.Status.ReadyReplicas > 0 ||
		(!enableDefaultNetwork && kcp.Status.Initialization.ControlPlaneInitialized != nil && *kcp.Status.Initialization.ControlPlaneInitialized && replicas > 0) {
		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.AvailableCondition),
			Status:  metav1.ConditionTrue,
			Reason:  "Available",
			Message: "",
		})
	}

	// Surface lastRemediation data in status.
	// LastRemediation is the remediation currently in progress, in any, or the
	// most recent of the remediation we are keeping track on machines.
	var lastRemediation *RemediationData

	if v, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		remediationData, err := RemediationDataFromAnnotation(v)
		if err != nil {
			return err
		}
		lastRemediation = remediationData
	} else {
		for _, m := range controlPlane.Machines.UnsortedList() {
			if v, ok := m.Annotations[controlplanev1.RemediationForAnnotation]; ok {
				remediationData, err := RemediationDataFromAnnotation(v)
				if err != nil {
					return err
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

	return nil
}

func setInitializedCondition(_ context.Context, kcp *controlplanev1.CK8sControlPlane) {
	if ptr.Deref(kcp.Status.Initialization.ControlPlaneInitialized, false) {
		conditions.Set(kcp, metav1.Condition{
			Type:   clusterv1.ClusterControlPlaneInitializedCondition,
			Status: metav1.ConditionTrue,
			Reason: clusterv1.ClusterControlPlaneInitializedReason,
		})
		return
	}

	conditions.Set(kcp, metav1.Condition{
		Type:   clusterv1.ClusterControlPlaneInitializedCondition,
		Status: metav1.ConditionFalse,
		Reason: clusterv1.ClusterControlPlaneNotInitializedReason,
	})
}

// reconcile handles CK8sControlPlane reconciliation.
func (r *CK8sControlPlaneReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.CK8sControlPlane) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", kcp.Namespace, "CK8sControlPlane", kcp.Name, "cluster", cluster.Name)
	logger.Info("Reconcile CK8sControlPlane")

	// Make sure to reconcile the external infrastructure reference.
	if err := r.reconcileExternalReference(ctx, cluster, &kcp.Spec.MachineTemplate.InfrastructureRef); err != nil {
		return reconcile.Result{}, err
	}

	certificates := secret.NewCertificatesForInitialControlPlane(&kcp.Spec.CK8sConfigSpec)
	controllerRef := metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane"))
	if err := certificates.LookupOrGenerate(ctx, r.Client, util.ObjectKey(cluster), *controllerRef); err != nil {
		logger.Error(err, "unable to lookup or create cluster certificates")
		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.CertificatesAvailableCondition),
			Status:  metav1.ConditionFalse,
			Reason:  string(controlplanev1.CertificatesGenerationFailedReason),
			Message: "Failed to lookup or create cluster certificates",
		})
		return reconcile.Result{}, err
	}
	conditions.Set(kcp, metav1.Condition{
		Type:    string(controlplanev1.CertificatesAvailableCondition),
		Status:  metav1.ConditionTrue,
		Reason:  "CertificatesGenerated",
		Message: "Successfully looked up or created cluster certificates",
	})

	if err := token.Reconcile(ctx, r.Client, client.ObjectKeyFromObject(cluster), kcp); err != nil {
		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.TokenAvailableCondition),
			Status:  metav1.ConditionFalse,
			Reason:  string(controlplanev1.TokenGenerationFailedReason),
			Message: "Failed to lookup or create cluster tokens",
		})
		return reconcile.Result{}, err
	}
	conditions.Set(kcp, metav1.Condition{
		Type:    string(controlplanev1.TokenAvailableCondition),
		Status:  metav1.ConditionTrue,
		Reason:  "TokenGenerated",
		Message: "Successfully looked up or created cluster tokens",
	})

	// If ControlPlaneEndpoint is not set, requeue to wait for it to be set.
	// (berkayoz): This change to requeue instead of returning is to ensure
	// intermittent reconcile skips such as the one that happens in `Workload cluster scaling` tests
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		logger.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return reconcile.Result{RequeueAfter: 3 * time.Second}, nil
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

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane").GroupKind()))
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
		return reconcile.Result{}, fmt.Errorf("failed to sync Machines: %w", err)
	}

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

		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.MachinesSpecUpToDateCondition),
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.RollingUpdateInProgressReason,
			Message: fmt.Sprintf("Rolling out %d control plane machines", len(needRollout)),
		})
		return r.upgradeControlPlane(ctx, cluster, kcp, controlPlane, needRollout)
	default:
		// make sure last upgrade operation is marked as completed.
		// NOTE: we are checking the condition already exists in order to avoid to set this condition at the first
		// reconciliation/before a rolling upgrade actually starts.
		if conditions.Has(controlPlane.KCP, string(controlplanev1.MachinesSpecUpToDateCondition)) {
			conditions.Set(kcp, metav1.Condition{
				Type:    string(controlplanev1.MachinesSpecUpToDateCondition),
				Status:  metav1.ConditionTrue,
				Reason:  "RollingUpdateCompleted",
				Message: "Successfully rolled out all control plane machines",
			})
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

		conditions.Set(kcp, metav1.Condition{
			Type:    string(controlplanev1.AvailableCondition),
			Status:  metav1.ConditionFalse,
			Reason:  controlplanev1.WaitingForCK8sServerReason,
			Message: "Initializing control plane",
		})
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

func (r *CK8sControlPlaneReconciler) reconcileExternalReference(ctx context.Context, cluster *clusterv1.Cluster, ref *corev1.ObjectReference) error {
	if !strings.HasSuffix(ref.Kind, clusterv1.TemplateSuffix) {
		return nil
	}

	logger := r.Log.WithValues("namespace", ref.Namespace, "CK8sControlPlane", ref.Name, "cluster", cluster.Name)
	logger.Info("Reconciling external template reference", "ref", ref)

	// Ensure the ref namespace is populated for objects not yet defaulted by webhook
	// https://github.com/kubernetes-sigs/cluster-api/pull/11361
	if ref.Namespace == "" {
		ref = ref.DeepCopy()
		ref.Namespace = cluster.Namespace
	}

	obj, err := external.Get(ctx, r.Client, ref)
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
	if !util.IsControlledBy(configSecret, kcp, controlplanev1.GroupVersion.WithKind("CK8sControlPlane").GroupKind()) {
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
	if controlPlane.KCP.Status.Initialization.ControlPlaneInitialized == nil || !*controlPlane.KCP.Status.Initialization.ControlPlaneInitialized {
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
	logger := r.Log.WithValues("namespace", kcp.Namespace, "CK8sControlPlane", kcp.Name, "cluster", cluster.Name)

	if kcp.Spec.RolloutStrategy == nil {
		logger.Info("RolloutStrategy is empty, unable to continue")
		return ctrl.Result{}, nil
	}

	maxNodes := *kcp.Spec.Replicas + int32(kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntValue())
	if int32(controlPlane.Machines.Len()) < maxNodes {
		// scaleUp ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
		return r.scaleUpControlPlane(ctx, cluster, kcp, controlPlane)
	}
	return r.scaleDownControlPlane(ctx, cluster, kcp, controlPlane, machinesRequireUpgrade)
}
