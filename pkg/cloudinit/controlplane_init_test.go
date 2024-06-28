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
	"os"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
)

func TestNewInitControlPlane(t *testing.T) {
	g := NewWithT(t)

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
		Token:              "test-token",
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
		"/capi/scripts/bootstrap.sh",
		"/capi/scripts/load-images.sh",
		"/capi/scripts/wait-apiserver-ready.sh",
		"/capi/scripts/deploy-manifests.sh",
		"/capi/scripts/configure-token.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
		"postrun1",
		"postrun2",
	}))

	// Define the expected files to write with their content, path, permissions, and owner.
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
			Path:        "/capi/etc/microcluster-address",
			Content:     "10.0.0.10:2380",
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
			Path:        "/capi/etc/token",
			Content:     "test-token",
			Permissions: "0400",
			Owner:       "root:root",
		},
		{
			Path:        "/capi/manifests/00-k8sd-proxy.yaml",
			Content:     "test-daemonset",
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
