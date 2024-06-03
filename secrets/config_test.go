// Copyright 2024 The Prometheus Authors
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

package secrets

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	yamlv2 "gopkg.in/yaml.v2"
	yamlv3 "gopkg.in/yaml.v3"
)

type yamlPackage struct {
	Unmarshal func(in []byte, out interface{}) (err error)
	Marshal   func(in interface{}) (out []byte, err error)
}

type testConfig interface {
	Configs() []ProviderConfig
}

type testConfigYAMLv2 struct {
	ConfigsYAMLv2 *Configs `yaml:"configs,omitempty"`
}

func (c *testConfigYAMLv2) Configs() []ProviderConfig {
	if c.ConfigsYAMLv2 == nil {
		return nil
	}
	return *c.ConfigsYAMLv2
}

func TestDecodeYAMLv2(t *testing.T) {
	testDecode(t, yamlPackage{
		Unmarshal: yamlv2.Unmarshal,
		Marshal:   yamlv2.Marshal,
	}, func() *testConfigYAMLv2 {
		return &testConfigYAMLv2{}
	}, true)
}

func TestDecodeYAMLv2WithYAMLv3(t *testing.T) {
	testDecode(t, yamlPackage{
		Unmarshal: yamlv3.Unmarshal,
		Marshal:   yamlv3.Marshal,
	}, func() *testConfigYAMLv2 {
		return &testConfigYAMLv2{}
	}, false)
}

func testDecode[T testConfig](t *testing.T, yaml yamlPackage, newTestConfig func() T, isV2 bool) {
	t.Run("none", func(t *testing.T) {
		c := newTestConfig()
		err := yaml.Unmarshal([]byte(""), c)
		require.NoError(t, err)
		require.Empty(t, c.Configs())

		out, err := yaml.Marshal(c)
		require.NoError(t, err)
		require.Equal(t, "{}\n", string(out))
	})
	t.Run("empty", func(t *testing.T) {
		c := newTestConfig()
		err := yaml.Unmarshal([]byte("configs:\n"), c)
		require.NoError(t, err)
		require.Empty(t, c.Configs())

		out, err := yaml.Marshal(c)
		require.NoError(t, err)
		require.Equal(t, "{}\n", string(out))
	})
	t.Run("invalid", func(t *testing.T) {
		c := newTestConfig()
		err := yaml.Unmarshal([]byte(`
configs:
- test
`), c)
		require.ErrorContains(t, err, "unable to unmarshal secret configurations")
		require.Empty(t, c.Configs())

		out, err := yaml.Marshal(c)
		require.NoError(t, err)
		require.Equal(t, "configs: []\n", string(out))
	})
	t.Run("missing provider", func(t *testing.T) {
		c := testConfigYAMLv2{}
		err := yaml.Unmarshal([]byte(`
configs:
- name: a
`), &c)
		require.ErrorContains(t, err, "expected secret provider but found none")
		require.Empty(t, c.Configs())
	})
	t.Run("multiple providers", func(t *testing.T) {
		c := newTestConfig()
		err := yaml.Unmarshal([]byte(`
configs:
- name: a
  provider1:
    value: i
  provider2: k
`), c)
		require.ErrorContains(t, err, "expected single secret provider but found 2")
		require.Empty(t, c.Configs())
	})
	t.Run("valid", func(t *testing.T) {
		c := newTestConfig()
		config := []byte(`
configs:
- name: a
  provider1: i
- name: b
  provider1:
    value: j
- name: c
  provider2:
    foo: [bar, baz]
    value: k
- name: d
  provider2: [1]
`)
		err := yaml.Unmarshal(config, c)
		require.NoError(t, err)

		for providerIndex := range c.Configs() {
			providerConfig := &c.Configs()[providerIndex]
			for i := range providerConfig.Configs {
				providerConfig.Configs[i].Data = nodeNormalize(providerConfig.Configs[i].Data)
			}
		}

		expected := []ProviderConfig{
			{
				Name: "provider1",
				Configs: []Config[*yamlv3.Node]{
					{
						Name: "a",
						Data: &yamlv3.Node{
							Kind:  yamlv3.ScalarNode,
							Value: "i",
						},
					},
					{
						Name: "b",
						Data: &yamlv3.Node{
							Kind: yamlv3.MappingNode,
							Content: []*yamlv3.Node{
								{
									Kind:  yamlv3.ScalarNode,
									Value: "value",
								},
								{
									Kind:  yamlv3.ScalarNode,
									Value: "j",
								},
							},
						},
					},
				},
			},
			{
				Name: "provider2",
				Configs: []Config[*yamlv3.Node]{
					{
						Name: "c",
						Data: &yamlv3.Node{
							Kind: yamlv3.MappingNode,
							Content: []*yamlv3.Node{
								{
									Kind:  yamlv3.ScalarNode,
									Value: "foo",
								},
								{
									Kind: yamlv3.SequenceNode,
									Content: []*yamlv3.Node{
										{
											Kind:  yamlv3.ScalarNode,
											Value: "bar",
										},
										{
											Kind:  yamlv3.ScalarNode,
											Value: "baz",
										},
									},
								},
								{
									Kind:  yamlv3.ScalarNode,
									Value: "value",
								},
								{
									Kind:  yamlv3.ScalarNode,
									Value: "k",
								},
							},
						},
					},
					{
						Name: "d",
						Data: &yamlv3.Node{
							Kind: yamlv3.SequenceNode,
							Content: []*yamlv3.Node{
								{
									Kind:  yamlv3.ScalarNode,
									Value: "1",
								},
							},
						},
					},
				},
			},
		}

		// See: https://github.com/go-yaml/yaml/issues/661
		// TODO(TheSpiritXIII): Forks fix this to be v2-compatible, e.g. sigs.k8s.io/yaml.
		expectedYAML := []byte(`configs:
    - name: a
      provider1: i
    - name: b
      provider1:
        value: j
    - name: c
      provider2:
        foo:
            - bar
            - baz
        value: k
    - name: d
      provider2:
        - 1
`)

		if isV2 {
			expectedYAML = []byte(`configs:
- name: a
  provider1: i
- name: b
  provider1:
    value: j
- name: c
  provider2:
    foo:
    - bar
    - baz
    value: k
- name: d
  provider2:
  - 1
`)
		}
		if d := cmp.Diff(expected, c.Configs()); d != "" {
			t.Errorf("expected %v, got %v: %s", expected, c.Configs(), d)
		}

		out, err := yaml.Marshal(&c)
		require.NoError(t, err)
		require.Equal(t, string(expectedYAML), string(out))
	})
}

// nodeNormalize removes all formatting metadata from the given node and its children.
func nodeNormalize(node *yamlv3.Node) *yamlv3.Node {
	var content []*yamlv3.Node
	for _, node := range node.Content {
		content = append(content, nodeNormalize(node))
	}
	return &yamlv3.Node{
		Kind:    node.Kind,
		Value:   node.Value,
		Content: content,
	}
}
