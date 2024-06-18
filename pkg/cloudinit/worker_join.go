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

// JoinWorkerInput defines the context to generate a join controlplane instance user data.
type JoinWorkerInput struct {
	BaseUserData
	// JoinToken is the token to use to join the cluster.
	JoinToken string
}

// NewJoinWorker returns the user data string to be used on a controlplane instance.
func NewJoinWorker(input JoinWorkerInput) (CloudConfig, error) {
	config, err := NewBaseCloudConfig(input.BaseUserData)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("failed to generate base cloud-config: %w", err)
	}

	// write files
	config.WriteFiles = append(config.WriteFiles, File{
		Path:        "/capi/etc/join-token",
		Content:     input.JoinToken,
		Permissions: "0400",
		Owner:       "root:root",
	})

	// run commands
	config.RunCommands = append(config.RunCommands, input.PreRunCommands...)
	if !input.AirGapped {
		config.RunCommands = append(config.RunCommands, "/capi/scripts/install.sh")
	}
	config.RunCommands = append(config.RunCommands,
		"/capi/scripts/load-images.sh",
		"/capi/scripts/join-cluster.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
	)
	config.RunCommands = append(config.RunCommands, input.PostRunCommands...)

	return config, nil
}
