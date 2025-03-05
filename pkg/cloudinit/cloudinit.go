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
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
)

const (
	defaultYamlIndent = 2
)

var (
	// managedCloudInitFields is a list of fields that are managed internally
	// and user cannot provide them as additional user data.
	managedCloudInitFields = []string{"bootcmd", "runcmd", "write_files"}
)

// CloudConfig is cloud-init userdata. The schema matches the examples found in
// https://cloudinit.readthedocs.io/en/latest/topics/examples.html.
type CloudConfig struct {
	// WriteFiles is a list of files cloud-init will create on the first boot.
	WriteFiles []File `yaml:"write_files"`

	// RunCommands is a list of commands to execute during the first boot.
	RunCommands []string `yaml:"runcmd"`

	// BootCommands is a list of commands to run early in the boot process.
	BootCommands []string `yaml:"bootcmd,omitempty"`

	// AdditionalUserData is an arbitrary key/value map of user defined configuration
	AdditionalUserData map[string]string `yaml:"-"`
}

// GenerateCloudConfig generates userdata from a CloudConfig.
func GenerateCloudConfig(ctx context.Context, config CloudConfig) ([]byte, error) {
	tmpl := template.Must(template.New("cloud-config-template").Funcs(templateFuncsMap).ParseFS(
		embeddedScriptsFS,
		"scripts/cloud-config-template",
		"scripts/additional-user-data-template",
	))

	if err := FormatAdditionalUserData(ctx, config.AdditionalUserData); err != nil {
		return nil, fmt.Errorf("failed to parse additional user data: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, config); err != nil {
		return nil, fmt.Errorf("failed to render cloud-config: %w", err)
	}
	return buf.Bytes(), nil
}

func FormatAdditionalUserData(ctx context.Context, additionalUserData map[string]string) error {
	tmpl := template.Must(template.New("cloud-config-template").Funcs(templateFuncsMap).ParseFS(
		embeddedScriptsFS,
		"scripts/additional-user-data-template"))
	tmpl = template.Must(tmpl.Parse(`{{template "additional" .}}`))

	log := logr.FromContextOrDiscard(ctx)

	// managed keys are removed from provided additional user data
	for _, key := range managedCloudInitFields {
		if _, ok := additionalUserData[key]; ok {
			delete(additionalUserData, key)
			log.Info(fmt.Sprintf("user provided key %s removed since it's internally managed by Ck8s CAPI", key))
		}
	}

	for k, v := range additionalUserData {
		buf := bytes.Buffer{}
		en := yaml.NewEncoder(&buf)
		en.SetIndent(defaultYamlIndent)

		// if the value is a YAML mapping first validate the content
		// and then format the value such that mapping becomes a valid yaml
		// value for the key
		// e.g. map[string]string{"key": "type: mapping"} becomes
		// key:
		//   type: mapping
		mappingValue := map[string]any{}
		if err := yaml.Unmarshal([]byte(v), &mappingValue); err == nil {
			if err := en.Encode(&mappingValue); err != nil {
				return fmt.Errorf("invalid mapping value: %s with error: %w", v, err)
			}

			indent := "\n" + strings.Repeat(" ", defaultYamlIndent)
			additionalUserData[k] = indent + strings.ReplaceAll(buf.String(), "\n", indent)

			continue
		}

		// if the value is a YAML sequence first validate the content
		// and then format the value that the value becomes a valid yaml sequence
		// e.g. map[string]string{"key": "- type: sequence"} becomes
		// key:
		//   - type: sequence
		sequenceValue := []any{}
		if err := yaml.Unmarshal([]byte(v), &sequenceValue); err == nil {
			if err := en.Encode(&sequenceValue); err != nil {
				return fmt.Errorf("invalid sequence value: %s with error: %w", v, err)
			}

			additionalUserData[k] = fmt.Sprintf("\n%s", buf.String())

			continue
		}

		// if the value is a YAML Literal, leave it as is since it's already
		// a valid yaml value
		// e.g. map[string]string{"key": "value"} becomes
		// key: value
	}

	// validate the formatted user data key/value map
	buf := &bytes.Buffer{}
	if err := tmpl.Execute(buf, additionalUserData); err != nil {
		return fmt.Errorf("failed to generate scripts/additional-user-data-template: %w", err)
	}

	out := make(map[string]any)
	if err := yaml.Unmarshal(buf.Bytes(), out); err != nil {
		return fmt.Errorf("failed to validate additional cloud-init user data: %w, please check if you have provided a valid yaml content: %s", err, buf.String())
	}

	return nil
}
