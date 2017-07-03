// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestGetWarmerBuildURL(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tests := []struct {
		Indices  []string
		Types    []string
		Names    []string
		Expected string
	}{
		{
			[]string{},
			[]string{},
			[]string{},
			"/_warmer",
		},
		{
			[]string{},
			[]string{},
			[]string{"warmer_1"},
			"/_warmer/warmer_1",
		},
		{
			[]string{},
			[]string{"tweet"},
			[]string{},
			"/_all/tweet/_warmer",
		},
		{
			[]string{},
			[]string{"tweet"},
			[]string{"warmer_1"},
			"/_all/tweet/_warmer/warmer_1",
		},
		{
			[]string{"test"},
			[]string{},
			[]string{},
			"/test/_warmer",
		},
		{
			[]string{"test"},
			[]string{},
			[]string{"warmer_1"},
			"/test/_warmer/warmer_1",
		},
		{
			[]string{"*"},
			[]string{},
			[]string{"warmer_1"},
			"/%2A/_warmer/warmer_1",
		},
		{
			[]string{"test"},
			[]string{"tweet"},
			[]string{"warmer_1"},
			"/test/tweet/_warmer/warmer_1",
		},
		{
			[]string{"index-1", "index-2"},
			[]string{"type-1", "type-2"},
			[]string{"warmer_1", "warmer_2"},
			"/index-1%2Cindex-2/type-1%2Ctype-2/_warmer/warmer_1%2Cwarmer_2",
		},
	}

	for _, test := range tests {
		path, _, err := client.GetWarmer().Index(test.Indices...).Type(test.Types...).Name(test.Names...).buildURL()
		if err != nil {
			t.Fatal(err)
		}
		if path != test.Expected {
			t.Errorf("expected %q; got: %q", test.Expected, path)
		}
	}
}
