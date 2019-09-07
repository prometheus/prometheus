// Copyright 2019 The Prometheus Authors
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
	"bytes"

	"gopkg.in/yaml.v3"
)

// NewDecoder returs yaml.Decoder with knownFields set to true
func NewDecoder(content []byte) *yaml.Decoder {
	r := bytes.NewReader([]byte(content))
	d := yaml.NewDecoder(r)
	d.KnownFields(true)
	return d
}

// NewEncoder returns a new yaml.Encoder with indent set to 2
func NewEncoder() (*yaml.Encoder, *bytes.Buffer) {
	var buf bytes.Buffer
	e := yaml.NewEncoder(&buf)
	e.SetIndent(2)
	return e, &buf
}

// Marshal serializes the value provided into a YAML document.
func Marshal(in interface{}) (*bytes.Buffer, error) {
	e, b := NewEncoder()
	defer e.Close()
	err := e.Encode(in)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Unmarshal decodes the yaml document.
func Unmarshal(in []byte, out interface{}) (err error) {
	d := NewDecoder(in)
	err = d.Decode(out)
	if err != nil {
		return err
	}
	return nil
}
