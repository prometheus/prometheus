// Copyright 2016 The Prometheus Authors
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
	"bytes"
	"encoding/json"
	"testing"
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
		json, err := json.Marshal(tt.tv)
		if err != nil {
			t.Errorf("%d. Marshal(%q) returned err: %s", i, tt.tv, err)
		} else {
			if !bytes.Equal(json, tt.json) {
				t.Errorf(
					"%d. Marshal(%q) => %q, want %q",
					i, tt.tv, json, tt.json,
				)
			}
		}
	}
}

func TestTagValueUnMarshaling(t *testing.T) {
	for i, tt := range stringtests {
		var tv TagValue
		err := json.Unmarshal(tt.json, &tv)
		if err != nil {
			t.Errorf("%d. Unmarshal(%q, &str) returned err: %s", i, tt.json, err)
		} else {
			if tv != tt.tv {
				t.Errorf(
					"%d. Unmarshal(%q, &str) => str==%q, want %q",
					i, tt.json, tv, tt.tv,
				)
			}
		}
	}
}
