// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"testing"
)

func TestIndicesForcemergeBuildURL(t *testing.T) {
	client := setupTestClient(t)

	tests := []struct {
		Indices  []string
		Expected string
	}{
		{
			[]string{},
			"/_forcemerge",
		},
		{
			[]string{"index1"},
			"/index1/_forcemerge",
		},
		{
			[]string{"index1", "index2"},
			"/index1%2Cindex2/_forcemerge",
		},
	}

	for i, test := range tests {
		path, _, err := client.Forcemerge().Index(test.Indices...).buildURL()
		if err != nil {
			t.Errorf("case #%d: %v", i+1, err)
			continue
		}
		if path != test.Expected {
			t.Errorf("case #%d: expected %q; got: %q", i+1, test.Expected, path)
		}
	}
}

func TestIndicesForcemerge(t *testing.T) {
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	_, err := client.Forcemerge(testIndexName).MaxNumSegments(1).WaitForMerge(true).Do()
	if err != nil {
		t.Fatal(err)
	}
	/*
		if !ok {
			t.Fatalf("expected forcemerge to succeed; got: %v", ok)
		}
	*/
}
