/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	format "github.com/onsi/gomega/format"
	"github.com/onsi/gomega/gstruct"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestNewInitControlPlane(t *testing.T) {
	g := NewWithT(t)

	format.MaxLength = 20000

	config, err := cloudinit.NewInitControlPlane(cloudinit.InitControlPlaneInput{
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
		},
		AuthToken:          "test-token",
		K8sdProxyDaemonSet: "test-daemonset",
	})

	g.Expect(err).ToNot(HaveOccurred())

	// Verify the boot commands.
	g.Expect(config.BootCommands).To(Equal([]string{"bootcmd"}))

	// Verify the run commands.
	g.Expect(config.RunCommands).To(Equal([]string{
		"set -x",
		"prerun1",
		"prerun2",
		"/capi/scripts/install.sh",
		"/capi/scripts/disable-host-services.sh",
		"/capi/scripts/bootstrap.sh",
		"/capi/scripts/load-images.sh",
		"/capi/scripts/wait-apiserver-ready.sh",
		"/capi/scripts/deploy-manifests.sh",
		"/capi/scripts/configure-auth-token.sh",
		"/capi/scripts/configure-node-token.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
		"postrun1",
		"postrun2",
	}))

	// NOTE (mateoflorido): Keep this test in sync with the expected paths in the controlplane_init.go file.
	g.Expect(config.WriteFiles).To(ConsistOf(
		HaveField("Path", "/capi/scripts/disable-host-services.sh"),
		HaveField("Path", "/capi/scripts/install.sh"),
		HaveField("Path", "/capi/scripts/bootstrap.sh"),
		HaveField("Path", "/capi/scripts/load-images.sh"),
		HaveField("Path", "/capi/scripts/join-cluster.sh"),
		HaveField("Path", "/capi/scripts/wait-apiserver-ready.sh"),
		HaveField("Path", "/capi/scripts/deploy-manifests.sh"),
		HaveField("Path", "/capi/scripts/configure-auth-token.sh"),
		HaveField("Path", "/capi/scripts/configure-proxy.sh"),
		HaveField("Path", "/capi/scripts/configure-snapstore-proxy.sh"),
		HaveField("Path", "/capi/scripts/configure-node-token.sh"),
		HaveField("Path", "/capi/scripts/create-sentinel-bootstrap.sh"),
		HaveField("Path", "/capi/etc/config.yaml"),
		HaveField("Path", "/capi/etc/microcluster-address"),
		HaveField("Path", "/capi/etc/node-name"),
		HaveField("Path", "/capi/etc/node-token"),
		HaveField("Path", "/capi/etc/token"),
		HaveField("Path", "/capi/etc/snap-channel"),
		HaveField("Path", "/capi/manifests/00-k8sd-proxy.yaml"),
		HaveField("Path", "/tmp/file"),
	), "Some /capi/scripts files are missing")
}

func TestNewInitControlPlaneWithOptionalProxies(t *testing.T) {
	g := NewWithT(t)
	format.MaxLength = 20000

	config, err := cloudinit.NewInitControlPlane(cloudinit.InitControlPlaneInput{
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
			SnapstoreProxyScheme: "http",
			SnapstoreProxyDomain: "snapstore.io",
			SnapstoreProxyID:     "abcd-1234-xyz",
			HTTPProxy:            "http://proxy.internal",
			HTTPSProxy:           "https://proxy.internal",
			NoProxy:              "10.0.0.0/8,10.152.183.1,192.168.0.0/16",
			ConfigFileContents:   "### config file ###",
			MicroclusterAddress:  "10.0.0.0/8",
		},
		AuthToken:          "test-token",
		K8sdProxyDaemonSet: "test-daemonset",
	})

	g.Expect(err).ToNot(HaveOccurred())

	// Verify the boot commands.
	g.Expect(config.BootCommands).To(Equal([]string{"bootcmd"}))

	// Verify the run commands.
	g.Expect(config.RunCommands).To(Equal([]string{
		"set -x",
		"/capi/scripts/configure-snapstore-proxy.sh",
		"/capi/scripts/configure-proxy.sh",
		"prerun1",
		"prerun2",
		"/capi/scripts/install.sh",
		"/capi/scripts/disable-host-services.sh",
		"/capi/scripts/bootstrap.sh",
		"/capi/scripts/load-images.sh",
		"/capi/scripts/wait-apiserver-ready.sh",
		"/capi/scripts/deploy-manifests.sh",
		"/capi/scripts/configure-auth-token.sh",
		"/capi/scripts/configure-node-token.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
		"postrun1",
		"postrun2",
	}))

	// NOTE (mateoflorido): Keep this test in sync with the expected paths in the controlplane_init.go file.
	g.Expect(config.WriteFiles).To(ConsistOf(
		HaveField("Path", "/capi/scripts/disable-host-services.sh"),
		HaveField("Path", "/capi/scripts/install.sh"),
		HaveField("Path", "/capi/scripts/bootstrap.sh"),
		HaveField("Path", "/capi/scripts/load-images.sh"),
		HaveField("Path", "/capi/scripts/join-cluster.sh"),
		HaveField("Path", "/capi/scripts/wait-apiserver-ready.sh"),
		HaveField("Path", "/capi/scripts/deploy-manifests.sh"),
		HaveField("Path", "/capi/scripts/configure-auth-token.sh"),
		HaveField("Path", "/capi/scripts/configure-proxy.sh"),
		HaveField("Path", "/capi/scripts/configure-snapstore-proxy.sh"),
		HaveField("Path", "/capi/scripts/configure-node-token.sh"),
		HaveField("Path", "/capi/scripts/create-sentinel-bootstrap.sh"),
		HaveField("Path", "/capi/etc/config.yaml"),
		HaveField("Path", "/capi/etc/http-proxy"),
		HaveField("Path", "/capi/etc/https-proxy"),
		HaveField("Path", "/capi/etc/no-proxy"),
		HaveField("Path", "/capi/etc/microcluster-address"),
		HaveField("Path", "/capi/etc/node-name"),
		HaveField("Path", "/capi/etc/node-token"),
		HaveField("Path", "/capi/etc/token"),
		HaveField("Path", "/capi/etc/snap-channel"),
		HaveField("Path", "/capi/manifests/00-k8sd-proxy.yaml"),
		HaveField("Path", "/capi/etc/snapstore-proxy-scheme"),
		HaveField("Path", "/capi/etc/snapstore-proxy-domain"),
		HaveField("Path", "/capi/etc/snapstore-proxy-id"),
		HaveField("Path", "/tmp/file"),
	), "Some /capi/scripts files are missing")
}

func TestUserSuppliedBootstrapConfig(t *testing.T) {
	g := NewWithT(t)

	config, err := cloudinit.NewInitControlPlane(cloudinit.InitControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			KubernetesVersion:  "v1.30.0",
			BootstrapConfig:    "### bootstrap config ###",
			ConfigFileContents: "### config file ###",
		},
	})

	g.Expect(err).ToNot(HaveOccurred())

	// Test that user-supplied bootstrap configuration takes precedence over ConfigFileContents.
	g.Expect(config.WriteFiles).To(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Path":    Equal("/capi/etc/config.yaml"),
		"Content": Equal("### bootstrap config ###"),
	})))

	g.Expect(config.WriteFiles).NotTo(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Path":    Equal("/capi/etc/config.yaml"),
		"Content": Equal("### config file ###"),
	})))
}

func TestNewInitControlPlaneInvalidVersionError(t *testing.T) {
	g := NewWithT(t)

	_, err := cloudinit.NewInitControlPlane(cloudinit.InitControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			KubernetesVersion: "invalid-version",
			BootCommands:      []string{"bootcmd"},
			PreRunCommands:    []string{"prerun1", "prerun2"},
			PostRunCommands:   []string{"postrun1", "postrun2"},
		}})

	g.Expect(err).To(HaveOccurred())
}

func TestNewInitControlPlaneAirGapped(t *testing.T) {
	g := NewWithT(t)

	config, err := cloudinit.NewInitControlPlane(cloudinit.InitControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			KubernetesVersion: "v1.30.0",
			BootCommands:      []string{"bootcmd"},
			PreRunCommands:    []string{"prerun1", "prerun2"},
			PostRunCommands:   []string{"postrun1", "postrun2"},
			AirGapped:         true,
		}})

	g.Expect(err).NotTo(HaveOccurred())

	// Verify the run commands is missing install.sh script.
	g.Expect(config.RunCommands).NotTo(ContainElement("/capi/scripts/install.sh"))
}

func TestNewInitControlPlaneSnapInstall(t *testing.T) {
	t.Run("DefaultSnapInstall", func(t *testing.T) {
		g := NewWithT(t)

		config, err := cloudinit.NewInitControlPlane(cloudinit.InitControlPlaneInput{
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
}

func TestNewInitControlPlaneSnapInstallOverrides(t *testing.T) {
	tests := []struct {
		name        string
		snapInstall *cloudinit.SnapInstallData
		notOptions  []cloudinit.InstallOption
	}{
		{
			name: "ChannelOverride",
			snapInstall: &cloudinit.SnapInstallData{
				Option: cloudinit.InstallOptionChannel,
				Value:  "v1.30/edge",
			},
			notOptions: []cloudinit.InstallOption{cloudinit.InstallOptionRevision, cloudinit.InstallOptionLocalPath},
		},
		{
			name: "RevisionOverride",
			snapInstall: &cloudinit.SnapInstallData{
				Option: cloudinit.InstallOptionRevision,
				Value:  "123",
			},
			notOptions: []cloudinit.InstallOption{cloudinit.InstallOptionChannel, cloudinit.InstallOptionLocalPath},
		},
		{
			name: "LocalPathOverride",
			snapInstall: &cloudinit.SnapInstallData{
				Option: cloudinit.InstallOptionLocalPath,
				Value:  "/path/to/k8s.snap",
			},
			notOptions: []cloudinit.InstallOption{cloudinit.InstallOptionChannel, cloudinit.InstallOptionRevision},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			config, err := cloudinit.NewInitControlPlane(cloudinit.InitControlPlaneInput{
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

			// Check that the incorrect files are not written
			for _, notOption := range tt.notOptions {
				g.Expect(config.WriteFiles).NotTo(ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Path": Equal(fmt.Sprintf("/capi/etc/snap-%s", notOption)),
				})))
			}
		})
	}
}
