package cloudinit_test

import (
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestNewJoinControlPlane(t *testing.T) {
	g := NewWithT(t)

	config, err := cloudinit.NewJoinControlPlane(cloudinit.JoinControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			KubernetesVersion: "v1.30.0",
			BootCommands:      []string{"bootcmd"},
			PreRunCommands:    []string{"prerun1", "prerun2"},
			PostRunCommands:   []string{"postrun1", "postrun2"},
			ExtraFiles: []cloudinit.File{{
				Path:        "/tmp/file",
				Content:     "test file",
				Permissions: "0400",
				Owner:       "root:root",
			}},
			ConfigFileContents:  "### config file ###",
			MicroclusterAddress: "10.0.0.11",
		},
		JoinToken: "test-token",
	})

	g.Expect(err).NotTo(HaveOccurred())

	// Verify the boot commands.
	g.Expect(config.BootCommands).To(Equal([]string{"bootcmd"}))

	// Verify the run commands.
	g.Expect(config.RunCommands).To(Equal([]string{
		"set -x",
		"prerun1",
		"prerun2",
		"/capi/scripts/install.sh",
		"/capi/scripts/load-images.sh",
		"/capi/scripts/join-cluster.sh",
		"/capi/scripts/wait-apiserver-ready.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
		"postrun1",
		"postrun2",
	}))

	// Define the expected extra files with their content, path, permissions, and owner.
	expectedWriteFiles := []cloudinit.File{
		{
			Path:        "/tmp/file",
			Content:     "test file",
			Permissions: "0400",
			Owner:       "root:root",
		},
		{
			Path:        "/capi/etc/config.yaml",
			Content:     "### config file ###",
			Permissions: "0400",
			Owner:       "root:root",
		},
		{
			Path:        "/capi/etc/snap-track",
			Content:     "1.30-classic/stable",
			Permissions: "0400",
			Owner:       "root:root",
		},
		{
			Path:        "/capi/etc/microcluster-address",
			Content:     "10.0.0.11:2380",
			Permissions: "0400",
			Owner:       "root:root",
		},
		{
			Path:        "/capi/etc/join-token",
			Content:     "test-token",
			Permissions: "0400",
			Owner:       "root:root",
		},
	}

	scriptFiles := map[string]string{
		"./scripts/install.sh":                   "/capi/scripts/install.sh",
		"./scripts/bootstrap.sh":                 "/capi/scripts/bootstrap.sh",
		"./scripts/load-images.sh":               "/capi/scripts/load-images.sh",
		"./scripts/join-cluster.sh":              "/capi/scripts/join-cluster.sh",
		"./scripts/wait-apiserver-ready.sh":      "/capi/scripts/wait-apiserver-ready.sh",
		"./scripts/deploy-manifests.sh":          "/capi/scripts/deploy-manifests.sh",
		"./scripts/configure-token.sh":           "/capi/scripts/configure-token.sh",
		"./scripts/create-sentinel-bootstrap.sh": "/capi/scripts/create-sentinel-bootstrap.sh",
	}

	// Read the content of each script file and append it to the expected write files.
	for relativePath, scriptPath := range scriptFiles {
		content, err := os.ReadFile(relativePath)
		g.Expect(err).NotTo(HaveOccurred())
		expectedWriteFiles = append(expectedWriteFiles, cloudinit.File{
			Path:        scriptPath,
			Content:     string(content),
			Permissions: "0500",
			Owner:       "root:root",
		})
	}

	g.Expect(config.WriteFiles).To(ConsistOf(expectedWriteFiles))
}

func TestNewJoinControlPlaneError(t *testing.T) {
	g := NewWithT(t)

	_, err := cloudinit.NewJoinControlPlane(cloudinit.JoinControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			KubernetesVersion: "invalid-version",
			BootCommands:      []string{"bootcmd"},
			PreRunCommands:    []string{"prerun1", "prerun2"},
			PostRunCommands:   []string{"postrun1", "postrun2"},
		},
		JoinToken: "test-token",
	})

	g.Expect(err).To(HaveOccurred())
}

func TestNewJoinControlPlaneAirGapped(t *testing.T) {
	g := NewWithT(t)

	config, err := cloudinit.NewJoinControlPlane(cloudinit.JoinControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			KubernetesVersion: "v1.30.0",
			BootCommands:      []string{"bootcmd"},
			PreRunCommands:    []string{"prerun1", "prerun2"},
			PostRunCommands:   []string{"postrun1", "postrun2"},
			AirGapped:         true,
		},
		JoinToken: "test-token",
	})

	g.Expect(err).NotTo(HaveOccurred())

	// Verify the run commands is missing install.sh script.
	g.Expect(config.RunCommands).NotTo(ContainElement("/capi/scripts/install.sh"))
}
