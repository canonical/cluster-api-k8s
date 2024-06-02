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
	"testing"

	"github.com/canonical/cluster-api-k8s/pkg/cloudinit"
	. "github.com/onsi/gomega"
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
			MicroclusterAddress: ":2380",
		},
	})

	// TODO: add tests for expected files and commands
	g.Expect(err).To(BeNil())
	g.Expect(config.RunCommands).To(Equal([]string{
		"set -x",
		"prerun1",
		"prerun2",
		"/opt/capi/scripts/install.sh",
		"/opt/capi/scripts/bootstrap.sh",
		"/opt/capi/scripts/wait-apiserver-ready.sh",
		"/opt/capi/scripts/deploy-manifests.sh",
		"/opt/capi/scripts/configure-token.sh",
		"postrun1",
		"postrun2",
	}))
}
