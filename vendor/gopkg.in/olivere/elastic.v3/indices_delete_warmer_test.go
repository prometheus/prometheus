// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestDeleteWarmerBuildURL(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tests := []struct {
		Indices  []string
		Names    []string
		Expected string
	}{
		{
			[]string{"test"},
			[]string{"warmer_1"},
			"/test/_warmer/warmer_1",
		},
		{
			[]string{"*"},
			[]string{"warmer_1"},
			"/%2A/_warmer/warmer_1",
		},
		{
			[]string{"_all"},
			[]string{"warmer_1"},
			"/_all/_warmer/warmer_1",
		},
		{
			[]string{"index-1", "index-2"},
			[]string{"warmer_1", "warmer_2"},
			"/index-1%2Cindex-2/_warmer/warmer_1%2Cwarmer_2",
		},
	}

	for _, test := range tests {
		path, _, err := client.DeleteWarmer().Index(test.Indices...).Name(test.Names...).buildURL()
		if err != nil {
			t.Fatal(err)
		}
		if path != test.Expected {
			t.Errorf("expected %q; got: %q", test.Expected, path)
		}
	}
}
