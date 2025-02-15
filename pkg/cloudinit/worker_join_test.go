package cloudinit_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestNewJoinWorker(t *testing.T) {
	g := NewWithT(t)

	config, err := cloudinit.NewJoinWorker(cloudinit.JoinWorkerInput{
		BaseUserData: cloudinit.BaseUserData{
			KubernetesVersion:    "v1.30.0",
			BootCommands:         []string{"bootcmd"},
			PreRunCommands:       []string{"prerun1", "prerun2"},
			PostRunCommands:      []string{"postrun1", "postrun2"},
			SnapstoreProxyScheme: "http",
			SnapstoreProxyDomain: "snapstore.io",
			SnapstoreProxyID:     "abcd-1234-xyz",
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
		"/capi/scripts/configure-snapstore-proxy.sh",
		"prerun1",
		"prerun2",
		"/capi/scripts/install.sh",
		"/capi/scripts/disable-host-services.sh",
		"/capi/scripts/load-images.sh",
		"/capi/scripts/join-cluster.sh",
		"/capi/scripts/configure-node-token.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
		"postrun1",
		"postrun2",
	}))

	// NOTE (mateoflorido): Keep this test in sync with the expected paths in the worker_join.go file.
	g.Expect(config.WriteFiles).To(ConsistOf(
		HaveField("Path", "/capi/scripts/install.sh"),
		HaveField("Path", "/capi/scripts/disable-host-services.sh"),
		HaveField("Path", "/capi/scripts/bootstrap.sh"),
		HaveField("Path", "/capi/scripts/load-images.sh"),
		HaveField("Path", "/capi/scripts/join-cluster.sh"),
		HaveField("Path", "/capi/scripts/wait-apiserver-ready.sh"),
		HaveField("Path", "/capi/scripts/deploy-manifests.sh"),
		HaveField("Path", "/capi/scripts/configure-auth-token.sh"),
		HaveField("Path", "/capi/scripts/configure-proxy.sh"),
		HaveField("Path", "/capi/scripts/configure-node-token.sh"),
		HaveField("Path", "/capi/scripts/create-sentinel-bootstrap.sh"),
		HaveField("Path", "/capi/scripts/configure-snapstore-proxy.sh"),
		HaveField("Path", "/capi/etc/config.yaml"),
		HaveField("Path", "/capi/etc/microcluster-address"),
		HaveField("Path", "/capi/etc/node-name"),
		HaveField("Path", "/capi/etc/node-token"),
		HaveField("Path", "/capi/etc/join-token"),
		HaveField("Path", "/capi/etc/snap-channel"),
		HaveField("Path", "/capi/etc/snapstore-proxy-scheme"),
		HaveField("Path", "/capi/etc/snapstore-proxy-domain"),
		HaveField("Path", "/capi/etc/snapstore-proxy-id"),
		HaveField("Path", "/tmp/file"),
	), "Some /capi/scripts files are missing")
}

func TestNewJoinWorkerWithProxySettings(t *testing.T) {
	g := NewWithT(t)
	for _, tc := range []struct {
		name             string
		baseUserData     cloudinit.BaseUserData
		expectRunCommand bool
		expectWriteFiles []types.GomegaMatcher
	}{
		{
			name: "AllProxyFieldsSet",
			baseUserData: cloudinit.BaseUserData{
				KubernetesVersion:   "v1.30.0",
				HTTPProxy:           "http://proxy.internal",
				HTTPSProxy:          "https://proxy.internal",
				NoProxy:             "10.0.0.0/8,10.152.183.1,192.168.0.0/16",
				MicroclusterAddress: "10.0.0.0/8",
			},
			expectRunCommand: true,
			expectWriteFiles: []types.GomegaMatcher{
				HaveField("Path", "/capi/scripts/configure-proxy.sh"),
				HaveField("Path", "/capi/etc/http-proxy"),
				HaveField("Path", "/capi/etc/https-proxy"),
				HaveField("Path", "/capi/etc/no-proxy"),
			},
		},
		{
			name: "HTTPSProxyOnly",
			baseUserData: cloudinit.BaseUserData{
				KubernetesVersion:   "v1.30.0",
				HTTPSProxy:          "https://proxy.internal",
				NoProxy:             "10.0.0.0/8,10.152.183.1,192.168.0.0/16",
				MicroclusterAddress: "10.0.0.0/8",
			},
			expectRunCommand: true,
			expectWriteFiles: []types.GomegaMatcher{
				HaveField("Path", "/capi/scripts/configure-proxy.sh"),
				HaveField("Path", "/capi/etc/https-proxy"),
			},
		},
		{
			name: "NoProxyFields",
			baseUserData: cloudinit.BaseUserData{
				KubernetesVersion:   "v1.30.0",
				MicroclusterAddress: "10.0.0.0/8",
			},
			expectRunCommand: false,
			expectWriteFiles: []types.GomegaMatcher{
				HaveField("Path", "/capi/scripts/configure-proxy.sh"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := cloudinit.NewJoinWorker(cloudinit.JoinWorkerInput{BaseUserData: tc.baseUserData})

			g.Expect(err).ToNot(HaveOccurred())
			// Verify proxy run command.
			if tc.expectRunCommand {
				g.Expect(config.RunCommands).To(ContainElement("/capi/scripts/configure-proxy.sh"))
			} else {
				g.Expect(config.RunCommands).NotTo(ContainElement("/capi/scripts/configure-proxy.sh"))
			}
			// Verify proxy files present.
			g.Expect(config.WriteFiles).To(ContainElements(tc.expectWriteFiles),
				"Required files in /capi directory  are missing")
		})
	}
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

func TestNewJoinWorkerSnapInstall(t *testing.T) {
	t.Run("DefaultSnapInstall", func(t *testing.T) {
		g := NewWithT(t)

		config, err := cloudinit.NewJoinWorker(cloudinit.JoinWorkerInput{
			BaseUserData: cloudinit.BaseUserData{
				KubernetesVersion: "v1.30.0",
				BootCommands:      []string{"bootcmd"},
				PreRunCommands:    []string{"prerun1", "prerun2"},
				PostRunCommands:   []string{"postrun1", "postrun2"},
			}})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(config.WriteFiles).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Path":    Equal(fmt.Sprintf("/capi/etc/snap-%s", cloudinit.InstallOptionChannel)),
			"Content": Equal("1.30-classic/stable"),
		})))
		g.Expect(config.WriteFiles).ToNot(ContainElement(HaveField("Path", fmt.Sprintf("/capi/etc/snap-%s", cloudinit.InstallOptionRevision))))
		g.Expect(config.WriteFiles).ToNot(ContainElement(HaveField("Path", fmt.Sprintf("/capi/etc/snap-%s", cloudinit.InstallOptionLocalPath))))
	})

	tests := []struct {
		name        string
		snapInstall *cloudinit.SnapInstallData
	}{
		{
			name: "ChannelOverride",
			snapInstall: &cloudinit.SnapInstallData{
				Option: cloudinit.InstallOptionChannel,
				Value:  "v1.30/stable",
			},
		},
		{
			name: "RevisionOverride",
			snapInstall: &cloudinit.SnapInstallData{
				Option: cloudinit.InstallOptionRevision,
				Value:  "123",
			},
		},
		{
			name: "LocalPathOverride",
			snapInstall: &cloudinit.SnapInstallData{
				Option: cloudinit.InstallOptionLocalPath,
				Value:  "/path/to/k8s.snap",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			config, err := cloudinit.NewJoinWorker(cloudinit.JoinWorkerInput{
				BaseUserData: cloudinit.BaseUserData{
					KubernetesVersion: "v1.30.0",
					SnapInstallData:   tt.snapInstall,
					BootCommands:      []string{"bootcmd"},
					PreRunCommands:    []string{"prerun1", "prerun2"},
					PostRunCommands:   []string{"postrun1", "postrun2"},
				}})

			g.Expect(err).NotTo(HaveOccurred())

			g.Expect(config.WriteFiles).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Path":    Equal(fmt.Sprintf("/capi/etc/snap-%s", tt.snapInstall.Option)),
				"Content": Equal(tt.snapInstall.Value),
			})))
		})
	}
}
