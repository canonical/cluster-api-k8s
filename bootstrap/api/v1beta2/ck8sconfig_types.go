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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CK8sConfigSpec defines the desired state of CK8sConfig.
type CK8sConfigSpec struct {
	// Version specifies the Kubernetes version.
	// +optional
	Version string `json:"version,omitempty"`

	// Files specifies extra files to be passed to user_data upon creation.
	// +optional
	Files []File `json:"files,omitempty"`

	// PreRunCommands specifies extra commands to run in cloud-init before k8s-snap setup runs.
	// +optional
	PreRunCommands []string `json:"preRunCommands,omitempty"`

	// PostRunCommands specifies extra commands to run in cloud-init after k8s-snap setup runs.
	// +optional
	PostRunCommands []string `json:"postRunCommands,omitempty"`

	// AirGapped is a boolean option to signal that we are deploying to an airgap environment.
	// In this case, the provider assumes that it cannot download and install binaries, and the user
	// should ensure that all nodes have a `/opt/capi/install.sh` script that installs k8s-snap.
	// +optional
	AirGapped bool `json:"airGapped,omitempty"`

	// CK8sControlPlaneConfig is configuration for the control plane node.
	// +optional
	ControlPlaneConfig CK8sControlPlaneConfig `json:"controlPlane,omitempty"`
}

// TODO
// Will need extend this func when implementing other database options.
func (c *CK8sConfigSpec) IsK8sDqlite() bool {
	return true
}

// CK8sControlPlaneConfig is configuration for control plane noes.
type CK8sControlPlaneConfig struct {
	// ExtraSANs is a list of SANs to include in the server certificates.
	// +optional
	ExtraSANs []string `json:"extraSANs,omitempty"`
}

// CK8sConfigStatus defines the observed state of CK8sConfig.
type CK8sConfigStatus struct {
	// Ready indicates the BootstrapData field is ready to be consumed
	Ready bool `json:"ready,omitempty"`

	BootstrapData []byte `json:"bootstrapData,omitempty"`

	// DataSecretName is the name of the secret that stores the bootstrap data script.
	// +optional
	DataSecretName *string `json:"dataSecretName,omitempty"`

	// FailureReason will be set on non-retryable errors
	// +optional
	FailureReason string `json:"failureReason,omitempty"`

	// FailureMessage will be set on non-retryable errors
	// +optional
	FailureMessage string `json:"failureMessage,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the CK8sConfig.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// CK8sConfig is the Schema for the ck8sconfigs API.
type CK8sConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CK8sConfigSpec   `json:"spec,omitempty"`
	Status CK8sConfigStatus `json:"status,omitempty"`
}

func (c *CK8sConfig) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

func (c *CK8sConfig) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// CK8sConfigList contains a list of CK8sConfig.
type CK8sConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CK8sConfig `json:"items"`
}

// Encoding specifies the cloud-init file encoding.
// +kubebuilder:validation:Enum=base64;gzip;gzip+base64
type Encoding string

const (
	// Base64 implies the contents of the file are encoded as base64.
	Base64 Encoding = "base64"
	// Gzip implies the contents of the file are encoded with gzip.
	Gzip Encoding = "gzip"
	// GzipBase64 implies the contents of the file are first base64 encoded and then gzip encoded.
	GzipBase64 Encoding = "gzip+base64"
)

// File defines the input for generating write_files in cloud-init.
type File struct {
	// Path specifies the full path on disk where to store the file.
	Path string `json:"path"`

	// Owner specifies the ownership of the file, e.g. "root:root".
	// +optional
	Owner string `json:"owner,omitempty"`

	// Permissions specifies the permissions to assign to the file, e.g. "0640".
	// +optional
	Permissions string `json:"permissions,omitempty"`

	// Encoding specifies the encoding of the file contents.
	// +optional
	Encoding Encoding `json:"encoding,omitempty"`

	// Content is the actual content of the file.
	// +optional
	Content string `json:"content,omitempty"`

	// ContentFrom is a referenced source of content to populate the file.
	// +optional
	ContentFrom *FileSource `json:"contentFrom,omitempty"`
}

// FileSource is a union of all possible external source types for file data.
// Only one field may be populated in any given instance. Developers adding new
// sources of data for target systems should add them here.
type FileSource struct {
	// Secret represents a secret that should populate this file.
	Secret SecretFileSource `json:"secret"`
}

// Adapts a Secret into a FileSource.
//
// The contents of the target Secret's Data field will be presented
// as files using the keys in the Data field as the file names.
type SecretFileSource struct {
	// Name of the secret in the CK8sBootstrapConfig's namespace to use.
	Name string `json:"name"`

	// Key is the key in the secret's data map for this value.
	Key string `json:"key"`
}

func init() {
	SchemeBuilder.Register(&CK8sConfig{}, &CK8sConfigList{})
}
