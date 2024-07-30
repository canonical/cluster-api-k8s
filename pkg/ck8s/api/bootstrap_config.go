package apiv1

// BootstrapConfig is copied from https://github.com/canonical/k8s-snap/tree/main/src/k8s/api/v1/bootstrap_config.go#L9
type BootstrapConfig struct {
	// ClusterConfig
	ClusterConfig UserFacingClusterConfig `json:"cluster-config,omitempty" yaml:"cluster-config,omitempty"`

	// Seed configuration for the control plane (flat on purpose). Empty values are ignored
	ControlPlaneTaints  []string `json:"control-plane-taints,omitempty" yaml:"control-plane-taints,omitempty"`
	PodCIDR             *string  `json:"pod-cidr,omitempty" yaml:"pod-cidr,omitempty"`
	ServiceCIDR         *string  `json:"service-cidr,omitempty" yaml:"service-cidr,omitempty"`
	DisableRBAC         *bool    `json:"disable-rbac,omitempty" yaml:"disable-rbac,omitempty"`
	SecurePort          *int     `json:"secure-port,omitempty" yaml:"secure-port,omitempty"`
	K8sDqlitePort       *int     `json:"k8s-dqlite-port,omitempty" yaml:"k8s-dqlite-port,omitempty"`
	DatastoreType       *string  `json:"datastore-type,omitempty" yaml:"datastore-type,omitempty"`
	DatastoreServers    []string `json:"datastore-servers,omitempty" yaml:"datastore-servers,omitempty"`
	DatastoreCACert     *string  `json:"datastore-ca-crt,omitempty" yaml:"datastore-ca-crt,omitempty"`
	DatastoreClientCert *string  `json:"datastore-client-crt,omitempty" yaml:"datastore-client-crt,omitempty"`
	DatastoreClientKey  *string  `json:"datastore-client-key,omitempty" yaml:"datastore-client-key,omitempty"`
	ShouldRemoveK8sNode *bool    `json:"should-remove-k8s-node,omitempty" yaml:"should-remove-k8s-node,omitempty"`

	// Seed configuration for certificates
	ExtraSANs []string `json:"extra-sans,omitempty" yaml:"extra-sans,omitempty"`

	// Seed configuration for external certificates (cluster-wide)
	CACert                          *string `json:"ca-crt,omitempty" yaml:"ca-crt,omitempty"`
	CAKey                           *string `json:"ca-key,omitempty" yaml:"ca-key,omitempty"`
	ClientCACert                    *string `json:"client-ca-crt,omitempty" yaml:"client-ca-crt,omitempty"`
	ClientCAKey                     *string `json:"client-ca-key,omitempty" yaml:"client-ca-key,omitempty"`
	FrontProxyCACert                *string `json:"front-proxy-ca-crt,omitempty" yaml:"front-proxy-ca-crt,omitempty"`
	FrontProxyCAKey                 *string `json:"front-proxy-ca-key,omitempty" yaml:"front-proxy-ca-key,omitempty"`
	FrontProxyClientCert            *string `json:"front-proxy-client-crt,omitempty" yaml:"front-proxy-client-crt,omitempty"`
	FrontProxyClientKey             *string `json:"front-proxy-client-key,omitempty" yaml:"front-proxy-client-key,omitempty"`
	APIServerKubeletClientCert      *string `json:"apiserver-kubelet-client-crt,omitempty" yaml:"apiserver-kubelet-client-crt,omitempty"`
	APIServerKubeletClientKey       *string `json:"apiserver-kubelet-client-key,omitempty" yaml:"apiserver-kubelet-client-key,omitempty"`
	AdminClientCert                 *string `json:"admin-client-crt,omitempty" yaml:"admin-client-crt,omitempty"`
	AdminClientKey                  *string `json:"admin-client-key,omitempty" yaml:"admin-client-key,omitempty"`
	KubeProxyClientCert             *string `json:"kube-proxy-client-crt,omitempty" yaml:"kube-proxy-client-crt,omitempty"`
	KubeProxyClientKey              *string `json:"kube-proxy-client-key,omitempty" yaml:"kube-proxy-client-key,omitempty"`
	KubeSchedulerClientCert         *string `json:"kube-scheduler-client-crt,omitempty" yaml:"kube-scheduler-client-crt,omitempty"`
	KubeSchedulerClientKey          *string `json:"kube-scheduler-client-key,omitempty" yaml:"kube-scheduler-client-key,omitempty"`
	KubeControllerManagerClientCert *string `json:"kube-controller-manager-client-crt,omitempty" yaml:"kube-controller-manager-client-crt,omitempty"`
	KubeControllerManagerClientKey  *string `json:"kube-controller-manager-client-key,omitempty" yaml:"kube-ControllerManager-client-key,omitempty"`
	ServiceAccountKey               *string `json:"service-account-key,omitempty" yaml:"service-account-key,omitempty"`

	// Seed configuration for external certificates (node-specific)
	APIServerCert     *string `json:"apiserver-crt,omitempty" yaml:"apiserver-crt,omitempty"`
	APIServerKey      *string `json:"apiserver-key,omitempty" yaml:"apiserver-key,omitempty"`
	KubeletCert       *string `json:"kubelet-crt,omitempty" yaml:"kubelet-crt,omitempty"`
	KubeletKey        *string `json:"kubelet-key,omitempty" yaml:"kubelet-key,omitempty"`
	KubeletClientCert *string `json:"kubelet-client-crt,omitempty" yaml:"kubelet-client-crt,omitempty"`
	KubeletClientKey  *string `json:"kubelet-client-key,omitempty" yaml:"kubelet-client-key,omitempty"`

	// ExtraNodeConfigFiles will be written to /var/snap/k8s/common/args/conf.d
	ExtraNodeConfigFiles map[string]string `json:"extra-node-config-files,omitempty" yaml:"extra-node-config-files,omitempty"`

	// Extra args to add to individual services (set any arg to null to delete)
	ExtraNodeKubeAPIServerArgs         map[string]*string `json:"extra-node-kube-apiserver-args,omitempty" yaml:"extra-node-kube-apiserver-args,omitempty"`
	ExtraNodeKubeControllerManagerArgs map[string]*string `json:"extra-node-kube-controller-manager-args,omitempty" yaml:"extra-node-kube-controller-manager-args,omitempty"`
	ExtraNodeKubeSchedulerArgs         map[string]*string `json:"extra-node-kube-scheduler-args,omitempty" yaml:"extra-node-kube-scheduler-args,omitempty"`
	ExtraNodeKubeProxyArgs             map[string]*string `json:"extra-node-kube-proxy-args,omitempty" yaml:"extra-node-kube-proxy-args,omitempty"`
	ExtraNodeKubeletArgs               map[string]*string `json:"extra-node-kubelet-args,omitempty" yaml:"extra-node-kubelet-args,omitempty"`
	ExtraNodeContainerdArgs            map[string]*string `json:"extra-node-containerd-args,omitempty" yaml:"extra-node-containerd-args,omitempty"`
	ExtraNodeK8sDqliteArgs             map[string]*string `json:"extra-node-k8s-dqlite-args,omitempty" yaml:"extra-node-k8s-dqlite-args,omitempty"`
}
