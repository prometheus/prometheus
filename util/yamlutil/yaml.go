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

package yamlutil

import (
	"errors"
	"fmt"

	yamlv2 "go.yaml.in/yaml/v2"
	yamlv3 "go.yaml.in/yaml/v3"
)

// UnmarshalStrict unmarshals a YAML document while rejecting unknown fields.
// YAML merge keys are expanded before strict decoding so that explicit keys can
// override values inherited from an anchor without being reported as duplicates.
func UnmarshalStrict(in []byte, out any) error {
	normalized, err := resolveMergeKeys(in)
	if err != nil {
		return err
	}
	return yamlv2.UnmarshalStrict(normalized, out)
}

func resolveMergeKeys(in []byte) ([]byte, error) {
	var document yamlv3.Node
	if err := yamlv3.Unmarshal(in, &document); err != nil {
		return nil, err
	}

	changed, err := resolveNode(&document, map[*yamlv3.Node]bool{})
	if err != nil || !changed {
		return in, err
	}
	return yamlv3.Marshal(&document)
}

func resolveNode(node *yamlv3.Node, visiting map[*yamlv3.Node]bool) (bool, error) {
	if node == nil {
		return false, nil
	}
	if node.Kind == yamlv3.AliasNode {
		if visiting[node] {
			return false, fmt.Errorf("yaml: anchor %q contains itself", node.Value)
		}
		visiting[node] = true
		changed, err := resolveNode(node.Alias, visiting)
		delete(visiting, node)
		return changed, err
	}

	changed := false
	for _, child := range node.Content {
		childChanged, err := resolveNode(child, visiting)
		if err != nil {
			return false, err
		}
		changed = changed || childChanged
	}
	if node.Kind != yamlv3.MappingNode {
		return changed, nil
	}

	explicitKeys := map[string]struct{}{}
	var explicitContent []*yamlv3.Node
	var mergeValues []*yamlv3.Node
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Tag == "!!merge" {
			mergeValues = append(mergeValues, value)
			continue
		}
		explicitKeys[nodeKey(key)] = struct{}{}
		explicitContent = append(explicitContent, key, value)
	}
	if len(mergeValues) == 0 {
		return changed, nil
	}

	mergedKeys := map[string]struct{}{}
	var mergedContent []*yamlv3.Node
	for _, value := range mergeValues {
		mappings, err := mergeMappings(value)
		if err != nil {
			return false, err
		}
		for _, mapping := range mappings {
			for i := 0; i < len(mapping.Content); i += 2 {
				key := nodeKey(mapping.Content[i])
				if _, ok := explicitKeys[key]; ok {
					continue
				}
				if _, ok := mergedKeys[key]; ok {
					continue
				}
				mergedKeys[key] = struct{}{}
				mergedContent = append(mergedContent, mapping.Content[i], mapping.Content[i+1])
			}
		}
	}
	node.Content = append(mergedContent, explicitContent...)
	return true, nil
}

func mergeMappings(node *yamlv3.Node) ([]*yamlv3.Node, error) {
	if node.Kind == yamlv3.AliasNode {
		node = node.Alias
	}
	switch node.Kind {
	case yamlv3.MappingNode:
		return []*yamlv3.Node{node}, nil
	case yamlv3.SequenceNode:
		mappings := make([]*yamlv3.Node, 0, len(node.Content))
		for _, child := range node.Content {
			if child.Kind == yamlv3.AliasNode {
				child = child.Alias
			}
			if child.Kind != yamlv3.MappingNode {
				return nil, errors.New("yaml: map merge requires a map or sequence of maps")
			}
			mappings = append(mappings, child)
		}
		return mappings, nil
	default:
		return nil, errors.New("yaml: map merge requires a map or sequence of maps")
	}
}

func nodeKey(node *yamlv3.Node) string {
	return node.Tag + "\x00" + node.Value
}
