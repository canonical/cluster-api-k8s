package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
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
	_, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Machine{}).
		Build(r)
	if err != nil {
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
	return err
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

	// Check if the machine has an in-place upgrade annotation
	if upgradeOption, ok := mAnnotations[bootstrapv1.InPlaceUpgradeToAnnotation]; ok {
		log.Info("Found in-place upgrade annotation", "upgrade-option", upgradeOption)

		// Lookup the cluster the machine belongs to
		cluster, err := util.GetClusterByName(ctx, r.Client, m.Namespace, m.Spec.ClusterName)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Get the workload cluster for the machine
		workloadCluster, err := r.getWorkloadClusterForMachine(ctx, util.ObjectKey(cluster), m)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get workload cluster for machine")
		}

		// Lookup the node token for the machine
		nodeToken, err := token.LookupNodeToken(ctx, r.Client, util.ObjectKey(cluster), m.Name)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to lookup node token")
		}

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create patch helper for machine")
		}

		upgradeStatus, hasUpgradeStatusAnnotation := mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation]

		refreshID, hasRefreshIDAnnotation := mAnnotations[bootstrapv1.InPlaceUpgradeRefreshIDAnnotation]

		if hasUpgradeStatusAnnotation && hasRefreshIDAnnotation {
			switch upgradeStatus {
			case bootstrapv1.InPlaceUpgradeInProgressStatus:

				status, err := workloadCluster.GetRefreshStatusForMachine(ctx, m, nodeToken, &refreshID)
				if err != nil {
					log.Info("Failed to get refresh status for machine", "error", err)
					return ctrl.Result{}, errors.Wrapf(err, "failed to get refresh status for machine")
				}

				if !status.Completed {
					log.Info("In-place upgrade still in progress, requeuing...")
					return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
				}

				switch status.Status {
				case "Done":
					log.Info("In-place upgrade completed successfully")
					if err := r.markUpgradeDone(ctx, m, upgradeOption); err != nil {
						return ctrl.Result{}, errors.Wrapf(err, "failed to mark in place upgrade status")
					}
				case "Error":
					log.Info("In-place upgrade failed", "error", status.ErrorMessage)
					if err := r.markUpgradeFailed(ctx, m, upgradeOption, status.ErrorMessage); err != nil {
						return ctrl.Result{}, errors.Wrapf(err, "failed to mark in place upgrade status")
					}
				default:
					log.Info("Found invalid refresh status, marking as failed")
					if err := r.markUpgradeFailed(ctx, m, upgradeOption, "invalid refresh status"); err != nil {
						return ctrl.Result{}, errors.Wrapf(err, "failed to mark in place upgrade status")
					}
				}
			case bootstrapv1.InPlaceUpgradeDoneStatus:
				delete(mAnnotations, bootstrapv1.InPlaceUpgradeToAnnotation)
				delete(mAnnotations, bootstrapv1.InPlaceUpgradeRefreshIDAnnotation)
				mAnnotations[bootstrapv1.InPlaceUpgradeReleaseAnnotation] = upgradeOption
				m.SetAnnotations(mAnnotations)
				if err := patchHelper.Patch(ctx, m); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "failed to patch machine annotations")
				}
			case bootstrapv1.InPlaceUpgradeFailedStatus:
				delete(mAnnotations, bootstrapv1.InPlaceUpgradeStatusAnnotation)
				delete(mAnnotations, bootstrapv1.InPlaceUpgradeRefreshIDAnnotation)
				m.SetAnnotations(mAnnotations)
				if err := patchHelper.Patch(ctx, m); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "failed to patch machine annotations")
				}
			default:
				log.Info("Found invalid in-place upgrade status, marking as failed")
				if err := r.markUpgradeFailed(ctx, m, upgradeOption, "invalid in-place upgrade status"); err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "failed to mark in place upgrade status")
				}
			}
		} else {
			// Handle the in-place upgrade request
			delete(mAnnotations, bootstrapv1.InPlaceUpgradeStatusAnnotation)
			delete(mAnnotations, bootstrapv1.InPlaceUpgradeRefreshIDAnnotation)
			delete(mAnnotations, bootstrapv1.InPlaceUpgradeReleaseAnnotation)
			m.SetAnnotations(mAnnotations)
			if err := patchHelper.Patch(ctx, m); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to patch machine annotations")
			}

			// Perform the in-place upgrade through snap refresh
			changeID, err := workloadCluster.RefreshMachine(ctx, m, nodeToken, &upgradeOption)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to refresh machine: %w", err)
			}

			// Set in place upgrade status to in progress
			if err := r.markUpgradeInProgress(ctx, m, upgradeOption, changeID); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to mark in place upgrade status")
			}
		}
	}

	return ctrl.Result{}, nil
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

func (r *InPlaceUpgradeReconciler) markUpgradeInProgress(ctx context.Context, m *clusterv1.Machine, upgradeOption string, changeID string) error {
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for machine")
	}
	mAnnotations := m.GetAnnotations()
	mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeInProgressStatus
	mAnnotations[bootstrapv1.InPlaceUpgradeRefreshIDAnnotation] = changeID
	m.SetAnnotations(mAnnotations)
	if err := patchHelper.Patch(ctx, m); err != nil {
		return errors.Wrapf(err, "failed to patch machine annotations")
	}
	r.recorder.Eventf(m, corev1.EventTypeNormal, bootstrapv1.InPlaceUpgradeInProgressEvent, "Performing in place upgrade with %s", upgradeOption)
	return nil
}

func (r *InPlaceUpgradeReconciler) markUpgradeDone(ctx context.Context, m *clusterv1.Machine, upgradeOption string) error {
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for machine")
	}
	mAnnotations := m.GetAnnotations()
	mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeDoneStatus
	m.SetAnnotations(mAnnotations)
	if err := patchHelper.Patch(ctx, m); err != nil {
		return errors.Wrapf(err, "failed to patch machine annotations")
	}
	r.recorder.Eventf(m, corev1.EventTypeNormal, bootstrapv1.InPlaceUpgradeDoneEvent, "Successfully performed in place upgrade with %s", upgradeOption)
	return nil
}

func (r *InPlaceUpgradeReconciler) markUpgradeFailed(ctx context.Context, m *clusterv1.Machine, upgradeOption string, message string) error {
	patchHelper, err := patch.NewHelper(m, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to create patch helper for machine")
	}
	mAnnotations := m.GetAnnotations()
	mAnnotations[bootstrapv1.InPlaceUpgradeStatusAnnotation] = bootstrapv1.InPlaceUpgradeFailedStatus
	m.SetAnnotations(mAnnotations)
	if err := patchHelper.Patch(ctx, m); err != nil {
		return errors.Wrapf(err, "failed to patch machine annotations")
	}
	r.recorder.Eventf(m, corev1.EventTypeWarning, bootstrapv1.InPlaceUpgradeFailedEvent, "Failed to perform in place upgrade with %s: %s", upgradeOption, message)
	return nil
}
