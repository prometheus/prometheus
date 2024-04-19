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

package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/prometheus/prometheus/secrets"
)

func TestMapString(t *testing.T) {
	baseProvider := SecretDataProvider(func(ctx context.Context, value string) (string, error) {
		return value, nil
	})
	provider := MapNode(baseProvider)
	t.Run("valid", func(t *testing.T) {
		s, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: "foo",
				},
			},
			{
				Name: "y",
				Data: &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: "bar",
				},
			},
			{
				Name: "z",
				Data: &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: "baz",
				},
			},
		})

		ctx := context.Background()
		require.NoError(t, err)
		require.Len(t, s, 3)
		data, err := s[0].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "foo", data)
		data, err = s[1].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "bar", data)
		data, err = s[2].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "baz", data)
	})
	t.Run("empty", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: nil,
			},
		})
		require.ErrorContains(t, err, "yaml: empty node")
	})
	t.Run("array", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind: yaml.SequenceNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "foo",
						},
					},
				},
			},
		})
		require.ErrorContains(t, err, "yaml: unmarshal errors:\n")
	})
	t.Run("object", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "value",
						},
						{
							Kind:  yaml.ScalarNode,
							Value: "foo",
						},
					},
				},
			},
		})
		require.ErrorContains(t, err, "yaml: unmarshal errors:\n")
	})
}

func TestMapArray(t *testing.T) {
	baseProvider := SecretDataProvider(func(ctx context.Context, value []string) (string, error) {
		return strings.Join(value, ","), nil
	})
	provider := MapNode(baseProvider)
	t.Run("valid", func(t *testing.T) {
		secrets, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind: yaml.SequenceNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "foo",
						},
						{
							Kind:  yaml.ScalarNode,
							Value: "bar",
						},
						{
							Kind:  yaml.ScalarNode,
							Value: "baz",
						},
					},
				},
			},
			{
				Name: "y",
				Data: &yaml.Node{
					Kind: yaml.SequenceNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "apple",
						},
					},
				},
			},
			{
				Name: "z",
				Data: &yaml.Node{
					Kind: yaml.SequenceNode,
				},
			},
		})

		ctx := context.Background()
		require.NoError(t, err)
		require.Len(t, secrets, 3)
		data, err := secrets[0].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "foo,bar,baz", data)
		data, err = secrets[1].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "apple", data)
		data, err = secrets[2].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "", data)
	})
	t.Run("empty", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: nil,
			},
		})
		require.ErrorContains(t, err, "yaml: empty node")
	})
	t.Run("string", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: "foo",
				},
			},
		})
		require.ErrorContains(t, err, "yaml: unmarshal errors:\n")
	})
	t.Run("object", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "value",
						},
						{
							Kind:  yaml.ScalarNode,
							Value: "foo",
						},
					},
				},
			},
		})
		require.ErrorContains(t, err, "yaml: unmarshal errors:\n")
	})
}

func TestMapObject(t *testing.T) {
	baseProvider := SecretDataProvider(func(ctx context.Context, data testData) (string, error) {
		return data.Value, nil
	})
	provider := MapNode(baseProvider)
	t.Run("valid", func(t *testing.T) {
		secrets, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "value",
						},
						{
							Kind:  yaml.ScalarNode,
							Value: "foo",
						},
					},
				},
			},
			{
				Name: "y",
				Data: &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "value",
						},
						{
							Kind:  yaml.ScalarNode,
							Value: "bar",
						},
					},
				},
			},
			{
				Name: "z",
				Data: &yaml.Node{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "value",
						},
						{
							Kind:  yaml.ScalarNode,
							Value: "baz",
						},
					},
				},
			},
		})

		ctx := context.Background()
		require.NoError(t, err)
		require.Len(t, secrets, 3)
		data, err := secrets[0].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "foo", data)
		data, err = secrets[1].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "bar", data)
		data, err = secrets[2].Fetch(ctx)
		require.NoError(t, err)
		require.Equal(t, "baz", data)
	})
	t.Run("empty", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: nil,
			},
		})
		require.ErrorContains(t, err, "yaml: empty node")
	})
	t.Run("string", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: "foo",
				},
			},
		})
		require.ErrorContains(t, err, "yaml: unmarshal errors:\n")
	})
	t.Run("array", func(t *testing.T) {
		_, err := provider.Apply([]secrets.Config[*yaml.Node]{
			{
				Name: "x",
				Data: &yaml.Node{
					Kind: yaml.SequenceNode,
					Content: []*yaml.Node{
						{
							Kind:  yaml.ScalarNode,
							Value: "foo",
						},
					},
				},
			},
		})
		require.ErrorContains(t, err, "yaml: unmarshal errors:\n")
	})
}

type testData struct {
	Value string `yaml:"value"`
}
