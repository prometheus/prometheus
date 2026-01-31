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

package discovery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/yamlutil"
)

func TestConfigsCustomUnMarshalMarshal(t *testing.T) {
	input := `static_configs:
- targets:
  - foo:1234
  - bar:4321
`
	cfg := &Configs{}
	err := yamlutil.UnmarshalStrict([]byte(input), cfg)
	require.NoError(t, err)

	output, err := yamlutil.Marshal(cfg)
	require.NoError(t, err)
	// yaml/v4 uses different sequence formatting than v2
	expected := `static_configs:
  - targets:
      - foo:1234
      - bar:4321
`
	require.Equal(t, expected, string(output))
}
