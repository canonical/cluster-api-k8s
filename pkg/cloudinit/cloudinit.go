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
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
)

// CloudConfig is cloud-init userdata. The schema matches the examples found in
// https://cloudinit.readthedocs.io/en/latest/topics/examples.html.
type CloudConfig struct {
	// WriteFiles is a list of files cloud-init will create on the first boot.
	WriteFiles []File `yaml:"write_files"`

	// RunCommands is a list of commands to execute during the first boot.
	RunCommands []string `yaml:"runcmd"`

	// BootCommands is a list of commands to run early in the boot process.
	BootCommands []string `yaml:"bootcmd"`
}

//go:embed scripts/cloud-config-template
var cloudConfigTemplate string

// GenerateCloudConfig generates userdata from a CloudConfig.
func GenerateCloudConfig(config CloudConfig) ([]byte, error) {
	tmpl := template.Must(template.New("CloudConfigTemplate").Funcs(templateFuncsMap).Parse(cloudConfigTemplate))

	b := &bytes.Buffer{}
	if err := tmpl.Execute(b, config); err != nil {
		return nil, fmt.Errorf("failed to render cloud-config: %w", err)
	}
	return b.Bytes(), nil
}
