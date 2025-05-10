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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bootstrapv1beta2 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
)

// CK8sControlPlaneTemplateSpec defines the desired state of CK8sControlPlaneTemplateSpec.
type CK8sControlPlaneTemplateSpec struct {
	Template CK8sControlPlaneTemplateResource `json:"template"`
}

type CK8sControlPlaneTemplateResource struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta metav1.ObjectMeta                    `json:"metadata,omitempty"`
	Spec       CK8sControlPlaneTemplateResourceSpec `json:"spec"`
}

type CK8sControlPlaneTemplateResourceSpec struct {
	// CK8sConfigSpec is a CK8sConfigSpec
	// to use for initializing and joining machines to the control plane.
	// +optional
	CK8sConfigSpec bootstrapv1beta2.CK8sConfigSpec `json:"spec,omitempty"`

	// RolloutAfter is a field to indicate an rollout should be performed
	// after the specified time even if no changes have been made to the
	// CK8sControlPlane
	// +optional
	RolloutAfter *metav1.Time `json:"rolloutAfter,omitempty"`

	// MachineTemplate contains information about how machines should be shaped
	// when creating or updating a control plane.
	MachineTemplate CK8sControlPlaneMachineTemplate `json:"machineTemplate,omitempty"`

	// The RemediationStrategy that controls how control plane machine remediation happens.
	// +optional
	RemediationStrategy *RemediationStrategy `json:"remediationStrategy,omitempty"`

	// rolloutStrategy is the RolloutStrategy to use to replace control plane machines with
	// new ones.
	// +optional
	// +kubebuilder:default={rollingUpdate: {maxSurge: 1}}
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// CK8sControlPlaneTemplate is the Schema for the ck8scontrolplanetemplate API.
type CK8sControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CK8sControlPlaneTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// CK8sControlPlaneTemplateList contains a list of CK8sControlPlaneTemplate.
type CK8sControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CK8sControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CK8sControlPlaneTemplate{}, &CK8sControlPlaneTemplateList{})
}
