// Copyright 2014 The Prometheus Authors
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

package metric

import (
	"testing"
)

func TestAnchoredMatcher(t *testing.T) {
	m, err := NewLabelMatcher(RegexMatch, "", "foo|bar|baz")
	if err != nil {
		t.Fatal(err)
	}

	if !m.Match("foo") {
		t.Errorf("Expected match for %q but got none", "foo")
	}
	if m.Match("fooo") {
		t.Errorf("Unexpected match for %q", "fooo")
	}
}
