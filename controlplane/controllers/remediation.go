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
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/collections"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"

	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta2"
	"github.com/canonical/cluster-api-k8s/pkg/ck8s"
)

// reconcileUnhealthyMachines tries to remediate CK8sControlPlane unhealthy machines
// based on the process described in https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20191017-kubeadm-based-control-plane.md#remediation-using-delete-and-recreate
// taken from the kubeadm codebase and adapted for the ck8s provider.
func (r *CK8sControlPlaneReconciler) reconcileUnhealthyMachines(ctx context.Context, controlPlane *ck8s.ControlPlane) (ret ctrl.Result, retErr error) {
	log := ctrl.LoggerFrom(ctx)
	reconciliationTime := time.Now().UTC()

	// Cleanup pending remediation actions not completed for any reasons (e.g. number of current replicas is less or equal to 1)
	// if the underlying machine is now back to healthy / not deleting.
	errList := []error{}
	healthyMachines := controlPlane.HealthyMachines()
	for _, m := range healthyMachines {
		if conditions.IsTrue(m, clusterv1.MachineHealthCheckSucceededCondition) &&
			conditions.IsFalse(m, clusterv1.MachineOwnerRemediatedCondition) &&
			m.DeletionTimestamp.IsZero() {
			patchHelper, err := patch.NewHelper(m, r.Client)
			if err != nil {
				errList = append(errList, fmt.Errorf("failed to get PatchHelper for machine %s: %w", m.Name, err))
				continue
			}

			conditions.Delete(m, clusterv1.MachineOwnerRemediatedCondition)

			if err := patchHelper.Patch(ctx, m, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				clusterv1.MachineOwnerRemediatedCondition,
			}}); err != nil {
				errList = append(errList, fmt.Errorf("failed to patch machine %s: %w", m.Name, err))
			}
		}
	}
	if len(errList) > 0 {
		return ctrl.Result{}, kerrors.NewAggregate(errList)
	}

	// Gets all machines that have `MachineHealthCheckSucceeded=False` (indicating a problem was detected on the machine)
	// and `MachineOwnerRemediated` present, indicating that this controller is responsible for performing remediation.
	unhealthyMachines := controlPlane.UnhealthyMachines()

	// If there are no unhealthy machines, return so KCP can proceed with other operations (ctrl.Result nil).
	if len(unhealthyMachines) == 0 {
		return ctrl.Result{}, nil
	}

	// Select the machine to be remediated, which is the oldest machine marked as unhealthy not yet provisioned (if any)
	// or the oldest machine marked as unhealthy.
	//
	// NOTE: The current solution is considered acceptable for the most frequent use case (only one unhealthy machine),
	// however, in the future this could potentially be improved for the scenario where more than one unhealthy machine exists
	// by considering which machine has lower impact on etcd quorum.
	machineToBeRemediated := getMachineToBeRemediated(unhealthyMachines)

	// Returns if the machine is in the process of being deleted.
	if !machineToBeRemediated.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	log = log.WithValues("Machine", klog.KObj(machineToBeRemediated), "initialized", controlPlane.KCP.Status.Initialized)

	// Returns if another remediation is in progress but the new Machine is not yet created.
	// Note: This condition is checked after we check for unhealthy Machines and if machineToBeRemediated
	// is being deleted to avoid unnecessary logs if no further remediation should be done.
	if _, ok := controlPlane.KCP.Annotations[controlplanev1.RemediationInProgressAnnotation]; ok {
		log.Info("Another remediation is already in progress. Skipping remediation.")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(machineToBeRemediated, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		// Always attempt to Patch the Machine conditions after each reconcileUnhealthyMachines.
		if err := patchHelper.Patch(ctx, machineToBeRemediated, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.MachineOwnerRemediatedCondition,
		}}); err != nil {
			log.Error(err, "Failed to patch control plane Machine", "Machine", machineToBeRemediated.Name)
			if retErr == nil {
				retErr = fmt.Errorf("failed to patch control plane Machine %s: %w", machineToBeRemediated.Name, err)
			}
		}
	}()

	// Before starting remediation, run preflight checks in order to verify it is safe to remediate.
	// If any of the following checks fails, we'll surface the reason in the MachineOwnerRemediated condition.

	// Check if KCP is allowed to remediate considering retry limits:
	// - Remediation cannot happen because retryPeriod is not yet expired.
	// - KCP already reached MaxRetries limit.
	remediationInProgressData, canRemediate, err := r.checkRetryLimits(log, machineToBeRemediated, controlPlane, reconciliationTime)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !canRemediate {
		// NOTE: log lines and conditions surfacing why it is not possible to remediate are set by checkRetryLimits.
		return ctrl.Result{}, nil
	}

	if controlPlane.KCP.Status.Initialized {
		// Executes checks that apply only if the control plane is already initialized; in this case KCP can
		// remediate only if it can safely assume that the operation preserves the operation state of the
		// existing cluster (or at least it doesn't make it worse).

		// The cluster MUST have more than one replica, because this is the smallest cluster size that allows any etcd failure tolerance.
		if controlPlane.Machines.Len() <= 1 {
			log.Info("A control plane machine needs remediation, but the number of current replicas is less or equal to 1. Skipping remediation", "Replicas", controlPlane.Machines.Len())
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate if current replicas are less or equal to 1")
			return ctrl.Result{}, nil
		}

		// The cluster MUST NOT have healthy machines still being provisioned. This rule prevents KCP taking actions while the cluster is in a transitional state.
		if controlPlane.HasHealthyMachineStillProvisioning() {
			log.Info("A control plane machine needs remediation, but there are other control-plane machines being provisioned. Skipping remediation")
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP waiting for control plane machine provisioning to complete before triggering remediation")
			return ctrl.Result{}, nil
		}

		// The cluster MUST have no machines with a deletion timestamp. This rule prevents KCP taking actions while the cluster is in a transitional state.
		if controlPlane.HasDeletingMachine() {
			log.Info("A control plane machine needs remediation, but there are other control-plane machines being deleted. Skipping remediation")
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP waiting for control plane machine deletion to complete before triggering remediation")
			return ctrl.Result{}, nil
		}

		// NOTE(neoaggelos): etcd requires manual adjustment of the cluster nodes to always ensure that a quorum of healthy nodes are available,
		// so that the cluster does not lock and cause the cluster to go down. In the case of k8s-dqlite, this is automatically handled by the
		// go-dqlite layer, and Canonical Kubernetes has logic to automatically keep a quorum of nodes in normal operation.
		//
		// Therefore, we have removed this check for simplicity, but should remember that we need this precondition before proceeing.
	}

	microclusterPort := controlPlane.KCP.Spec.CK8sConfigSpec.ControlPlaneConfig.GetMicroclusterPort()
	clusterObjectKey := util.ObjectKey(controlPlane.Cluster)
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, clusterObjectKey, microclusterPort)
	if err != nil {
		log.Error(err, "failed to create client to workload cluster")
		return ctrl.Result{}, fmt.Errorf("failed to create client to workload cluster: %w", err)
	}

	if machineToBeRemediated.Status.NodeRef != nil {
		// TODO: If the node is not part of the microcluster, this may still return an error. We should catch that case,
		// and proceed with the machine removal.
		if err := workloadCluster.RemoveMachineFromCluster(ctx, machineToBeRemediated); err != nil {
			log.Error(err, "failed to remove machine from microcluster")
			return ctrl.Result{}, fmt.Errorf("failed to remove machine from microcluster: %w", err)
		}
	}

	// Delete the machine
	if err := r.Delete(ctx, machineToBeRemediated); err != nil {
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationFailedReason, clusterv1.ConditionSeverityError, "%s", err.Error())
		return ctrl.Result{}, fmt.Errorf("failed to delete unhealthy machine %s: %w", machineToBeRemediated.Name, err)
	}

	// Surface the operation is in progress.
	log.Info("Remediating unhealthy machine")
	conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.RemediationInProgressReason, clusterv1.ConditionSeverityWarning, "")

	// Prepare the info for tracking the remediation progress into the RemediationInProgressAnnotation.
	remediationInProgressValue, err := remediationInProgressData.Marshal()
	if err != nil {
		return ctrl.Result{}, err
	}

	// Set annotations tracking remediation details so they can be picked up by the machine
	// that will be created as part of the scale up action that completes the remediation.
	annotations.AddAnnotations(controlPlane.KCP, map[string]string{
		controlplanev1.RemediationInProgressAnnotation: remediationInProgressValue,
	})

	return ctrl.Result{Requeue: true}, nil
}

// Gets the machine to be remediated, which is the oldest machine marked as unhealthy not yet provisioned (if any)
// or the oldest machine marked as unhealthy.
func getMachineToBeRemediated(unhealthyMachines collections.Machines) *clusterv1.Machine {
	machineToBeRemediated := unhealthyMachines.Filter(collections.Not(collections.HasNode())).Oldest()
	if machineToBeRemediated == nil {
		machineToBeRemediated = unhealthyMachines.Oldest()
	}
	return machineToBeRemediated
}

// checkRetryLimits checks if KCP is allowed to remediate considering retry limits:
// - Remediation cannot happen because retryPeriod is not yet expired.
// - KCP already reached the maximum number of retries for a machine.
// NOTE: Counting the number of retries is required In order to prevent infinite remediation e.g. in case the
// first Control Plane machine is failing due to quota issue.
func (r *CK8sControlPlaneReconciler) checkRetryLimits(log logr.Logger, machineToBeRemediated *clusterv1.Machine, controlPlane *ck8s.ControlPlane, reconciliationTime time.Time) (*RemediationData, bool, error) {
	// Get last remediation info from the machine.
	var lastRemediationData *RemediationData
	if value, ok := machineToBeRemediated.Annotations[controlplanev1.RemediationForAnnotation]; ok {
		l, err := RemediationDataFromAnnotation(value)
		if err != nil {
			return nil, false, err
		}
		lastRemediationData = l
	}

	remediationInProgressData := &RemediationData{
		Machine:    machineToBeRemediated.Name,
		Timestamp:  metav1.Time{Time: reconciliationTime},
		RetryCount: 0,
	}

	// If there is no last remediation, this is the first try of a new retry sequence.
	if lastRemediationData == nil {
		return remediationInProgressData, true, nil
	}

	// Gets MinHealthyPeriod and RetryPeriod from the remediation strategy, or use defaults.
	minHealthyPeriod := controlplanev1.DefaultMinHealthyPeriod
	if controlPlane.KCP.Spec.RemediationStrategy != nil && controlPlane.KCP.Spec.RemediationStrategy.MinHealthyPeriod != nil {
		minHealthyPeriod = controlPlane.KCP.Spec.RemediationStrategy.MinHealthyPeriod.Duration
	}
	retryPeriod := time.Duration(0)
	if controlPlane.KCP.Spec.RemediationStrategy != nil {
		retryPeriod = controlPlane.KCP.Spec.RemediationStrategy.RetryPeriod.Duration
	}

	// Gets the timestamp of the last remediation; if missing, default to a value
	// that ensures both MinHealthyPeriod and RetryPeriod are expired.
	// NOTE: this could potentially lead to executing more retries than expected or to executing retries before than
	// expected, but this is considered acceptable when the system recovers from someone/something changes or deletes
	// the RemediationForAnnotation on Machines.
	lastRemediationTime := reconciliationTime.Add(-2 * maxDuration(minHealthyPeriod, retryPeriod))
	if !lastRemediationData.Timestamp.IsZero() {
		lastRemediationTime = lastRemediationData.Timestamp.Time
	}

	// Once we get here we already know that there was a last remediation for the Machine.
	// If the current remediation is happening before minHealthyPeriod is expired, then KCP considers this
	// as a remediation for the same previously unhealthy machine.
	// NOTE: If someone/something changes the RemediationForAnnotation on Machines (e.g. changes the Timestamp),
	// this could potentially lead to executing more retries than expected, but this is considered acceptable in such a case.
	var retryForSameMachineInProgress bool
	if lastRemediationTime.Add(minHealthyPeriod).After(reconciliationTime) {
		retryForSameMachineInProgress = true
		log = log.WithValues("RemediationRetryFor", klog.KRef(machineToBeRemediated.Namespace, lastRemediationData.Machine))
	}

	// If the retry for the same machine is not in progress, this is the first try of a new retry sequence.
	if !retryForSameMachineInProgress {
		return remediationInProgressData, true, nil
	}

	// If the remediation is for the same machine, carry over the retry count.
	remediationInProgressData.RetryCount = lastRemediationData.RetryCount

	// Check if remediation can happen because retryPeriod is passed.
	if lastRemediationTime.Add(retryPeriod).After(reconciliationTime) {
		log.Info(fmt.Sprintf("A control plane machine needs remediation, but the operation already failed in the latest %s. Skipping remediation", retryPeriod))
		conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate this machine because the operation already failed in the latest %s (RetryPeriod)", retryPeriod)
		return remediationInProgressData, false, nil
	}

	// Check if remediation can happen because of maxRetry is not reached yet, if defined.
	if controlPlane.KCP.Spec.RemediationStrategy != nil && controlPlane.KCP.Spec.RemediationStrategy.MaxRetry != nil {
		maxRetry := int(*controlPlane.KCP.Spec.RemediationStrategy.MaxRetry)
		if remediationInProgressData.RetryCount >= maxRetry {
			log.Info(fmt.Sprintf("A control plane machine needs remediation, but the operation already failed %d times (MaxRetry %d). Skipping remediation", remediationInProgressData.RetryCount, maxRetry))
			conditions.MarkFalse(machineToBeRemediated, clusterv1.MachineOwnerRemediatedCondition, clusterv1.WaitingForRemediationReason, clusterv1.ConditionSeverityWarning, "KCP can't remediate this machine because the operation already failed %d times (MaxRetry)", maxRetry)
			return remediationInProgressData, false, nil
		}
	}

	// All the check passed, increase the remediation retry count.
	remediationInProgressData.RetryCount++

	return remediationInProgressData, true, nil
}

// maxDuration returns the longer of two time.Duration values.
func maxDuration(x, y time.Duration) time.Duration {
	if x < y {
		return y
	}
	return x
}

// RemediationData struct is used to keep track of information stored in the RemediationInProgressAnnotation in KCP
// during remediation and then into the RemediationForAnnotation on the replacement machine once it is created.
type RemediationData struct {
	// Machine is the machine name of the latest machine being remediated.
	Machine string `json:"machine"`

	// Timestamp is when last remediation happened. It is represented in RFC3339 form and is in UTC.
	Timestamp metav1.Time `json:"timestamp"`

	// RetryCount used to keep track of remediation retry for the last remediated machine.
	// A retry happens when a machine that was created as a replacement for an unhealthy machine also fails.
	RetryCount int `json:"retryCount"`
}

// RemediationDataFromAnnotation gets RemediationData from an annotation value.
func RemediationDataFromAnnotation(value string) (*RemediationData, error) {
	ret := &RemediationData{}
	if err := json.Unmarshal([]byte(value), ret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value %s for %s annotation: %w", value, clusterv1.RemediationInProgressReason, err)
	}
	return ret, nil
}

// Marshal an RemediationData into an annotation value.
func (r *RemediationData) Marshal() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value for %s annotation: %w", clusterv1.RemediationInProgressReason, err)
	}
	return string(b), nil
}

// ToStatus converts a RemediationData into a LastRemediationStatus struct.
func (r *RemediationData) ToStatus() *controlplanev1.LastRemediationStatus {
	return &controlplanev1.LastRemediationStatus{
		Machine:    r.Machine,
		Timestamp:  r.Timestamp,
		RetryCount: int32(r.RetryCount),
	}
}
