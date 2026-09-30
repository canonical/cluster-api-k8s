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
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	controlplanev1 "github.com/canonical/cluster-api-k8s/controlplane/api/v1beta3"
)

// ConvertTo converts this CK8sControlPlane from the v1beta2 spoke version to the v1beta3 hub version.
func (src *CK8sControlPlane) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*controlplanev1.CK8sControlPlane)
	if !ok {
		return fmt.Errorf("expected *v1beta3.CK8sControlPlane, got %T", dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if err := roundTripConvert(src.Spec, &dst.Spec); err != nil {
		return fmt.Errorf("failed to convert spec to v1beta3: %w", err)
	}

	if err := roundTripConvert(src.Status, &dst.Status); err != nil {
		return fmt.Errorf("failed to convert status to v1beta3: %w", err)
	}

	updated := src.Status.UpdatedReplicas
	dst.Status.UpToDateReplicas = &updated

	ready := src.Status.ReadyReplicas
	dst.Status.ReadyReplicas = &ready

	if src.Status.Initialized {
		initialized := true
		dst.Status.Initialization.ControlPlaneInitialized = &initialized
	}

	// v1beta2 has no AvailableReplicas field; preserve legacy behavior by mirroring ReadyReplicas.
	dst.Status.AvailableReplicas = &ready

	if len(src.Status.Conditions) > 0 {
		convertedConditions := make([]metav1.Condition, 0, len(src.Status.Conditions))
		if err := roundTripConvert(src.Status.Conditions, &convertedConditions); err != nil {
			return fmt.Errorf("failed to convert conditions to v1beta3: %w", err)
		}
		dst.Status.Conditions = convertedConditions
	}

	return nil
}

// ConvertFrom converts from the v1beta3 hub version to this v1beta2 spoke version.
func (dst *CK8sControlPlane) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*controlplanev1.CK8sControlPlane)
	if !ok {
		return fmt.Errorf("expected *v1beta3.CK8sControlPlane, got %T", srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta

	if err := roundTripConvert(src.Spec, &dst.Spec); err != nil {
		return fmt.Errorf("failed to convert spec from v1beta3: %w", err)
	}

	if err := roundTripConvert(src.Status, &dst.Status); err != nil {
		return fmt.Errorf("failed to convert status from v1beta3: %w", err)
	}

	if src.Status.UpToDateReplicas != nil {
		dst.Status.UpdatedReplicas = *src.Status.UpToDateReplicas
	}

	if src.Status.ReadyReplicas != nil {
		dst.Status.ReadyReplicas = *src.Status.ReadyReplicas
		dst.Status.Ready = *src.Status.ReadyReplicas > 0
	}

	if src.Status.Initialization.ControlPlaneInitialized != nil {
		dst.Status.Initialized = *src.Status.Initialization.ControlPlaneInitialized
	}

	if len(src.Status.Conditions) > 0 {
		if err := roundTripConvert(src.Status.Conditions, &dst.Status.Conditions); err != nil {
			return fmt.Errorf("failed to convert conditions from v1beta3: %w", err)
		}
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
