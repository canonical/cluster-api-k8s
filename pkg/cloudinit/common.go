package cloudinit

import (
	"fmt"
	"net"
	"path/filepath"
	"strconv"

	"k8s.io/apimachinery/pkg/util/version"
)

type BaseUserData struct {
	// KubernetesVersion is the Kubernetes version from the cluster object.
	KubernetesVersion string
	// SnapRiskLevel is the risk level of the snap channels.
	SnapRiskLevel string
	// BootCommands is a list of commands to run early in the boot process.
	BootCommands []string
	// PreRunCommands is a list of commands to run prior to k8s installation.
	PreRunCommands []string
	// PostRunCommands is a list of commands to run after k8s installation.
	PostRunCommands []string
	// ExtraFiles is a list of extra files to load on the host.
	ExtraFiles []File
	// ConfigFileContents is the contents of the k8s configuration file.
	ConfigFileContents string
	// AirGapped declares that a custom installation script is to be used.
	AirGapped bool
	// MicroclusterAddress is the address to use for microcluster.
	MicroclusterAddress string
	// MicroclusterPort is the port to use for microcluster.
	MicroclusterPort int
}

func NewBaseCloudConfig(data BaseUserData) (CloudConfig, error) {
	kubernetesVersion, err := version.ParseSemantic(data.KubernetesVersion)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("failed to parse kubernetes version %q: %w", data.KubernetesVersion, err)
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
	// write files
	config.WriteFiles = append(
		config.WriteFiles,
		append(
			data.ExtraFiles,
			File{
				Path:        "/capi/etc/config.yaml",
				Content:     data.ConfigFileContents,
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
				Path:        "/capi/etc/snap-track",
				Content:     fmt.Sprintf("%d.%d-classic/%s", kubernetesVersion.Major(), kubernetesVersion.Minor(), data.SnapRiskLevel),
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
