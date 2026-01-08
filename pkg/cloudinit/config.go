package cloudinit

import (
	"fmt"

	apiv1 "github.com/canonical/k8s-snap-api/api/v1"
	kubeyaml "sigs.k8s.io/yaml"
)

// MergeBootstrapConfigFileContents merges the user-provided bootstrap configuration
// with the generated bootstrap configuration. In case of conflicts, the user-provided
// configuration takes precedence.
func MergeBootstrapConfigFileContents(userConfigStr, generatedConfigStr string) (string, error) {
	merged := apiv1.BootstrapConfig{}
	if err := kubeyaml.Unmarshal([]byte(generatedConfigStr), &merged); err != nil {
		return "", fmt.Errorf("failed to unmarshal generated bootstrap config: %w", err)
	}

	if err := kubeyaml.Unmarshal([]byte(userConfigStr), &merged); err != nil {
		return "", fmt.Errorf("failed to unmarshal user-provided bootstrap config: %w", err)
	}

	mergedBytes, err := kubeyaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("failed to marshal final bootstrap config: %w", err)
	}

	return string(mergedBytes), nil
}
