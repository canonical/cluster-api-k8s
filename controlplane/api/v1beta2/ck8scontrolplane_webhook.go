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
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupWebhookWithManager will setup the webhooks for the CK8sControlPlane.
func (in *CK8sControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).
		WithDefaulter(&CK8sControlPlane{}).
		WithValidator(&CK8sControlPlane{}).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1beta2-ck8scontrolplane,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=ck8scontrolplanes,versions=v1beta2,name=validation.ck8scontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1
// +kubebuilder:webhook:verbs=create;update,path=/mutate-controlplane-cluster-x-k8s-io-v1beta2-ck8scontrolplane,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=ck8scontrolplanes,versions=v1beta2,name=default.ck8scontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ admission.CustomDefaulter = &CK8sControlPlane{}
var _ admission.CustomValidator = &CK8sControlPlane{}

// ValidateCreate will do any extra validation when creating a CK8sControlPlane.
func (in *CK8sControlPlane) ValidateCreate(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return []string{}, nil
}

// ValidateUpdate will do any extra validation when updating a CK8sControlPlane.
func (in *CK8sControlPlane) ValidateUpdate(_ context.Context, _, _ runtime.Object) (admission.Warnings, error) {
	return []string{}, nil
}

// ValidateDelete allows you to add any extra validation when deleting.
func (in *CK8sControlPlane) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return []string{}, nil
}

// Default will set default values for the CK8sControlPlane.
func (in *CK8sControlPlane) Default(_ context.Context, obj runtime.Object) error {
	c, ok := obj.(*CK8sControlPlane)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a KubeadmConfig but got a %T", obj))
	}

	defaultCK8sControlPlaneSpec(&c.Spec, c.Namespace)
	return nil
}

func defaultCK8sControlPlaneSpec(s *CK8sControlPlaneSpec, namespace string) {
	if s.Replicas == nil {
		replicas := int32(1)
		s.Replicas = &replicas
	}

	if s.MachineTemplate.InfrastructureRef.Namespace == "" {
		s.MachineTemplate.InfrastructureRef.Namespace = namespace
	}
}
