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
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func unmarshalYAMLStrict(data []byte, v interface{}) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(v)
}

func TestConfigsCustomUnMarshalMarshal(t *testing.T) {
	input := `static_configs:
- targets:
  - foo:1234
  - bar:4321
`
	cfg := &Configs{}
	err := unmarshalYAMLStrict([]byte(input), cfg)
	require.NoError(t, err)

	output, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.Equal(t, input, string(output))
}
