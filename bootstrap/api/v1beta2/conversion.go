/*


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

package v1beta2

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	bootstrapv1 "github.com/canonical/cluster-api-k8s/bootstrap/api/v1beta3"
)

// ConvertTo converts this CK8sConfig from the v1beta2 spoke version to the v1beta3 hub version.
func (src *CK8sConfig) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.CK8sConfig)
	if !ok {
		return fmt.Errorf("expected *v1beta3.CK8sConfig, got %T", dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if err := roundTripConvert(src.Spec, &dst.Spec); err != nil {
		return fmt.Errorf("failed to convert spec to v1beta3: %w", err)
	}

	if err := roundTripConvert(src.Status, &dst.Status); err != nil {
		return fmt.Errorf("failed to convert status to v1beta3: %w", err)
	}

	if len(src.Status.Conditions) > 0 {
		convertedConditions := make([]metav1.Condition, 0, len(src.Status.Conditions))
		if err := roundTripConvert(src.Status.Conditions, &convertedConditions); err != nil {
			return fmt.Errorf("failed to convert conditions to v1beta3: %w", err)
		}
		dst.Status.Conditions = convertedConditions
	}

	if src.Status.DataSecretName != nil {
		created := true
		dst.Status.Initialization.DataSecretCreated = &created
	}

	return nil
}

// ConvertFrom converts from the v1beta3 hub version to this v1beta2 spoke version.
func (dst *CK8sConfig) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.CK8sConfig)
	if !ok {
		return fmt.Errorf("expected *v1beta3.CK8sConfig, got %T", srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if err := roundTripConvert(src.Spec, &dst.Spec); err != nil {
		return fmt.Errorf("failed to convert spec from v1beta3: %w", err)
	}

	if err := roundTripConvert(src.Status, &dst.Status); err != nil {
		return fmt.Errorf("failed to convert status from v1beta3: %w", err)
	}

	if len(src.Status.Conditions) > 0 {
		legacyConditions := make(clusterv1.Conditions, 0, len(src.Status.Conditions))
		if err := roundTripConvert(src.Status.Conditions, &legacyConditions); err != nil {
			return fmt.Errorf("failed to convert conditions from v1beta3: %w", err)
		}
		dst.Status.Conditions = legacyConditions
	}

	if src.Status.Initialization.DataSecretCreated != nil {
		dst.Status.Ready = *src.Status.Initialization.DataSecretCreated
	}

	return nil
}

// ConvertTo converts this CK8sConfigTemplate from the v1beta2 spoke version to the v1beta3 hub version.
func (src *CK8sConfigTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*bootstrapv1.CK8sConfigTemplate)
	if !ok {
		return fmt.Errorf("expected *v1beta3.CK8sConfigTemplate, got %T", dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if err := roundTripConvert(src.Spec, &dst.Spec); err != nil {
		return fmt.Errorf("failed to convert template spec to v1beta3: %w", err)
	}

	return nil
}

// ConvertFrom converts from the v1beta3 hub version to this v1beta2 spoke version.
func (dst *CK8sConfigTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*bootstrapv1.CK8sConfigTemplate)
	if !ok {
		return fmt.Errorf("expected *v1beta3.CK8sConfigTemplate, got %T", srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if err := roundTripConvert(src.Spec, &dst.Spec); err != nil {
		return fmt.Errorf("failed to convert template spec from v1beta3: %w", err)
	}

	return nil
}

func roundTripConvert(src any, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
