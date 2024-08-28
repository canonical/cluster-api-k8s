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

import (
	"fmt"

	apiv1 "github.com/canonical/k8s-snap-api/api/v1"
	"gopkg.in/yaml.v2"
)

// JoinControlPlaneInput defines the context to generate a join controlplane instance user data.
type JoinControlPlaneInput struct {
	BaseUserData
	// JoinToken is the token to use to join the cluster.
	JoinToken string
}

// NewJoinControlPlane returns the user data string to be used on a controlplane instance.
func NewJoinControlPlane(input JoinControlPlaneInput) (CloudConfig, error) {
	input, err := addJoinTokenToExtraSANsConfig(input)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("failed to add join token to ExtraSANs: %w", err)
	}

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
		"/capi/scripts/wait-apiserver-ready.sh",
		"/capi/scripts/create-sentinel-bootstrap.sh",
	)
	config.RunCommands = append(config.RunCommands, input.PostRunCommands...)

	return config, nil
}

// addJoinTokenToExtraSANsConfig adds the JoinToken to the ExtraSANs field in the control plane node join config.
// This is required because the token name and kubelet name diverge in the CAPI context.
// See https://github.com/canonical/k8s-snap/pull/629 for more details.
func addJoinTokenToExtraSANsConfig(input JoinControlPlaneInput) (JoinControlPlaneInput, error) {
	var joinConfig apiv1.ControlPlaneJoinConfig
	err := yaml.Unmarshal([]byte(input.BaseUserData.ConfigFileContents), &joinConfig)
	if err != nil {
		return JoinControlPlaneInput{}, fmt.Errorf("failed to parse config file: %w", err)
	}

	if joinConfig.ExtraSANS == nil {
		joinConfig.ExtraSANS = []string{}
	}

	joinConfig.ExtraSANS = append(joinConfig.ExtraSANS, input.JoinToken)
	updatedConfig, err := yaml.Marshal(joinConfig)
	if err != nil {
		return JoinControlPlaneInput{}, fmt.Errorf("failed to marshal updated config: %w", err)
	}
	input.BaseUserData.ConfigFileContents = string(updatedConfig)
	return input, nil
}
