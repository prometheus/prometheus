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

package consul

import "testing"

func TestShouldWatch(t *testing.T) {
	for _, tc := range []struct {
		Discovery      *Discovery
		ShouldMatch    []string
		ShouldNotMatch []string
	}{
		{ // empty rules match everything
			Discovery:   &Discovery{},
			ShouldMatch: []string{"foo", "bar", "baz"},
		},
		{ // exact matches
			Discovery: &Discovery{
				watchedServices: []string{"foo", "bar", "baz"},
			},
			ShouldMatch:    []string{"foo", "bar", "baz"},
			ShouldNotMatch: []string{"zab", "aar"},
		},
		{ // regexp matches
			Discovery: &Discovery{
				filter: ".*a.*",
			},
			ShouldMatch:    []string{"bar", "baz", "zab"},
			ShouldNotMatch: []string{"foo"},
		},
		{ // exact matches and regexp
			Discovery: &Discovery{
				watchedServices: []string{"foo", "oof"},
				filter:          "ba.+",
			},
			ShouldMatch:    []string{"foo", "oof", "bar", "baz"},
			ShouldNotMatch: []string{"zab", "ba"},
		},
	} {
		for _, svc := range tc.ShouldMatch {
			if !tc.Discovery.shouldWatch(svc) {
				t.Errorf("Should watch service %s", svc)
			}
		}
		for _, svc := range tc.ShouldNotMatch {
			if tc.Discovery.shouldWatch(svc) {
				t.Errorf("Should not watch service %s", svc)
			}
		}
	}
}
