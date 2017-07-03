// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"testing"
)

func TestExists(t *testing.T) {
	client := setupTestClientAndCreateIndexAndAddDocs(t) //, SetTraceLog(log.New(os.Stdout, "", 0)))

	exists, err := client.Exists().Index(testIndexName).Type("comment").Id("1").Parent("tweet").Do()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected document to exist")
	}
}

func TestExistsValidate(t *testing.T) {
	client := setupTestClient(t)

	// No index -> fail with error
	res, err := NewExistsService(client).Type("tweet").Id("1").Do()
	if err == nil {
		t.Fatalf("expected Delete to fail without index name")
	}
	if res != false {
		t.Fatalf("expected result to be false; got: %v", res)
	}

	// No type -> fail with error
	res, err = NewExistsService(client).Index(testIndexName).Id("1").Do()
	if err == nil {
		t.Fatalf("expected Delete to fail without index name")
	}
	if res != false {
		t.Fatalf("expected result to be false; got: %v", res)
	}

	// No id -> fail with error
	res, err = NewExistsService(client).Index(testIndexName).Type("tweet").Do()
	if err == nil {
		t.Fatalf("expected Delete to fail without index name")
	}
	if res != false {
		t.Fatalf("expected result to be false; got: %v", res)
	}
}
