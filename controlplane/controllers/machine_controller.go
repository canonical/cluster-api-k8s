package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CK8sControlPlaneReconciler reconciles a CK8sControlPlane object.
type MachineReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	EtcdDialTimeout time.Duration
	EtcdCallTimeout time.Duration

	// NOTE(neoaggelos): See note below
	/**
	managementCluster ck8s.ManagementCluster
	**/
}

func (r *MachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, log *logr.Logger) error {
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Machine{}).
		Build(r)

	// NOTE(neoaggelos): See note below
	/**
	if r.managementCluster == nil {
		r.managementCluster = &ck8s.Management{
			Client:          r.Client,
			EtcdDialTimeout: r.EtcdDialTimeout,
			EtcdCallTimeout: r.EtcdCallTimeout,
		}
	}
	**/

	return err
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
func (r *MachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "machine", req.Name)

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

	if m.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// if machine registered PreTerminate hook, wait for capi asks to resolve PreTerminateDeleteHook
	if annotations.HasWithPrefix(clusterv1.PreTerminateDeleteHookAnnotationPrefix, m.ObjectMeta.Annotations) &&
		m.ObjectMeta.Annotations[clusterv1.PreTerminateDeleteHookAnnotationPrefix] == ck8sHookName {
		if !conditions.IsFalse(m, clusterv1.PreTerminateDeleteHookSucceededCondition) {
			logger.Info("wait for machine drain and detech volume operation complete.")
			return ctrl.Result{}, nil
		}

		// NOTE(neoaggelos): The upstream control plane provider adds the annotation "clusterv1.PreTerminateDeleteHookAnnotationPrefix"
		// to machines that are getting deleted.
		//
		// This happens in two scenarios:
		// - scale.go: The control plane is getting scaled down
		// - remediation.go: New control plane machines are getting rolled out to replace ones with outdated config.
		//
		// In the case of upstream, these machines are still part of the etcd cluster. The reconcile loop has already ensured that they
		// have transferred their leadership role (if they were the leader).
		//
		// In the case of Canonical Kubernetes, the node removal happens by executing the k8sd RemoveNode endpoint, which takes care of
		// removing the node from the datastore quorum as well. Therefore, we should not need to do any more actions in this case. It should
		// suffice to simply delete the annotation.
		//
		// Note that this currently makes the annotation a no-op in the code here. However, we still keep the logic in the code is case it
		// is needed in the future.

		/**
		cluster, err := util.GetClusterFromMetadata(ctx, r.Client, m.ObjectMeta)
		if err != nil {
			logger.Info("unable to get cluster.")
			return ctrl.Result{}, errors.Wrapf(err, "unable to get cluster")
		}

		workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
		if err != nil {
			logger.Error(err, "failed to create client to workload cluster")
			return ctrl.Result{}, errors.Wrapf(err, "failed to create client to workload cluster")
		}

		etcdRemoved, err := workloadCluster.RemoveEtcdMemberForMachine(ctx, m)
		if err != nil {
			logger.Error(err, "failed to remove etcd member for machine")
			return ctrl.Result{}, err
		}
		if !etcdRemoved {
			logger.Info("wait embedded etcd controller to remove etcd")
			return ctrl.Result{Requeue: true}, err
		}

		// It is possible that the machine has no machine ref yet, will record the machine name in log
		logger.Info("etcd remove etcd member succeeded", "machine name", m.Name)
		**/

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create patch helper for machine")
		}

		mAnnotations := m.GetAnnotations()
		delete(mAnnotations, clusterv1.PreTerminateDeleteHookAnnotationPrefix)
		m.SetAnnotations(mAnnotations)
		if err := patchHelper.Patch(ctx, m); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed patch machine")
		}
	}

	return ctrl.Result{}, nil
}
