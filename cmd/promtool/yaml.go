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

package main

import (
	"bytes"
	"errors"
	"io"

	"go.yaml.in/yaml/v4"
)

// loadYAML preserves the first-document and empty-input behavior of v2's
// UnmarshalStrict while using the v4 Loader API.
func loadYAML(data []byte, out any) error {
	loader, err := yaml.NewLoader(bytes.NewReader(data), yaml.WithV2Defaults(), yaml.WithKnownFields())
	if err != nil {
		return err
	}
	err = loader.Load(out)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
