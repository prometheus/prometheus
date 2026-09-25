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

package parser

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/features"
)

func TestRegisterFeaturesCustomFunctions(t *testing.T) {
	funcs := map[string]*Function{
		"custom_func": {
			Name:       "custom_func",
			ArgTypes:   []ValueType{ValueTypeMatrix},
			ReturnType: ValueTypeVector,
		},
		"experimental_func": {
			Name:         "experimental_func",
			ArgTypes:     []ValueType{ValueTypeMatrix},
			ReturnType:   ValueTypeVector,
			Experimental: true,
		},
	}

	r := features.NewRegistry()
	NewParser(Options{Functions: funcs}).RegisterFeatures(r)
	require.Equal(t, map[string]bool{
		"custom_func":       true,
		"experimental_func": false,
	}, r.Get()[features.PromQLFunctions])

	r = features.NewRegistry()
	NewParser(Options{}).RegisterFeatures(r)
	require.NotContains(t, r.Get()[features.PromQLFunctions], "custom_func")
	require.Contains(t, r.Get()[features.PromQLFunctions], "rate")
}
