// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestIndicesLifecycle(t *testing.T) {
	client := setupTestClient(t)

	// Create index
	createIndex, err := client.CreateIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !createIndex.Acknowledged {
		t.Errorf("expected IndicesCreateResult.Acknowledged %v; got %v", true, createIndex.Acknowledged)
	}

	// Check if index exists
	indexExists, err := client.IndexExists(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !indexExists {
		t.Fatalf("index %s should exist, but doesn't\n", testIndexName)
	}

	// Delete index
	deleteIndex, err := client.DeleteIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !deleteIndex.Acknowledged {
		t.Errorf("expected DeleteIndexResult.Acknowledged %v; got %v", true, deleteIndex.Acknowledged)
	}

	// Check if index exists
	indexExists, err = client.IndexExists(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if indexExists {
		t.Fatalf("index %s should not exist, but does\n", testIndexName)
	}
}

func TestIndicesCreateValidate(t *testing.T) {
	client := setupTestClient(t)

	// No index name -> fail with error
	res, err := NewIndicesCreateService(client).Body(testMapping).Do()
	if err == nil {
		t.Fatalf("expected IndicesCreate to fail without index name")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}
}
