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

// Package yamlutil provides YAML utility functions.
package yamlutil

import (
	"bytes"
	"errors"
	"io"

	"go.yaml.in/yaml/v4"
)

// UnmarshalStrict is like yaml.Unmarshal but returns an error if there are
// keys in the data that do not have corresponding struct members.
func UnmarshalStrict(in []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(in))
	dec.KnownFields(true)
	err := dec.Decode(out)
	// yaml/v4 returns io.EOF for empty input, but for compatibility with v2
	// behavior we treat this as success (no error).
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// Marshal marshals v into YAML with 2-space indentation.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
