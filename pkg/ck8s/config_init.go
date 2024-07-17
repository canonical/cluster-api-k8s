package ck8s

import (
	"fmt"
	"strings"

	"k8s.io/utils/ptr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta2"
	apiv1 "github.com/canonical/cluster-api-k8s/pkg/ck8s/api"
	"github.com/canonical/cluster-api-k8s/pkg/secret"
)

type InitControlPlaneConfig struct {
	ControlPlaneEndpoint  string
	ControlPlaneConfig    bootstrapv1.CK8sControlPlaneConfig
	InitConfig            bootstrapv1.CK8sInitConfiguration
	PopulatedCertificates secret.Certificates
	DatastoreType         string
	DatastoreServers      []string

	ClusterNetwork *clusterv1.ClusterNetwork
}

func GenerateInitControlPlaneConfig(cfg InitControlPlaneConfig) (apiv1.BootstrapConfig, error) {
	out := apiv1.BootstrapConfig{}

	// seed certificates
	for _, cert := range cfg.PopulatedCertificates {
		if cert == nil || cert.KeyPair == nil {
			continue
		}
		switch cert.Purpose {
		case secret.ClusterCA:
			out.CACert = ptr.To(string(cert.KeyPair.Cert))
			out.CAKey = ptr.To(string(cert.KeyPair.Key))
		case secret.ClientClusterCA:
			out.ClientCACert = ptr.To(string(cert.KeyPair.Cert))
			out.ClientCAKey = ptr.To(string(cert.KeyPair.Key))
		case secret.ServiceAccount:
			out.ServiceAccountKey = ptr.To(string(cert.KeyPair.Key))
		case secret.FrontProxyCA:
			out.FrontProxyCACert = ptr.To(string(cert.KeyPair.Cert))
			out.FrontProxyCAKey = ptr.To(string(cert.KeyPair.Key))
		case secret.EtcdCA:
			out.DatastoreCACert = ptr.To(string(cert.KeyPair.Cert))
		case secret.APIServerEtcdClient:
			out.DatastoreClientCert = ptr.To(string(cert.KeyPair.Cert))
			out.DatastoreClientKey = ptr.To(string(cert.KeyPair.Key))
		}
	}
	// ensure required certificates
	if out.CACert == nil {
		return apiv1.BootstrapConfig{}, fmt.Errorf("missing server CA certificate")
	}
	if out.ClientCACert == nil {
		return apiv1.BootstrapConfig{}, fmt.Errorf("missing client CA certificate")
	}

	// cloud provider
	if v := cfg.ControlPlaneConfig.CloudProvider; v != "" {
		out.ClusterConfig.CloudProvider = ptr.To(v)
	}

	switch cfg.DatastoreType {
	case "", "k8s-dqlite":
		// Set default datastore type to k8s-dqlite
		out.DatastoreType = ptr.To("k8s-dqlite")

		k8sDqlitePort := cfg.ControlPlaneConfig.K8sDqlitePort
		if k8sDqlitePort == 0 {
			k8sDqlitePort = 2379
		}
		out.K8sDqlitePort = ptr.To(k8sDqlitePort)
	default:
		out.DatastoreType = ptr.To("external")
		out.DatastoreServers = cfg.DatastoreServers
	}

	// annotations
	out.ClusterConfig.Annotations = cfg.InitConfig.Annotations

	// features
	out.ClusterConfig.DNS.Enabled = ptr.To(cfg.InitConfig.GetEnableDefaultDNS())
	out.ClusterConfig.LocalStorage.Enabled = ptr.To(cfg.InitConfig.GetEnableDefaultLocalStorage())
	out.ClusterConfig.MetricsServer.Enabled = ptr.To(cfg.InitConfig.GetEnableDefaultMetricsServer())
	out.ClusterConfig.Network.Enabled = ptr.To(cfg.InitConfig.GetEnableDefaultNetwork())

	// networking
	if cfg.ClusterNetwork != nil {
		if v := ptr.Deref(cfg.ClusterNetwork.APIServerPort, 0); v != 0 {
			out.SecurePort = ptr.To(int(v))
		}
		if pods := cfg.ClusterNetwork.Pods; pods != nil {
			if len(pods.CIDRBlocks) > 0 {
				out.PodCIDR = ptr.To(strings.Join(pods.CIDRBlocks, ","))
			}
		}
		if services := cfg.ClusterNetwork.Services; services != nil {
			if len(services.CIDRBlocks) > 0 {
				out.ServiceCIDR = ptr.To(strings.Join(services.CIDRBlocks, ","))
			}
		}
		if v := cfg.ClusterNetwork.ServiceDomain; v != "" {
			out.ClusterConfig.DNS.ClusterDomain = ptr.To(v)
		}
	}

	// extra SANs
	out.ExtraSANs = append(out.ExtraSANs, cfg.ControlPlaneEndpoint)

	if v := cfg.ControlPlaneConfig.NodeTaints; len(v) > 0 {
		out.ControlPlaneTaints = v
	}

	out.ExtraNodeKubeAPIServerArgs = cfg.ControlPlaneConfig.ExtraKubeAPIServerArgs

	return out, nil
}
