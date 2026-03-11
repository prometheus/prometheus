// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package template

import (
	"math"
	"testing"
)

// Some test cases rely upon architecture-specific behaviors with respect
// to numerical conversions. The logic remains the same across architectures,
// but outputs can vary, so the cases are only run on amd64.
// See https://github.com/prometheus/prometheus/issues/10185 for more details.
func TestTemplateExpansionAMD64(t *testing.T) {
	testTemplateExpansion(t, []scenario{
		{
			// HumanizeDuration - MaxInt64.
			text:   "{{ humanizeDuration . }}",
			input:  math.MaxInt64,
			output: "-106751991167300d -15h -30m -8s",
		},
		{
			// HumanizeDuration - MaxUint64.
			text:   "{{ humanizeDuration . }}",
			input:  uint(math.MaxUint64),
			output: "-106751991167300d -15h -30m -8s",
		},
	})
}
