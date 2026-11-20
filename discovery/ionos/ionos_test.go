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

package ionos

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestSDConfigUnmarshalYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		input           string
		expectedFilters []*Filter
		wantErr         string
	}{
		{
			name:  "NoFilters",
			input: "datacenter_id: dc-1\n",
		},
		{
			name: "ValidFilters",
			input: "datacenter_id: dc-1\n" +
				"filters:\n" +
				"  - name: metadata.state\n" +
				"    values: [AVAILABLE]\n" +
				"  - name: properties.cores\n" +
				"    values: [\"2\", \"4\"]\n",
			expectedFilters: []*Filter{
				{Name: "metadata.state", Values: []string{"AVAILABLE"}},
				{Name: "properties.cores", Values: []string{"2", "4"}},
			},
		},
		{
			name: "FilterWithEmptyValues",
			input: "datacenter_id: dc-1\n" +
				"filters:\n" +
				"  - name: metadata.state\n" +
				"    values: []\n",
			wantErr: "ionos SD configuration filter values cannot be empty",
		},
		{
			name:    "MissingDatacenterID",
			input:   "port: 80\n",
			wantErr: "datacenter id can't be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cfg SDConfig
			err := yaml.Unmarshal([]byte(tc.input), &cfg)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedFilters, cfg.Filters)
		})
	}
}
