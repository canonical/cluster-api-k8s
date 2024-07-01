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

	// BootCommands specifies extra commands to run in cloud-init early in the boot process.
	// +optional
	BootCommands []string `json:"bootCommands,omitempty"`

	// PreRunCommands specifies extra commands to run in cloud-init before k8s-snap setup runs.
	// +optional
	PreRunCommands []string `json:"preRunCommands,omitempty"`

	// PostRunCommands specifies extra commands to run in cloud-init after k8s-snap setup runs.
	// +optional
	PostRunCommands []string `json:"postRunCommands,omitempty"`

	// AirGapped is used to signal that we are deploying to an airgap environment. In this case,
	// the provider will not attempt to install k8s-snap on the machine. The user is expected to
	// install k8s-snap manually with preRunCommands, or provide an image with k8s-snap pre-installed.
	// +optional
	AirGapped bool `json:"airGapped,omitempty"`

	// CK8sControlPlaneConfig is configuration for the control plane node.
	// +optional
	ControlPlaneConfig CK8sControlPlaneConfig `json:"controlPlane,omitempty"`

	// CK8sInitConfig is configuration for the initializing the cluster features.
	// +optional
	InitConfig CK8sInitConfiguration `json:"initConfig,omitempty"`
}

// TODO
// Will need extend this func when implementing other database options.
func (c *CK8sConfigSpec) IsEtcdManaged() bool {
	return true
}

// CK8sControlPlaneConfig is configuration for control plane nodes.
type CK8sControlPlaneConfig struct {
	// ExtraSANs is a list of SANs to include in the server certificates.
	// +optional
	ExtraSANs []string `json:"extraSANs,omitempty"`

	// CloudProvider is the cloud-provider configuration option to set.
	// +optional
	CloudProvider string `json:"cloudProvider,omitempty"`

	// NodeTaints is taints to add to the control plane kubelet nodes.
	// +optional
	NodeTaints []string `json:"nodeTaints,omitempty"`

	// K8sDqlitePort is the port to use for k8s-dqlite. If unset, 2379 (etcd) will be used.
	// +optional
	K8sDqlitePort int `json:"k8sDqlitePort,omitempty"`

	// MicroclusterAddress is the address (or CIDR) to use for microcluster. If unset, the default node interface is chosen.
	MicroclusterAddress string `json:"microclusterAddress,omitempty"`

	// MicroclusterPort is the port to use for microcluster. If unset, ":2380" (etcd peer) will be used.
	// +optional
	MicroclusterPort int `json:"microclusterPort,omitempty"`

	// ExtraKubeAPIServerArgs is extra arguments to add to kube-apiserver.
	// +optional
	ExtraKubeAPIServerArgs map[string]*string `json:"extraKubeAPIServerArgs,omitempty"`
}

// CK8sInitConfiguration is configuration for the initializing the cluster features.
type CK8sInitConfiguration struct {
	// Annotations is a map of annotations to add to the control plane node.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// EnableDefaultDNS specifies whether to enable the default DNS configuration.
	// +optional
	EnableDefaultDNS *bool `json:"enableDefaultDNS,omitempty"`

	// EnableDefaultLocalStorage specifies whether to enable the default local storage.
	// +optional
	EnableDefaultLocalStorage *bool `json:"enableDefaultLocalStorage,omitempty"`

	// EnableDefaultMetricsServer specifies whether to enable the default metrics server.
	// +optional
	EnableDefaultMetricsServer *bool `json:"enableDefaultMetricsServer,omitempty"`

	// EnableDefaultNetwork specifies whether to enable the default CNI.
	// +optional
	EnableDefaultNetwork *bool `json:"enableDefaultNetwork,omitempty"`
}

// GetEnableDefaultNetwork returns the EnableDefaultNetwork field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultDNS() bool {
	if c.EnableDefaultDNS == nil {
		return true
	}
	return *c.EnableDefaultDNS
}

// GetEnableDefaultLocalStorage returns the EnableDefaultLocalStorage field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultLocalStorage() bool {
	if c.EnableDefaultLocalStorage == nil {
		return true
	}
	return *c.EnableDefaultLocalStorage
}

// GetEnableDefaultMetricsServer returns the EnableDefaultMetricsServer field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultMetricsServer() bool {
	if c.EnableDefaultMetricsServer == nil {
		return true
	}
	return *c.EnableDefaultMetricsServer
}

// GetEnableDefaultNetwork returns the EnableDefaultNetwork field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultNetwork() bool {
	if c.EnableDefaultNetwork == nil {
		return true
	}
	return *c.EnableDefaultNetwork
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
