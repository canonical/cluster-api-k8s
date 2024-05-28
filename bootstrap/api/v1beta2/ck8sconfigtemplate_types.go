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
)

// CK8sConfigTemplateSpec defines the desired state of CK8sConfigTemplate.
type CK8sConfigTemplateSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	Template CK8sConfigTemplateResource `json:"template"`
}

// CK8sConfigTemplateResource defines the Template structure.
type CK8sConfigTemplateResource struct {
	Spec CK8sConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

// CK8sConfigTemplate is the Schema for the ck8sconfigtemplates API.
type CK8sConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec CK8sConfigTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// CK8sConfigTemplateList contains a list of CK8sConfigTemplate.
type CK8sConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CK8sConfigTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CK8sConfigTemplate{}, &CK8sConfigTemplateList{})
}
