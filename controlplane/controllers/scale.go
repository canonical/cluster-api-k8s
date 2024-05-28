/*
Copyright 2020 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"

	bootstrapv1 "github.com/k3s-io/cluster-api-k3s/bootstrap/api/v1beta2"
	controlplanev1 "github.com/k3s-io/cluster-api-k3s/controlplane/api/v1beta2"
	k3s "github.com/k3s-io/cluster-api-k3s/pkg/k3s"
)

var ErrPreConditionFailed = errors.New("precondition check failed")

func (r *KThreesControlPlaneReconciler) initializeControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KThreesControlPlane, controlPlane *k3s.ControlPlane) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Perform an uncached read of all the owned machines. This check is in place to make sure
	// that the controller cache is not misbehaving and we end up initializing the cluster more than once.
	ownedMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, util.ObjectKey(cluster), collections.OwnedMachines(kcp))
	if err != nil {
		logger.Error(err, "failed to perform an uncached read of control plane machines for cluster")
		return ctrl.Result{}, err
	}
	if len(ownedMachines) > 0 {
		return ctrl.Result{}, fmt.Errorf(
			"control plane has already been initialized, found %d owned machine for cluster %s/%s: controller cache or management cluster is misbehaving. %w",
			len(ownedMachines), cluster.Namespace, cluster.Name, err,
		)
	}

	bootstrapSpec := controlPlane.InitialControlPlaneConfig()
	fd := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create initial control plane Machine")
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedInitialization", "Failed to create initial control plane Machine for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *KThreesControlPlaneReconciler) scaleUpControlPlane(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KThreesControlPlane, controlPlane *k3s.ControlPlane) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Run preflight checks to ensure that the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	if result, err := r.preflightChecks(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Create the bootstrap configuration
	bootstrapSpec := controlPlane.JoinControlPlaneConfig()
	fd := controlPlane.NextFailureDomainForScaleUp(ctx)
	if err := r.cloneConfigsAndGenerateMachine(ctx, cluster, kcp, bootstrapSpec, fd); err != nil {
		logger.Error(err, "Failed to create additional control plane Machine")
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedScaleUp", "Failed to create additional control plane Machine for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are other operations to perform
	return ctrl.Result{Requeue: true}, nil
}

func (r *KThreesControlPlaneReconciler) scaleDownControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.KThreesControlPlane,
	controlPlane *k3s.ControlPlane,
	outdatedMachines collections.Machines,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	// Pick the Machine that we should scale down.
	machineToDelete, err := selectMachineForScaleDown(ctx, controlPlane, outdatedMachines)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to select machine for scale down: %w", err)
	}

	// Run preflight checks ensuring the control plane is stable before proceeding with a scale up/scale down operation; if not, wait.
	// Given that we're scaling down, we can exclude the machineToDelete from the preflight checks.
	if result, err := r.preflightChecks(ctx, controlPlane, machineToDelete); err != nil || !result.IsZero() {
		return result, err
	}

	if machineToDelete == nil {
		logger.Info("Failed to pick control plane Machine to delete")
		return ctrl.Result{}, fmt.Errorf("failed to pick control plane Machine to delete: %w", err)
	}

	// If KCP should manage etcd, If etcd leadership is on machine that is about to be deleted, move it to the newest member available.
	if controlPlane.IsEtcdManaged() {
		workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
		if err != nil {
			logger.Error(err, "Failed to create client to workload cluster")
			return ctrl.Result{}, fmt.Errorf("failed to create client to workload cluster: %w", err)
		}

		etcdLeaderCandidate := controlPlane.Machines.Newest()
		if err := workloadCluster.ForwardEtcdLeadership(ctx, machineToDelete, etcdLeaderCandidate); err != nil {
			logger.Error(err, "Failed to move leadership to candidate machine", "candidate", etcdLeaderCandidate.Name)
			return ctrl.Result{}, err
		}

		patchHelper, err := patch.NewHelper(machineToDelete, r.Client)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create patch helper for machine")
		}

		mAnnotations := machineToDelete.GetAnnotations()
		mAnnotations[clusterv1.PreTerminateDeleteHookAnnotationPrefix] = k3sHookName
		machineToDelete.SetAnnotations(mAnnotations)

		if err := patchHelper.Patch(ctx, machineToDelete); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed patch machine for adding preTerminate hook")
		}
	}

	logger = logger.WithValues("machine", machineToDelete)
	if err := r.Client.Delete(ctx, machineToDelete); err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to delete control plane machine")
		r.recorder.Eventf(kcp, corev1.EventTypeWarning, "FailedScaleDown",
			"Failed to delete control plane Machine %s for cluster %s/%s control plane: %v", machineToDelete.Name, cluster.Namespace, cluster.Name, err)
		return ctrl.Result{}, err
	}

	// Requeue the control plane, in case there are additional operations to perform
	return ctrl.Result{Requeue: true}, nil
}

// preflightChecks checks if the control plane is stable before proceeding with a scale up/scale down operation,
// where stable means that:
// - There are no machine deletion in progress
// - All the health conditions on KCP are true.
// - All the health conditions on the control plane machines are true.
// If the control plane is not passing preflight checks, it requeue.
//
// NOTE: this func uses KCP conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *KThreesControlPlaneReconciler) preflightChecks(_ context.Context, controlPlane *k3s.ControlPlane, excludeFor ...*clusterv1.Machine) (ctrl.Result, error) { //nolint:unparam
	logger := r.Log.WithValues("namespace", controlPlane.KCP.Namespace, "KThreesControlPlane", controlPlane.KCP.Name, "cluster", controlPlane.Cluster.Name)

	// If there is no KCP-owned control-plane machines, then control-plane has not been initialized yet,
	// so it is considered ok to proceed.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}, nil
	}

	// If there are deleting machines, wait for the operation to complete.
	if controlPlane.HasDeletingMachine() {
		logger.Info("Waiting for machines to be deleted", "Machines", strings.Join(controlPlane.Machines.Filter(collections.HasDeletionTimestamp).Names(), ", "))
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Check machine health conditions; if there are conditions with False or Unknown, then wait.
	allMachineHealthConditions := []clusterv1.ConditionType{controlplanev1.MachineAgentHealthyCondition}
	if controlPlane.IsEtcdManaged() {
		allMachineHealthConditions = append(allMachineHealthConditions,
			controlplanev1.MachineEtcdMemberHealthyCondition,
		)
	}

	machineErrors := []error{}

loopmachines:
	for _, machine := range controlPlane.Machines {
		for _, excluded := range excludeFor {
			// If this machine should be excluded from the individual
			// health check, continue the out loop.
			if machine.Name == excluded.Name {
				continue loopmachines
			}
		}

		for _, condition := range allMachineHealthConditions {
			if err := preflightCheckCondition("machine", machine, condition); err != nil {
				machineErrors = append(machineErrors, err)
			}
		}
	}

	if len(machineErrors) > 0 {
		aggregatedError := kerrors.NewAggregate(machineErrors)
		r.recorder.Eventf(controlPlane.KCP, corev1.EventTypeWarning, "ControlPlaneUnhealthy",
			"Waiting for control plane to pass preflight checks to continue reconciliation: %v", aggregatedError)
		logger.Info("Waiting for control plane to pass preflight checks", "failures", aggregatedError.Error())

		return ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

func preflightCheckCondition(kind string, obj conditions.Getter, condition clusterv1.ConditionType) error {
	c := conditions.Get(obj, condition)
	if c == nil {
		return fmt.Errorf("%s %s does not have %s condition: %w", kind, obj.GetName(), condition, ErrPreConditionFailed)
	}
	if c.Status == corev1.ConditionFalse {
		return fmt.Errorf("%s %s reports %s condition is false (%s, %s): %w", kind, obj.GetName(), condition, c.Severity, c.Message, ErrPreConditionFailed)
	}
	if c.Status == corev1.ConditionUnknown {
		return fmt.Errorf("%s %s reports %s condition is unknown (%s): %w", kind, obj.GetName(), condition, c.Message, ErrPreConditionFailed)
	}

	return nil
}

func selectMachineForScaleDown(ctx context.Context, controlPlane *k3s.ControlPlane, outdatedMachines collections.Machines) (*clusterv1.Machine, error) {
	machines := controlPlane.Machines
	switch {
	case controlPlane.MachineWithDeleteAnnotation(outdatedMachines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(outdatedMachines)
	case controlPlane.MachineWithDeleteAnnotation(machines).Len() > 0:
		machines = controlPlane.MachineWithDeleteAnnotation(machines)
	case outdatedMachines.Len() > 0:
		machines = outdatedMachines
	}
	return controlPlane.MachineInFailureDomainWithMostMachines(ctx, machines)
}

func (r *KThreesControlPlaneReconciler) cloneConfigsAndGenerateMachine(ctx context.Context, cluster *clusterv1.Cluster, kcp *controlplanev1.KThreesControlPlane, bootstrapSpec *bootstrapv1.KThreesConfigSpec, failureDomain *string) error {
	var errs []error

	// Since the cloned resource should eventually have a controller ref for the Machine, we create an
	// OwnerReference here without the Controller field set
	infraCloneOwner := &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "KThreesControlPlane",
		Name:       kcp.Name,
		UID:        kcp.UID,
	}

	// Clone the infrastructure template
	infraRef, err := external.CreateFromTemplate(ctx, &external.CreateFromTemplateInput{
		Client:      r.Client,
		TemplateRef: &kcp.Spec.MachineTemplate.InfrastructureRef,
		Namespace:   kcp.Namespace,
		OwnerRef:    infraCloneOwner,
		ClusterName: cluster.Name,
		Labels:      k3s.ControlPlaneLabelsForCluster(cluster.Name, kcp.Spec.MachineTemplate),
	})
	if err != nil {
		// Safe to return early here since no resources have been created yet.
		return fmt.Errorf("failed to clone infrastructure template: %w", err)
	}

	// Clone the bootstrap configuration
	bootstrapRef, err := r.generateKThreesConfig(ctx, kcp, cluster, bootstrapSpec)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to generate bootstrap config: %w", err))
	}

	// Only proceed to generating the Machine if we haven't encountered an error
	if len(errs) == 0 {
		if err := r.generateMachine(ctx, kcp, cluster, infraRef, bootstrapRef, failureDomain); err != nil {
			errs = append(errs, fmt.Errorf("failed to create Machine: %w", err))
		}
	}

	// If we encountered any errors, attempt to clean up any dangling resources
	if len(errs) > 0 {
		if err := r.cleanupFromGeneration(ctx, infraRef, bootstrapRef); err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup generated resources: %w", err))
		}

		return kerrors.NewAggregate(errs)
	}

	return nil
}

func (r *KThreesControlPlaneReconciler) cleanupFromGeneration(ctx context.Context, remoteRefs ...*corev1.ObjectReference) error {
	var errs []error

	for _, ref := range remoteRefs {
		if ref != nil {
			config := &unstructured.Unstructured{}
			config.SetKind(ref.Kind)
			config.SetAPIVersion(ref.APIVersion)
			config.SetNamespace(ref.Namespace)
			config.SetName(ref.Name)

			if err := r.Client.Delete(ctx, config); err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, fmt.Errorf("failed to cleanup generated resources after error: %w", err))
			}
		}
	}

	return kerrors.NewAggregate(errs)
}

func (r *KThreesControlPlaneReconciler) generateKThreesConfig(ctx context.Context, kcp *controlplanev1.KThreesControlPlane, cluster *clusterv1.Cluster, spec *bootstrapv1.KThreesConfigSpec) (*corev1.ObjectReference, error) {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "KThreesControlPlane",
		Name:       kcp.Name,
		UID:        kcp.UID,
	}

	bootstrapConfig := &bootstrapv1.KThreesConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(kcp.Name + "-"),
			Namespace:       kcp.Namespace,
			Labels:          k3s.ControlPlaneLabelsForCluster(cluster.Name, kcp.Spec.MachineTemplate),
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}

	if err := r.Client.Create(ctx, bootstrapConfig); err != nil {
		return nil, fmt.Errorf("failed to create bootstrap configuration: %w", err)
	}

	bootstrapRef := &corev1.ObjectReference{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "KThreesConfig",
		Name:       bootstrapConfig.GetName(),
		Namespace:  bootstrapConfig.GetNamespace(),
		UID:        bootstrapConfig.GetUID(),
	}

	return bootstrapRef, nil
}

func (r *KThreesControlPlaneReconciler) generateMachine(ctx context.Context, kcp *controlplanev1.KThreesControlPlane, cluster *clusterv1.Cluster, infraRef, bootstrapRef *corev1.ObjectReference, failureDomain *string) error {
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SimpleNameGenerator.GenerateName(kcp.Name + "-"),
			Namespace: kcp.Namespace,
			Labels:    k3s.ControlPlaneLabelsForCluster(cluster.Name, kcp.Spec.MachineTemplate),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(kcp, controlplanev1.GroupVersion.WithKind("KThreesControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       cluster.Name,
			Version:           &kcp.Spec.Version,
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain:           failureDomain,
			NodeDrainTimeout:        kcp.Spec.MachineTemplate.NodeDrainTimeout,
			NodeVolumeDetachTimeout: kcp.Spec.MachineTemplate.NodeVolumeDetachTimeout,
			NodeDeletionTimeout:     kcp.Spec.MachineTemplate.NodeDeletionTimeout,
		},
	}

	annotations := map[string]string{}

	// Machine's bootstrap config may be missing ClusterConfiguration if it is not the first machine in the control plane.
	// We store ClusterConfiguration as annotation here to detect any changes in KCP ClusterConfiguration and rollout the machine if any.
	serverConfig, err := json.Marshal(kcp.Spec.KThreesConfigSpec.ServerConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal cluster configuration: %w", err)
	}
	annotations[controlplanev1.KThreesServerConfigurationAnnotation] = string(serverConfig)

	// In case this machine is being created as a consequence of a remediation, then add an annotation
	// tracking remediating data.
	// NOTE: This is required in order to track remediation retries.
	if remediationData, ok := kcp.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		annotations[controlplanev1.RemediationForAnnotation] = remediationData
	}

	machine.SetAnnotations(annotations)

	if err := r.Client.Create(ctx, machine); err != nil {
		return fmt.Errorf("failed to create machine: %w", err)
	}

	// Remove the annotation tracking that a remediation is in progress (the remediation completed when
	// the replacement machine has been created above).
	delete(kcp.Annotations, controlplanev1.RemediationInProgressAnnotation)
	return nil
}
