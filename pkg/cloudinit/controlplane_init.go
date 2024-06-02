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
}

// NewInitControlPlane returns the user data string to be used on a controlplane instance.
func NewInitControlPlane(input InitControlPlaneInput) (CloudConfig, error) {
	config, err := NewBaseCloudConfig(input.BaseUserData)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("failed to generate base cloud-config: %w", err)
	}

	// write files
	config.WriteFiles = append(config.WriteFiles, File{
		Path:        "/opt/capi/etc/auth-token",
		Content:     input.Token,
		Permissions: "0400",
		Owner:       "root:root",
	})

	// run commands
	config.RunCommands = append(config.RunCommands, input.PreRunCommands...)
	config.RunCommands = append(config.RunCommands,
		"/opt/capi/scripts/install.sh",
		"/opt/capi/scripts/bootstrap.sh",
		"/opt/capi/scripts/wait-apiserver-ready.sh",
		"/opt/capi/scripts/deploy-manifests.sh",
		"/opt/capi/scripts/configure-token.sh",
	)
	config.RunCommands = append(config.RunCommands, input.PostRunCommands...)

	return config, nil
}
