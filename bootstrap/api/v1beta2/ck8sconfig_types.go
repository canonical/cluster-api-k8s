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

	// BootstrapConfig is the data to be passed to the bootstrap script.
	BootstrapConfig *BootstrapConfig `json:"bootstrapConfig,omitempty"`

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

	// The snap store proxy domain's scheme, e.g. "http" or "https" without "://"
	// Defaults to "http".
	// +optional
	// +kubebuilder:default=http
	// +kubebuilder:validation:Enum=http;https
	SnapstoreProxyScheme string `json:"snapstoreProxyScheme,omitempty"`

	// The snap store proxy domain
	// +optional
	SnapstoreProxyDomain string `json:"snapstoreProxyDomain,omitempty"`

	// The snap store proxy ID
	// +optional
	SnapstoreProxyID string `json:"snapstoreProxyId,omitempty"`

	// HTTPSProxy is optional https proxy configuration
	// +optional
	HTTPSProxy string `json:"httpsProxy,omitempty"`

	// HTTPProxy is optional http proxy configuration
	// +optional
	HTTPProxy string `json:"httpProxy,omitempty"`

	// NoProxy is optional no proxy configuration
	// +optional
	NoProxy string `json:"noProxy,omitempty"`

	// Channel is the channel to use for the snap install.
	// +optional
	Channel string `json:"channel,omitempty"`

	// Revision is the revision to use for the snap install.
	// If Channel is set, this will be ignored.
	// +optional
	Revision string `json:"revision,omitempty"`

	// LocalPath is the path of a local snap file in the workload cluster to use for the snap install.
	// If Channel or Revision are set, this will be ignored.
	// +optional
	LocalPath string `json:"localPath,omitempty"`

	// CK8sControlPlaneConfig is configuration for the control plane node.
	// +optional
	ControlPlaneConfig CK8sControlPlaneConfig `json:"controlPlane,omitempty"`

	// CK8sInitConfig is configuration for the initializing the cluster features.
	// +optional
	InitConfig CK8sInitConfiguration `json:"initConfig,omitempty"`

	// NodeName is the name to use for the kubelet of this node. It is needed for clouds
	// where the cloud-provider has specific pre-requisites about the node names. It is
	// typically set in Jinja template form, e.g."{{ ds.meta_data.local_hostname }}".
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// ExtraKubeProxyArgs - extra arguments to add to kube-proxy.
	// +optional
	ExtraKubeProxyArgs map[string]*string `json:"extraKubeProxyArgs,omitempty"`

	// ExtraKubeletArgs - extra arguments to add to kubelet.
	// +optional
	ExtraKubeletArgs map[string]*string `json:"extraKubeletArgs,omitempty"`

	// ExtraContainerdArgs - extra arguments to add to containerd.
	// +optional
	ExtraContainerdArgs map[string]*string `json:"extraContainerdArgs,omitempty"`

	// ExtraK8sAPIServerProxyArgs - extra arguments to add to k8s-api-server-proxy.
	// +optional
	ExtraK8sAPIServerProxyArgs map[string]*string `json:"ExtraK8sAPIServerProxyArgs,omitempty"`
}

// IsEtcdManaged returns true if the control plane is using k8s-dqlite.
func (c *CK8sConfigSpec) IsEtcdManaged() bool {
	switch c.ControlPlaneConfig.DatastoreType {
	case "", "k8s-dqlite":
		return true
	}
	return false
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

	// DatastoreType is the type of datastore to use for the control plane.
	// +optional
	DatastoreType string `json:"datastoreType,omitempty"`

	// DatastoreServersSecretRef is a reference to a secret containing the datastore servers.
	// +optional
	DatastoreServersSecretRef SecretRef `json:"datastoreServersSecretRef,omitempty"`

	// K8sDqlitePort is the port to use for k8s-dqlite. If unset, 2379 (etcd) will be used.
	// +optional
	K8sDqlitePort int `json:"k8sDqlitePort,omitempty"`

	// MicroclusterAddress is the address (or CIDR) to use for microcluster. If unset, the default node interface is chosen.
	MicroclusterAddress string `json:"microclusterAddress,omitempty"`

	// MicroclusterPort is the port to use for microcluster. If unset, ":2380" (etcd peer) will be used.
	// +optional
	MicroclusterPort *int `json:"microclusterPort,omitempty"`

	// ExtraKubeAPIServerArgs - extra arguments to add to kube-apiserver.
	// +optional
	ExtraKubeAPIServerArgs map[string]*string `json:"extraKubeAPIServerArgs,omitempty"`

	// ExtraKubeControllerManagerArgs - extra arguments to add to kube-controller-manager.
	// +optional
	ExtraKubeControllerManagerArgs map[string]*string `json:"extraKubeControllerManagerArgs,omitempty"`

	// ExtraKubeSchedulerArgs - extra arguments to add to kube-scheduler.
	// +optional
	ExtraKubeSchedulerArgs map[string]*string `json:"extraKubeSchedulerArgs,omitempty"`

	// ExtraK8sDqliteArgs - extra arguments to add to k8s-dqlite.
	// +optional
	ExtraK8sDqliteArgs map[string]*string `json:"ExtraK8sDqliteArgs,omitempty"`
}

// GetMicroclusterPort returns the port to use for microcluster.
// If unset, 2380 (etcd peer) will be used.
func (c *CK8sControlPlaneConfig) GetMicroclusterPort() int {
	if c.MicroclusterPort == nil {
		return 2380
	}
	return *c.MicroclusterPort
}

// CK8sInitConfiguration is configuration for the initializing the cluster features.
type CK8sInitConfiguration struct {
	// Annotations are used to configure the behaviour of the built-in features.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// EnableDefaultDNS specifies whether to enable the default DNS configuration.
	// +optional
	EnableDefaultDNS *bool `json:"enableDefaultDNS,omitempty"`

	// EnableDefaultLoadBalancer specifies whether to enable the default LoadBalancer configuration.
	// +optional
	EnableDefaultLoadBalancer *bool `json:"enableDefaultLoadBalancer,omitempty"`

	// EnableDefaultGateway specifies whether to enable the default Gateway configuration.
	// +optional
	EnableDefaultGateway *bool `json:"enableDefaultGateway,omitempty"`

	// EnableDefaultIngress specifies whether to enable the default Ingress configuration.
	// +optional
	EnableDefaultIngress *bool `json:"enableDefaultIngress,omitempty"`

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

// GetEnableDefaultDNS returns the EnableDefaultDNS field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultDNS() bool {
	if c.EnableDefaultDNS == nil {
		return true
	}
	return *c.EnableDefaultDNS
}

// GetEnableDefaultLoadBalancer returns the EnableDefaultLoadBalancer field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultLoadBalancer() bool {
	if c.EnableDefaultLoadBalancer == nil {
		return true
	}
	return *c.EnableDefaultLoadBalancer
}

// GetEnableDefaultGateway returns the EnableDefaultGateway field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultGateway() bool {
	if c.EnableDefaultGateway == nil {
		return true
	}
	return *c.EnableDefaultGateway
}

// GetEnableDefaultIngress returns the EnableDefaultIngress field.
// If the field is not set, it returns true.
func (c *CK8sInitConfiguration) GetEnableDefaultIngress() bool {
	if c.EnableDefaultIngress == nil {
		return true
	}
	return *c.EnableDefaultIngress
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

type BootstrapConfig struct {
	// Content is the actual content of the file.
	// If this is set, ContentFrom is ignored.
	// +optional
	Content string `json:"content,omitempty"`

	// ContentFrom is a referenced source of content to populate the file.
	// +optional
	ContentFrom *FileSource `json:"contentFrom,omitempty"`
}

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

// SecretRef is a reference to a secret in the CK8sBootstrapConfig's namespace.
type SecretRef struct {
	// Name of the secret in the CK8sBootstrapConfig's namespace to use.
	Name string `json:"name"`

	// Key is the key in the secret's data map for this value.
	// +optional
	Key string `json:"key,omitempty"`
}

func init() {
	SchemeBuilder.Register(&CK8sConfig{}, &CK8sConfigList{})
}
