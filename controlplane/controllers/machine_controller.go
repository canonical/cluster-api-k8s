package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
)

// MachineReconciler reconciles a Machine object.
type MachineReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	K8sdDialTimeout time.Duration

	managementCluster ck8s.ManagementCluster
}

func (r *MachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, log *logr.Logger) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Machine{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event.CreateEvent) bool { return true },
			// UpdateFunc must stay unconditional: this is what fires when DeletionTimestamp
			// transitions from zero to set, as well as annotation/condition changes.
			UpdateFunc:  func(event.UpdateEvent) bool { return true },
			DeleteFunc:  func(event.DeleteEvent) bool { return true },
			GenericFunc: func(event.GenericEvent) bool { return true },
		}).
		Build(r)

	if r.managementCluster == nil {
		r.managementCluster = &ck8s.Management{
			Client:          r.Client,
			K8sdDialTimeout: r.K8sdDialTimeout,
		}
	}

	return err
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
func (r *MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "machine", req.Name)

	m := &clusterv1.Machine{}
	if err := r.Get(ctx, req.NamespacedName, m); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			logger.Info("node-remove-error: machine not found.")
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		logger.Info("node-remove-error: machine could not be retrieved.")
		return ctrl.Result{}, err
	}

	logger.Info("node-remove-info: machine gets deletion timestamp check")
	if m.DeletionTimestamp.IsZero() {
		logger.Info("node-remove-info: machine does not have a deletion timestamp.")
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}
	if m.Status.Deletion != nil && m.Status.Deletion.WaitForNodeVolumeDetachStartTime.IsZero() {
		logger.Info("node-remove-wait: m.Status.Deletion.WaitForNodeVolumeDetachStartTime IsZero")
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}
	c := conditions.Get(m, clusterv1.MachineDeletingCondition)
	if c == nil {
		logger.Info("node-remove-wait: clusterv1.MachineDeletingCondition is not set")
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}
	if c.Status != metav1.ConditionTrue {
		logger.Info("node-remove-wait: clusterv1.MachineDeletingCondition condition is not true")
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}
	cluster := &clusterv1.Cluster{}
	errCluster := r.Get(ctx, client.ObjectKey{
		Namespace: m.Namespace,
		Name:      m.Labels["cluster.x-k8s.io/cluster-name"],
	}, cluster)
	if errCluster != nil {
		logger.Info("node-remove-error: owner cluster could not be retrieved.")
		return ctrl.Result{}, errCluster
	}
	microclusterPort := 2380
	clusterObjectKey := util.ObjectKey(cluster)
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, clusterObjectKey, microclusterPort)
	if err != nil {
		logger.Info("node-remove-error: failed to create client to workload cluster")
		return ctrl.Result{}, fmt.Errorf("failed to create client to workload cluster: %w", err)
	}

	if c.Reason != clusterv1.MachineDeletingWaitingForPreTerminateHookReason {
		logger.Info("node-remove-wait: clusterv1.MachineDeletingCondition does not have clusterv1.MachineDeletingWaitingForPreTerminateHookReason", "current reason", c.Reason)
		if c.Reason == clusterv1.MachineDeletingWaitingForInfrastructureDeletionReason || c.Reason == clusterv1.MachineDeletingWaitingForBootstrapDeletionReason || c.Reason == clusterv1.MachineDeletingDeletionCompletedReason {
			logger.Info("node-ready-for-cluster-removal: ready to be re-removed from cluster")
			if err := workloadCluster.RemoveMachineFromCluster(ctx, m); err != nil {
				logger.Error(err, "failed to remove machine from microcluster")
				return ctrl.Result{}, fmt.Errorf("failed to remove machine from microcluster: %w", err)
			}
			logger.Info("node-ready-for-cluster-removal: re-removed from cluster")
		}
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}

	if v1beta1conditions.IsFalse(m, clusterv1.DrainingSucceededV1Beta1Condition) {
		logger.Info("node-remove-wait: wait for machine drain to complete - using v1beta1conditions.")
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}
	if v1beta1conditions.IsFalse(m, clusterv1.VolumeDetachSucceededV1Beta1Condition) {
		logger.Info("node-remove-wait: wait for machine volume detachment to complete - using v1beta1conditions.")
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}
	if m.Status.NodeRef.Name != "" {
		node, err := workloadCluster.GetNode(ctx, m)
		if err != nil {
			logger.Info("node-remove-error: failed to get machine corresponding node")
		} else if len(node.Status.VolumesAttached) != 0 {
			logger.Info("node-remove-wait: there are still volumes attached.")
			return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
		}
	}

	logger.Info("node-ready-for-annotation-removal: machine gets annotation check")
	// if machine registered PreTerminate hook, wait for capi asks to resolve PreTerminateDeleteHook
	if annotations.HasWithPrefix(clusterv1.PreTerminateDeleteHookAnnotationPrefix, m.Annotations) &&
		m.Annotations[PreTerminateHookCleanupAnnotation] == ck8sHookName {
		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create patch helper for machine: %w", err)
		}
		trueStr := "true"
		mAnnotations := m.GetAnnotations()
		if mAnnotations[clusterv1.ExcludeNodeDrainingAnnotation] != trueStr {
			mAnnotations[clusterv1.ExcludeNodeDrainingAnnotation] = trueStr
			mAnnotations[clusterv1.ExcludeWaitForNodeVolumeDetachAnnotation] = trueStr
			m.SetAnnotations(mAnnotations)
			if err := patchHelper.Patch(ctx, m); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to patch machine: %w", err)
			}
			logger.Info("node-ready-for-cluster-removal: ready to be removed from cluster")
			if err := workloadCluster.RemoveMachineFromCluster(ctx, m); err != nil {
				logger.Error(err, "failed to remove machine from microcluster")
				return ctrl.Result{}, fmt.Errorf("failed to remove machine from microcluster: %w", err)
			}
			logger.Info("node-ready-for-cluster-removal: removed from cluster")
			return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
		}

		logger.Info("node-ready-for-cluster-removal: ready to be re-removed from cluster")
		if err := workloadCluster.RemoveMachineFromCluster(ctx, m); err != nil {
			logger.Error(err, "failed to re-remove machine from microcluster")
		}
		logger.Info("node-ready-for-cluster-removal: re-removed from cluster")
		delete(mAnnotations, PreTerminateHookCleanupAnnotation)
		logger.Info("node-ready-for-annotation-removal: removing the annotation PreTerminateDeleteHookAnnotationPrefix")
		m.SetAnnotations(mAnnotations)
		if err := patchHelper.Patch(ctx, m); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to patch machine: %w", err)
		}
		logger.Info("node-ready-for-annotation-removal: machine got annotations removed")
	}

	return ctrl.Result{}, nil
}
