// Copyright 2015 The Prometheus Authors
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
	"errors"
	"io"

	"gopkg.in/yaml.v3"
)

// UnmarshalStrict unmarshals YAML content into the target
// struct, enforces strict field checking, and ignores EOF errors.
func UnmarshalStrict(content []byte, out interface{}) error {
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true) // Enables strict mode to reject unknown fields.

	err := decoder.Decode(out)
	if errors.Is(err, io.EOF) {
		return nil // Ignore EOF error.
	}
	return err
}
