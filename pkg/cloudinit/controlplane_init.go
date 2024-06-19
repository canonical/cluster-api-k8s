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

package cloudinit

import "fmt"

// InitControlPlaneInput defines the context to generate an init controlplane instance user data.
type InitControlPlaneInput struct {
	BaseUserData
	// Token is used to join more cluster nodes.
	Token string
	// K8sdProxyDaemonSet is the manifest that deploys k8sd-proxy to the cluster.
	K8sdProxyDaemonSet string
}

// NewInitControlPlane returns the user data string to be used on a controlplane instance.
func NewInitControlPlane(input InitControlPlaneInput) (CloudConfig, error) {
	config, err := NewBaseCloudConfig(input.BaseUserData)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("failed to generate base cloud-config: %w", err)
	}

	// write files
	config.WriteFiles = append(
		config.WriteFiles,
		File{
			Path:        "/capi/etc/token",
			Content:     input.Token,
			Permissions: "0400",
			Owner:       "root:root",
		},
		File{
			Path:        "/capi/manifests/00-k8sd-proxy.yaml",
			Content:     input.K8sdProxyDaemonSet,
			Permissions: "0400",
			Owner:       "root:root",
		},
	)

	// run commands
	config.RunCommands = append(config.RunCommands, input.PreRunCommands...)
	if !input.AirGapped {
		config.RunCommands = append(config.RunCommands, "/capi/scripts/install.sh")
	}
	config.RunCommands = append(config.RunCommands,
		"/capi/scripts/bootstrap.sh",
		"/capi/scripts/load-images.sh",
		"/capi/scripts/wait-apiserver-ready.sh",
		"/capi/scripts/deploy-manifests.sh",
		"/capi/scripts/configure-token.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
	)
	config.RunCommands = append(config.RunCommands, input.PostRunCommands...)

	return config, nil
}
