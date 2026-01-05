// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package opentsdb

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

var stringtests = []struct {
	tv   TagValue
	json []byte
}{
	{TagValue("foo-bar-42"), []byte(`"foo-bar-42"`)},
	{TagValue("foo_bar_42"), []byte(`"foo__bar__42"`)},
	{TagValue("http://example.org:8080"), []byte(`"http_.//example.org_.8080"`)},
	{TagValue("Björn's email: bjoern@soundcloud.com"), []byte(`"Bj_C3_B6rn_27s_20email_._20bjoern_40soundcloud.com"`)},
	{TagValue("日"), []byte(`"_E6_97_A5"`)},
}

func TestTagValueMarshaling(t *testing.T) {
	for i, tt := range stringtests {
		got, err := json.Marshal(tt.tv)
		require.NoError(t, err, "%d. Marshal(%q) returned error.", i, tt.tv)
		require.Equal(t, tt.json, got, "%d. Marshal(%q) not equal.", i, tt.tv)
	}
}

func TestTagValueUnMarshaling(t *testing.T) {
	for i, tt := range stringtests {
		var tv TagValue
		err := json.Unmarshal(tt.json, &tv)
		require.NoError(t, err, "%d. Unmarshal(%q, &str) returned error.", i, tt.json)
		require.Equal(t, tt.tv, tv, "%d. Unmarshal(%q, &str) not equal.", i, tt.json)
	}
}
