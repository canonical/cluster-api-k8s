package cloudinit

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"k8s.io/apimachinery/pkg/util/version"
)

type InstallOption string

const (
	InstallOptionChannel   InstallOption = "channel"
	InstallOptionRevision  InstallOption = "revision"
	InstallOptionLocalPath InstallOption = "local-path"
)

type SnapInstallData struct {
	// Option is the snap install option e.g. --channel, --revision.
	Option InstallOption
	// Value is the snap install value e.g. 1.30/stable, 123, /path/to/k8s.snap.
	Value string
}

type BaseUserData struct {
	// KubernetesVersion is the Kubernetes version from the cluster object.
	KubernetesVersion string
	// SnapInstallData is the snap install data.
	SnapInstallData *SnapInstallData
	// BootCommands is a list of commands to run early in the boot process.
	BootCommands []string
	// PreRunCommands is a list of commands to run prior to k8s installation.
	PreRunCommands []string
	// PostRunCommands is a list of commands to run after k8s installation.
	PostRunCommands []string
	// BootstrapConfig is the contents of the bootstrap configuration file.
	BootstrapConfig string
	// ExtraFiles is a list of extra files to load on the host.
	ExtraFiles []File
	// ConfigFileContents is the contents of the k8s configuration file.
	ConfigFileContents string
	// AirGapped declares that a custom installation script is to be used.
	AirGapped bool
	// The snap store proxy domain's scheme, e.g. "http" or "https" without "://"
	SnapstoreProxyScheme string
	// The snap store proxy domain
	SnapstoreProxyDomain string
	// The snap store proxy ID
	SnapstoreProxyID string
	// HTTPProxy is http_proxy configuration.
	HTTPProxy string
	// HTTPSProxy is https_proxy configuration.
	HTTPSProxy string
	// NoProxy is no_proxy configuration.
	NoProxy string
	// MicroclusterAddress is the address to use for microcluster.
	MicroclusterAddress string
	// MicroclusterPort is the port to use for microcluster.
	MicroclusterPort int
	// NodeName is the name of the node to set on microcluster.
	NodeName string
	// NodeToken is used for authenticating per-node k8sd endpoints.
	NodeToken string
}

func NewBaseCloudConfig(data BaseUserData) (CloudConfig, error) {
	kubernetesVersion, err := version.ParseSemantic(data.KubernetesVersion)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("failed to parse kubernetes version %q: %w", data.KubernetesVersion, err)
	}

	snapInstall := data.SnapInstallData
	// Default to k8s version if snap install option is not set or empty.
	if snapInstall == nil {
		snapInstall = &SnapInstallData{
			Option: InstallOptionChannel,
			Value:  fmt.Sprintf("%d.%d-classic/stable", kubernetesVersion.Major(), kubernetesVersion.Minor()),
		}
	}

	config := CloudConfig{
		RunCommands: []string{"set -x"},
		WriteFiles:  make([]File, 0, len(scripts)+len(data.ExtraFiles)+3),
	}

	// base files
	for script, contents := range scripts {
		config.WriteFiles = append(config.WriteFiles, File{
			Content:     contents,
			Path:        filepath.Join("/capi/scripts", (string(script))),
			Permissions: "0500",
			Owner:       "root:root",
		})
	}

	// snapstore proxy configuration
	if snapStoreConfigFiles := getSnapstoreProxyConfigFiles(data); snapStoreConfigFiles != nil {
		config.WriteFiles = append(config.WriteFiles, snapStoreConfigFiles...)
		config.RunCommands = append(config.RunCommands, "/capi/scripts/configure-snapstore-proxy.sh")
	}

	// proxy configuration
	if proxyConfigFiles := getProxyConfigFiles(data); proxyConfigFiles != nil {
		config.WriteFiles = append(config.WriteFiles, proxyConfigFiles...)
		config.RunCommands = append(config.RunCommands, "/capi/scripts/configure-proxy.sh")
	}

	var configFileContents string
	if data.BootstrapConfig != "" {
		configFileContents = data.BootstrapConfig
	} else {
		configFileContents = data.ConfigFileContents
	}

	// write files
	config.WriteFiles = append(
		config.WriteFiles,
		append(
			data.ExtraFiles,
			File{
				Path:        "/capi/etc/config.yaml",
				Content:     configFileContents,
				Permissions: "0400",
				Owner:       "root:root",
			},
			File{
				Path:        "/capi/etc/node-token",
				Content:     data.NodeToken,
				Permissions: "0400",
				Owner:       "root:root",
			},
			File{
				Path:        "/capi/etc/microcluster-address",
				Content:     makeMicroclusterAddress(data.MicroclusterAddress, data.MicroclusterPort),
				Permissions: "0400",
				Owner:       "root:root",
			},
			File{
				Path:        "/capi/etc/node-name",
				Content:     data.NodeName,
				Permissions: "0400",
				Owner:       "root:root",
			},
			File{
				Path:        fmt.Sprintf("/capi/etc/snap-%s", snapInstall.Option),
				Content:     snapInstall.Value,
				Permissions: "0400",
				Owner:       "root:root",
			},
		)...,
	)
	// boot commands
	config.BootCommands = data.BootCommands

	return config, nil
}

func makeMicroclusterAddress(address string, port int) string {
	return net.JoinHostPort(address, strconv.Itoa(port))
}

// getSnapstoreProxyConfigFiles returns the snapstore proxy config files.
// If the snapstore proxy domain or ID is not set, it returns nil.
// Nil indicates that no files are returned.
func getSnapstoreProxyConfigFiles(data BaseUserData) []File {
	snapstoreProxyScheme := data.SnapstoreProxyScheme
	snapstoreProxyDomain := data.SnapstoreProxyDomain
	snapstoreProxyID := data.SnapstoreProxyID

	scheme := "http"
	if snapstoreProxyScheme != "" {
		scheme = snapstoreProxyScheme
	}

	if snapstoreProxyDomain == "" || snapstoreProxyID == "" {
		return nil
	}

	schemeFile := File{
		Path:        "/capi/etc/snapstore-proxy-scheme",
		Content:     scheme,
		Permissions: "0400",
		Owner:       "root:root",
	}

	domainFile := File{
		Path:        "/capi/etc/snapstore-proxy-domain",
		Content:     snapstoreProxyDomain,
		Permissions: "0400",
		Owner:       "root:root",
	}

	storeIDFile := File{
		Path:        "/capi/etc/snapstore-proxy-id",
		Content:     snapstoreProxyID,
		Permissions: "0400",
		Owner:       "root:root",
	}

	return []File{schemeFile, domainFile, storeIDFile}
}

// getProxyConfigFiles returns the node proxy config files.
// If the HTTPProxy or HTTPPSProxy is not set, it returns nil.
// Nil indicates that no files are returned.
func getProxyConfigFiles(data BaseUserData) []File {
	if data.HTTPProxy == "" && data.HTTPSProxy == "" {
		return nil
	}
	return []File{
		{
			Path:        "/capi/etc/http-proxy",
			Content:     data.HTTPProxy,
			Permissions: "0400",
			Owner:       "root:root",
		},
		{
			Path:        "/capi/etc/https-proxy",
			Content:     data.HTTPSProxy,
			Permissions: "0400",
			Owner:       "root:root",
		},
		{
			Path:        "/capi/etc/no-proxy",
			Content:     data.NoProxy,
			Permissions: "0400",
			Owner:       "root:root",
		},
	}
}
