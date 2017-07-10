// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestIndicesExistsWithoutIndex(t *testing.T) {
	client := setupTestClient(t)

	// No index name -> fail with error
	res, err := NewIndicesExistsService(client).Do()
	if err == nil {
		t.Fatalf("expected IndicesExists to fail without index name")
	}
	if res != false {
		t.Fatalf("expected result to be false; got: %v", res)
	}
}
