//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1beta2

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cluster-api/api/v1beta1"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlane) DeepCopyInto(out *CK8sControlPlane) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlane.
func (in *CK8sControlPlane) DeepCopy() *CK8sControlPlane {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlane)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CK8sControlPlane) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneList) DeepCopyInto(out *CK8sControlPlaneList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CK8sControlPlane, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneList.
func (in *CK8sControlPlaneList) DeepCopy() *CK8sControlPlaneList {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CK8sControlPlaneList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneMachineTemplate) DeepCopyInto(out *CK8sControlPlaneMachineTemplate) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.InfrastructureRef = in.InfrastructureRef
	if in.NodeDrainTimeout != nil {
		in, out := &in.NodeDrainTimeout, &out.NodeDrainTimeout
		*out = new(v1.Duration)
		**out = **in
	}
	if in.NodeVolumeDetachTimeout != nil {
		in, out := &in.NodeVolumeDetachTimeout, &out.NodeVolumeDetachTimeout
		*out = new(v1.Duration)
		**out = **in
	}
	if in.NodeDeletionTimeout != nil {
		in, out := &in.NodeDeletionTimeout, &out.NodeDeletionTimeout
		*out = new(v1.Duration)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneMachineTemplate.
func (in *CK8sControlPlaneMachineTemplate) DeepCopy() *CK8sControlPlaneMachineTemplate {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneMachineTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneSpec) DeepCopyInto(out *CK8sControlPlaneSpec) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.CK8sConfigSpec.DeepCopyInto(&out.CK8sConfigSpec)
	if in.RolloutAfter != nil {
		in, out := &in.RolloutAfter, &out.RolloutAfter
		*out = (*in).DeepCopy()
	}
	in.MachineTemplate.DeepCopyInto(&out.MachineTemplate)
	if in.RemediationStrategy != nil {
		in, out := &in.RemediationStrategy, &out.RemediationStrategy
		*out = new(RemediationStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneSpec.
func (in *CK8sControlPlaneSpec) DeepCopy() *CK8sControlPlaneSpec {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneStatus) DeepCopyInto(out *CK8sControlPlaneStatus) {
	*out = *in
	if in.Version != nil {
		in, out := &in.Version, &out.Version
		*out = new(string)
		**out = **in
	}
	if in.FailureMessage != nil {
		in, out := &in.FailureMessage, &out.FailureMessage
		*out = new(string)
		**out = **in
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make(v1beta1.Conditions, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.LastRemediation != nil {
		in, out := &in.LastRemediation, &out.LastRemediation
		*out = new(LastRemediationStatus)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneStatus.
func (in *CK8sControlPlaneStatus) DeepCopy() *CK8sControlPlaneStatus {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneTemplate) DeepCopyInto(out *CK8sControlPlaneTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneTemplate.
func (in *CK8sControlPlaneTemplate) DeepCopy() *CK8sControlPlaneTemplate {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CK8sControlPlaneTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneTemplateList) DeepCopyInto(out *CK8sControlPlaneTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CK8sControlPlaneTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneTemplateList.
func (in *CK8sControlPlaneTemplateList) DeepCopy() *CK8sControlPlaneTemplateList {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CK8sControlPlaneTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneTemplateResource) DeepCopyInto(out *CK8sControlPlaneTemplateResource) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneTemplateResource.
func (in *CK8sControlPlaneTemplateResource) DeepCopy() *CK8sControlPlaneTemplateResource {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneTemplateResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneTemplateResourceSpec) DeepCopyInto(out *CK8sControlPlaneTemplateResourceSpec) {
	*out = *in
	in.CK8sConfigSpec.DeepCopyInto(&out.CK8sConfigSpec)
	if in.RolloutAfter != nil {
		in, out := &in.RolloutAfter, &out.RolloutAfter
		*out = (*in).DeepCopy()
	}
	in.MachineTemplate.DeepCopyInto(&out.MachineTemplate)
	if in.RemediationStrategy != nil {
		in, out := &in.RemediationStrategy, &out.RemediationStrategy
		*out = new(RemediationStrategy)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneTemplateResourceSpec.
func (in *CK8sControlPlaneTemplateResourceSpec) DeepCopy() *CK8sControlPlaneTemplateResourceSpec {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneTemplateResourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CK8sControlPlaneTemplateSpec) DeepCopyInto(out *CK8sControlPlaneTemplateSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CK8sControlPlaneTemplateSpec.
func (in *CK8sControlPlaneTemplateSpec) DeepCopy() *CK8sControlPlaneTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(CK8sControlPlaneTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LastRemediationStatus) DeepCopyInto(out *LastRemediationStatus) {
	*out = *in
	in.Timestamp.DeepCopyInto(&out.Timestamp)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LastRemediationStatus.
func (in *LastRemediationStatus) DeepCopy() *LastRemediationStatus {
	if in == nil {
		return nil
	}
	out := new(LastRemediationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RemediationStrategy) DeepCopyInto(out *RemediationStrategy) {
	*out = *in
	if in.MaxRetry != nil {
		in, out := &in.MaxRetry, &out.MaxRetry
		*out = new(int32)
		**out = **in
	}
	out.RetryPeriod = in.RetryPeriod
	if in.MinHealthyPeriod != nil {
		in, out := &in.MinHealthyPeriod, &out.MinHealthyPeriod
		*out = new(v1.Duration)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RemediationStrategy.
func (in *RemediationStrategy) DeepCopy() *RemediationStrategy {
	if in == nil {
		return nil
	}
	out := new(RemediationStrategy)
	in.DeepCopyInto(out)
	return out
}
