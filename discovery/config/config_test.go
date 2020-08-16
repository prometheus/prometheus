// Copyright 2020 The Prometheus Authors
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

package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
	"gopkg.in/yaml.v2"
)

func TestForNilSDConfig(t *testing.T) {
	// Get all the yaml fields names of the ServiceDiscoveryConfig struct.
	s := reflect.ValueOf(ServiceDiscoveryConfig{})
	configType := s.Type()
	n := s.NumField()
	fieldsSlice := make([]string, n)
	for i := 0; i < n; i++ {
		field := configType.Field(i)
		tag := field.Tag.Get("yaml")
		tag = strings.Split(tag, ",")[0]
		fieldsSlice = append(fieldsSlice, tag)
	}

	// Unmarshall all possible yaml keys and validate errors check upon nil
	// SD config.
	for _, f := range fieldsSlice {
		if f == "" {
			continue
		}
		t.Run(f, func(t *testing.T) {
			c := &ServiceDiscoveryConfig{}
			err := yaml.Unmarshal([]byte(fmt.Sprintf(`
---
%s:
-
`, f)), c)
			testutil.Ok(t, err)
			err = c.Validate()
			testutil.NotOk(t, err)
			testutil.Equals(t, fmt.Sprintf("empty or null section in %s", f), err.Error())
		})
	}
}
