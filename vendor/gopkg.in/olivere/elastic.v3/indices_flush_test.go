// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"testing"
)

func TestFlush(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	// Flush all indices
	res, err := client.Flush().Do()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Errorf("expected res to be != nil; got: %v", res)
	}
}

func TestFlushBuildURL(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tests := []struct {
		Indices               []string
		Expected              string
		ExpectValidateFailure bool
	}{
		{
			[]string{},
			"/_flush",
			false,
		},
		{
			[]string{"index1"},
			"/index1/_flush",
			false,
		},
		{
			[]string{"index1", "index2"},
			"/index1%2Cindex2/_flush",
			false,
		},
	}

	for i, test := range tests {
		err := NewIndicesFlushService(client).Index(test.Indices...).Validate()
		if err == nil && test.ExpectValidateFailure {
			t.Errorf("case #%d: expected validate to fail", i+1)
			continue
		}
		if err != nil && !test.ExpectValidateFailure {
			t.Errorf("case #%d: expected validate to succeed", i+1)
			continue
		}
		if !test.ExpectValidateFailure {
			path, _, err := NewIndicesFlushService(client).Index(test.Indices...).buildURL()
			if err != nil {
				t.Fatalf("case #%d: %v", i+1, err)
			}
			if path != test.Expected {
				t.Errorf("case #%d: expected %q; got: %q", i+1, test.Expected, path)
			}
		}
	}
}
