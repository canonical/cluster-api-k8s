package cloudinit_test

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestNewJoinWorker(t *testing.T) {
	g := NewWithT(t)

	config, err := cloudinit.NewJoinWorker(cloudinit.JoinWorkerInput{
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
			MicroclusterAddress: "10.0.0.10",
			MicroclusterPort:    8080,
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
		"/capi/scripts/create-sentinel-bootstrap.sh",
		"postrun1",
		"postrun2",
	}))

	// NOTE (mateoflorido): Keep this test in sync with the expected paths in the worker_join.go file.
	expectedPaths := []interface{}{
		HaveField("Path", "/capi/scripts/install.sh"),
		HaveField("Path", "/capi/scripts/bootstrap.sh"),
		HaveField("Path", "/capi/scripts/load-images.sh"),
		HaveField("Path", "/capi/scripts/join-cluster.sh"),
		HaveField("Path", "/capi/scripts/wait-apiserver-ready.sh"),
		HaveField("Path", "/capi/scripts/deploy-manifests.sh"),
		HaveField("Path", "/capi/scripts/configure-token.sh"),
		HaveField("Path", "/capi/scripts/create-sentinel-bootstrap.sh"),
		HaveField("Path", "/capi/etc/config.yaml"),
		HaveField("Path", "/capi/etc/microcluster-address"),
		HaveField("Path", "/capi/etc/join-token"),
		HaveField("Path", "/capi/etc/snap-track"),
		HaveField("Path", "/tmp/file"),
	}

	g.Expect(config.WriteFiles).To(ConsistOf(expectedPaths...), "Some /capi/scripts files are missing")
}

func TestNewJoinWorkerInvalidVersionError(t *testing.T) {
	g := NewWithT(t)

	_, err := cloudinit.NewJoinWorker(cloudinit.JoinWorkerInput{
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

func TestNewJoinWorkerAirGapped(t *testing.T) {
	g := NewWithT(t)

	config, err := cloudinit.NewJoinWorker(cloudinit.JoinWorkerInput{
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
