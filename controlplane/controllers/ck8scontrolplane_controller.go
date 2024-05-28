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
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
)

// CK8sControlPlaneReconciler reconciles a CK8sControlPlane object.
type CK8sControlPlaneReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	controller controller.Controller
	recorder   record.EventRecorder
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

func (r *CK8sControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.Namespace, "ck8sControlPlane", req.Name)
	logger.Info("Reconcile CK8sControlPlaneReconciler")

	return reconcile.Result{}, nil
}

func (r *CK8sControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, log *logr.Logger) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.CK8sControlPlane{}).
		Owns(&clusterv1.Machine{}).
		//	WithOptions(options).
		//	WithEventFilter(predicates.ResourceNotPaused(r.Log)).
		Build(r)
	if err != nil {
		return fmt.Errorf("failed setting up with a controller manager: %w", err)
	}

	err = c.Watch(
		source.Kind(mgr.GetCache(), &clusterv1.Cluster{}),
		handler.EnqueueRequestsFromMapFunc(r.ClusterToCK8sControlPlane(ctx, log)),
		predicates.ClusterUnpausedAndInfrastructureReady(r.Log),
	)
	if err != nil {
		return fmt.Errorf("failed adding Watch for Clusters to controller manager: %w", err)
	}

	r.Scheme = mgr.GetScheme()
	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("ck8s-control-plane-controller")

	return nil
}

// ClusterToCK8sControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for CK8sControlPlane based on updates to a Cluster.
func (r *CK8sControlPlaneReconciler) ClusterToCK8sControlPlane(ctx context.Context, log *logr.Logger) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []ctrl.Request {
		c, ok := o.(*clusterv1.Cluster)
		if !ok {
			r.Log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o))
			return nil
		}

		controlPlaneRef := c.Spec.ControlPlaneRef
		if controlPlaneRef != nil && controlPlaneRef.Kind == "CK8sControlPlane" {
			return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
		}

		return nil
	}
}
