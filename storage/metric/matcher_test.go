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
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/prometheus/common/model"
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

func TestLabelMatchersSort(t *testing.T) {
	// Line up Matchers in expected order:
	want := LabelMatchers{
		mustNewLabelMatcher(Equal, model.InstanceLabel, "not empty"),
		mustNewLabelMatcher(Equal, model.MetricNameLabel, "a:recording:rule"),
		mustNewLabelMatcher(Equal, model.MetricNameLabel, "up"),
		mustNewLabelMatcher(Equal, "nothing_special but much longer", "not empty"),
		mustNewLabelMatcher(Equal, "nothing_special", "not empty but longer"),
		mustNewLabelMatcher(Equal, "nothing_special", "not empty"),
		mustNewLabelMatcher(Equal, model.JobLabel, "not empty"),
		mustNewLabelMatcher(Equal, model.BucketLabel, "not empty"),
		mustNewLabelMatcher(RegexMatch, "irrelevant", "does not match empty string and is longer"),
		mustNewLabelMatcher(RegexMatch, "irrelevant", "does not match empty string"),
		mustNewLabelMatcher(RegexNoMatch, "irrelevant", "(matches empty string)?"),
		mustNewLabelMatcher(RegexNoMatch, "irrelevant", "(matches empty string with a longer expression)?"),
		mustNewLabelMatcher(NotEqual, "irrelevant", ""),
		mustNewLabelMatcher(Equal, "irrelevant", ""),
	}
	got := make(LabelMatchers, len(want))
	for i, j := range rand.Perm(len(want)) {
		got[i] = want[j]
	}
	sort.Sort(got)
	if !reflect.DeepEqual(want, got) {
		t.Errorf("unexpected sorting of matchers, got %v, want %v", got, want)
	}
}

func mustNewLabelMatcher(mt MatchType, name model.LabelName, val model.LabelValue) *LabelMatcher {
	m, err := NewLabelMatcher(mt, name, val)
	if err != nil {
		panic(err)
	}
	return m
}
