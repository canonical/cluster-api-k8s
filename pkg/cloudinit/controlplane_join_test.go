package cloudinit_test

import (
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
			MicroclusterAddress: ":2380",
		},
	})

	// TODO: add tests for expected files and commands
	g.Expect(err).To(BeNil())
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
}
