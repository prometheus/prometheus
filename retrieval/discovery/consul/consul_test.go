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
