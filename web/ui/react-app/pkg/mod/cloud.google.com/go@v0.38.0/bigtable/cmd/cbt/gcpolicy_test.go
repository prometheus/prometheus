/*
Copyright 2015 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigtable"
	"github.com/google/go-cmp/cmp"
)

func TestParseGCPolicy(t *testing.T) {
	for _, test := range []struct {
		in   string
		want bigtable.GCPolicy
	}{
		{
			"never",
			bigtable.NoGcPolicy(),
		},
		{
			"maxage=3h",
			bigtable.MaxAgePolicy(3 * time.Hour),
		},
		{
			"maxversions=2",
			bigtable.MaxVersionsPolicy(2),
		},
		{
			"maxversions=2 and maxage=1h",
			bigtable.IntersectionPolicy(bigtable.MaxVersionsPolicy(2), bigtable.MaxAgePolicy(time.Hour)),
		},
		{
			"(((maxversions=2 and (maxage=1h))))",
			bigtable.IntersectionPolicy(bigtable.MaxVersionsPolicy(2), bigtable.MaxAgePolicy(time.Hour)),
		},
		{
			"maxversions=7 or maxage=8h",
			bigtable.UnionPolicy(bigtable.MaxVersionsPolicy(7), bigtable.MaxAgePolicy(8*time.Hour)),
		},
		{
			"maxversions = 7||maxage = 8h",
			bigtable.UnionPolicy(bigtable.MaxVersionsPolicy(7), bigtable.MaxAgePolicy(8*time.Hour)),
		},
		{
			"maxversions=7||maxage=8h",
			bigtable.UnionPolicy(bigtable.MaxVersionsPolicy(7), bigtable.MaxAgePolicy(8*time.Hour)),
		},
		{
			"maxage=30d || (maxage=3d && maxversions=100)",
			bigtable.UnionPolicy(
				bigtable.MaxAgePolicy(30*24*time.Hour),
				bigtable.IntersectionPolicy(
					bigtable.MaxAgePolicy(3*24*time.Hour),
					bigtable.MaxVersionsPolicy(100))),
		},
		{
			"maxage=30d || (maxage=3d && maxversions=100) || maxversions=7",
			bigtable.UnionPolicy(
				bigtable.UnionPolicy(
					bigtable.MaxAgePolicy(30*24*time.Hour),
					bigtable.IntersectionPolicy(
						bigtable.MaxAgePolicy(3*24*time.Hour),
						bigtable.MaxVersionsPolicy(100))),
				bigtable.MaxVersionsPolicy(7)),
		},
		{
			// && and || have same precedence, left associativity
			"maxage=1h && maxage=2h || maxage=3h",
			bigtable.UnionPolicy(
				bigtable.IntersectionPolicy(
					bigtable.MaxAgePolicy(1*time.Hour),
					bigtable.MaxAgePolicy(2*time.Hour)),
				bigtable.MaxAgePolicy(3*time.Hour)),
		},
	} {
		got, err := parseGCPolicy(test.in)
		if err != nil {
			t.Errorf("%s: %v", test.in, err)
			continue
		}
		if !cmp.Equal(got, test.want, cmp.AllowUnexported(bigtable.IntersectionPolicy(), bigtable.UnionPolicy())) {
			t.Errorf("%s: got %+v, want %+v", test.in, got, test.want)
		}
	}
}

func TestParseGCPolicyErrors(t *testing.T) {
	for _, in := range []string{
		"",
		"a",
		"b = 1h",
		"c = 1",
		"maxage=1",       // need duration
		"maxversions=1h", // need int
		"maxage",
		"maxversions",
		"never=never",
		"maxversions=1 && never",
		"(((maxage=1h))",
		"((maxage=1h)))",
		"maxage=30d || ((maxage=3d && maxversions=100)",
		"maxversions = 3 and",
	} {
		_, err := parseGCPolicy(in)
		if err == nil {
			t.Errorf("%s: got nil, want error", in)
		}
	}
}

func TestTokenizeGCPolicy(t *testing.T) {
	for _, test := range []struct {
		in   string
		want []string
	}{
		{
			"maxage=5d",
			[]string{"maxage", "=", "5d"},
		},
		{
			"maxage = 5d",
			[]string{"maxage", "=", "5d"},
		},
		{
			"maxage=5d or maxversions=5",
			[]string{"maxage", "=", "5d", "or", "maxversions", "=", "5"},
		},
		{
			"maxage=5d || (maxversions=5)",
			[]string{"maxage", "=", "5d", "||", "(", "maxversions", "=", "5", ")"},
		},
		{
			"maxage=5d||( maxversions=5 )",
			[]string{"maxage", "=", "5d", "||", "(", "maxversions", "=", "5", ")"},
		},
	} {
		got, err := tokenizeGCPolicy(test.in)
		if err != nil {
			t.Errorf("%s: %v", test.in, err)
			continue
		}
		if diff := cmp.Diff(got, test.want); diff != "" {
			t.Errorf("%s: %s", test.in, diff)
		}
	}
}

func TestTokenizeGCPolicyErrors(t *testing.T) {
	for _, in := range []string{
		"a &",
		"a & b",
		"a &x b",
		"a |",
		"a | b",
		"a |& b",
		"a % b",
	} {
		_, err := tokenizeGCPolicy(in)
		if err == nil {
			t.Errorf("%s: got nil, want error", in)
		}
	}
}

func tokenizeGCPolicy(s string) ([]string, error) {
	var tokens []string
	r := strings.NewReader(s)
	for {
		tok, err := getToken(r)
		if err != nil {
			return nil, err
		}
		if tok == "" {
			break
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}
