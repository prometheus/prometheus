/*
Copyright 2017 The Kubernetes Authors.

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

package v1beta1

import "k8s.io/apimachinery/pkg/conversion"

// Convert_Slice_string_To_v1beta1_IncludeObjectPolicy allows converting a URL query parameter value
func Convert_Slice_string_To_v1beta1_IncludeObjectPolicy(input *[]string, out *IncludeObjectPolicy, s conversion.Scope) error {
	if len(*input) > 0 {
		*out = IncludeObjectPolicy((*input)[0])
	}
	return nil
}
