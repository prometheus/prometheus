// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestIndicesOpenValidate(t *testing.T) {
	client := setupTestClient(t)

	// No index name -> fail with error
	res, err := NewIndicesOpenService(client).Do()
	if err == nil {
		t.Fatalf("expected IndicesOpen to fail without index name")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}
}
