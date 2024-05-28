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

// Hub marks CK8sConfig as a conversion hub.
func (*CK8sConfig) Hub() {}

// Hub marks CK8sConfigList as a conversion hub.
func (*CK8sConfigList) Hub() {}

// Hub marks CK8sConfigTemplate as a conversion hub.
func (*CK8sConfigTemplate) Hub() {}

// Hub marks CK8sConfigTemplateList as a conversion hub.
func (*CK8sConfigTemplateList) Hub() {}
