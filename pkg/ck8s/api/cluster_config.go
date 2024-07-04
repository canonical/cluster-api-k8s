package apiv1

type UserFacingClusterConfig struct {
	Network       NetworkConfig       `json:"network,omitempty" yaml:"network,omitempty"`
	DNS           DNSConfig           `json:"dns,omitempty" yaml:"dns,omitempty"`
	Ingress       IngressConfig       `json:"ingress,omitempty" yaml:"ingress,omitempty"`
	LoadBalancer  LoadBalancerConfig  `json:"load-balancer,omitempty" yaml:"load-balancer,omitempty"`
	LocalStorage  LocalStorageConfig  `json:"local-storage,omitempty" yaml:"local-storage,omitempty"`
	Gateway       GatewayConfig       `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	MetricsServer MetricsServerConfig `json:"metrics-server,omitempty" yaml:"metrics-server,omitempty"`
	CloudProvider *string             `json:"cloud-provider,omitempty" yaml:"cloud-provider,omitempty"`
	Annotations   map[string]string   `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type DNSConfig struct {
	Enabled             *bool     `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	ClusterDomain       *string   `json:"cluster-domain,omitempty" yaml:"cluster-domain,omitempty"`
	ServiceIP           *string   `json:"service-ip,omitempty" yaml:"service-ip,omitempty"`
	UpstreamNameservers *[]string `json:"upstream-nameservers,omitempty" yaml:"upstream-nameservers,omitempty"`
}

type IngressConfig struct {
	Enabled             *bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	DefaultTLSSecret    *string `json:"default-tls-secret,omitempty" yaml:"default-tls-secret,omitempty"`
	EnableProxyProtocol *bool   `json:"enable-proxy-protocol,omitempty" yaml:"enable-proxy-protocol,omitempty"`
}

type LoadBalancerConfig struct {
	Enabled        *bool     `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	CIDRs          *[]string `json:"cidrs,omitempty" yaml:"cidrs,omitempty"`
	L2Mode         *bool     `json:"l2-mode,omitempty" yaml:"l2-mode,omitempty"`
	L2Interfaces   *[]string `json:"l2-interfaces,omitempty" yaml:"l2-interfaces,omitempty"`
	BGPMode        *bool     `json:"bgp-mode,omitempty" yaml:"bgp-mode,omitempty"`
	BGPLocalASN    *int      `json:"bgp-local-asn,omitempty" yaml:"bgp-local-asn,omitempty"`
	BGPPeerAddress *string   `json:"bgp-peer-address,omitempty" yaml:"bgp-peer-address,omitempty"`
	BGPPeerASN     *int      `json:"bgp-peer-asn,omitempty" yaml:"bgp-peer-asn,omitempty"`
	BGPPeerPort    *int      `json:"bgp-peer-port,omitempty" yaml:"bgp-peer-port,omitempty"`
}

type LocalStorageConfig struct {
	Enabled       *bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	LocalPath     *string `json:"local-path,omitempty" yaml:"local-path,omitempty"`
	ReclaimPolicy *string `json:"reclaim-policy,omitempty" yaml:"reclaim-policy,omitempty"`
	Default       *bool   `json:"default,omitempty" yaml:"default,omitempty"`
}

type NetworkConfig struct {
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

type GatewayConfig struct {
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

type MetricsServerConfig struct {
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}
