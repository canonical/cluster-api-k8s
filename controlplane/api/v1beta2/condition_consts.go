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

package v1beta2

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

// Conditions and condition Reasons for the KThreesControlPlane object.

const (
	// MachinesReady reports an aggregate of current status of the machines controlled by the KThreesControlPlane.
	MachinesReadyCondition clusterv1.ConditionType = "MachinesReady"
)

const (
	// CertificatesAvailableCondition documents that cluster certificates were generated as part of the
	// processing of a KThreesControlPlane object.
	CertificatesAvailableCondition clusterv1.ConditionType = "CertificatesAvailable"

	// CertificatesGenerationFailedReason (Severity=Warning) documents a KThreesControlPlane controller detecting
	// an error while generating certificates; those kind of errors are usually temporary and the controller
	// automatically recover from them.
	CertificatesGenerationFailedReason = "CertificatesGenerationFailed"
)

const (
	// AvailableCondition documents that the first control plane instance has completed the server init operation
	// and so the control plane is available and an API server instance is ready for processing requests.
	AvailableCondition clusterv1.ConditionType = "Available"

	// WaitingForKthreesServerReason (Severity=Info) documents a KThreesControlPlane object waiting for the first
	// control plane instance to complete the k3s server operation.
	WaitingForKthreesServerReason = "WaitingForKthreesServer"
)

const (
	// MachinesSpecUpToDateCondition documents that the spec of the machines controlled by the KThreesControlPlane
	// is up to date. Whe this condition is false, the KThreesControlPlane is executing a rolling upgrade.
	MachinesSpecUpToDateCondition clusterv1.ConditionType = "MachinesSpecUpToDate"

	// RollingUpdateInProgressReason (Severity=Warning) documents a KThreesControlPlane object executing a
	// rolling upgrade for aligning the machines spec to the desired state.
	RollingUpdateInProgressReason = "RollingUpdateInProgress"
)

const (
	// ResizedCondition documents a KThreesControlPlane that is resizing the set of controlled machines.
	ResizedCondition clusterv1.ConditionType = "Resized"

	// ScalingUpReason (Severity=Info) documents a KThreesControlPlane that is increasing the number of replicas.
	ScalingUpReason = "ScalingUp"

	// ScalingDownReason (Severity=Info) documents a KThreesControlPlane that is decreasing the number of replicas.
	ScalingDownReason = "ScalingDown"
)

const (
	// ControlPlaneComponentsHealthyCondition reports the overall status of the k3s server.
	ControlPlaneComponentsHealthyCondition clusterv1.ConditionType = "ControlPlaneComponentsHealthy"

	// ControlPlaneComponentsUnhealthyReason (Severity=Error) documents a control plane component not healthy.
	ControlPlaneComponentsUnhealthyReason = "ControlPlaneComponentsUnhealthy"

	// ControlPlaneComponentsUnknownReason reports a control plane component in unknown status.
	ControlPlaneComponentsUnknownReason = "ControlPlaneComponentsUnknown"

	// ControlPlaneComponentsInspectionFailedReason documents a failure in inspecting the control plane component status.
	ControlPlaneComponentsInspectionFailedReason = "ControlPlaneComponentsInspectionFailed"

	// MachineAgentHealthyCondition reports a machine's k3s agent's operational status.
	MachineAgentHealthyCondition clusterv1.ConditionType = "AgentHealthy"

	// PodProvisioningReason (Severity=Info) documents a pod waiting to be provisioned i.e., Pod is in "Pending" phase.
	PodProvisioningReason = "PodProvisioning"

	// PodMissingReason (Severity=Error) documents a pod does not exist.
	PodMissingReason = "PodMissing"

	// PodFailedReason (Severity=Error) documents if a pod failed during provisioning i.e., e.g CrashLoopbackOff, ImagePullBackOff
	// or if all the containers in a pod have terminated.
	PodFailedReason = "PodFailed"

	// PodInspectionFailedReason documents a failure in inspecting the pod status.
	PodInspectionFailedReason = "PodInspectionFailed"
)

const (
	// EtcdClusterHealthyCondition documents the overall etcd cluster's health.
	EtcdClusterHealthyCondition clusterv1.ConditionType = "EtcdClusterHealthyCondition"

	// EtcdClusterInspectionFailedReason documents a failure in inspecting the etcd cluster status.
	EtcdClusterInspectionFailedReason = "EtcdClusterInspectionFailed"

	// EtcdClusterUnknownReason reports an etcd cluster in unknown status.
	EtcdClusterUnknownReason = "EtcdClusterUnknown"

	// EtcdClusterUnhealthyReason (Severity=Error) is set when the etcd cluster is unhealthy.
	EtcdClusterUnhealthyReason = "EtcdClusterUnhealthy"

	// MachineEtcdMemberHealthyCondition report the machine's etcd member's health status.
	// NOTE: This conditions exists only if a stacked etcd cluster is used.
	MachineEtcdMemberHealthyCondition clusterv1.ConditionType = "EtcdMemberHealthy"

	// EtcdMemberInspectionFailedReason documents a failure in inspecting the etcd member status.
	EtcdMemberInspectionFailedReason = "MemberInspectionFailed"

	// EtcdMemberUnhealthyReason (Severity=Error) documents a Machine's etcd member is unhealthy.
	EtcdMemberUnhealthyReason = "EtcdMemberUnhealthy"
)

const (
	// TokenAvailableCondition documents whether the token required for nodes to join the cluster is available.
	TokenAvailableCondition clusterv1.ConditionType = "TokenAvailable"

	// TokenGenerationFailedReason documents that the token required for nodes to join the cluster could not be generated.
	TokenGenerationFailedReason = "TokenGenerationFailed"
)
