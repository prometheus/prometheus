// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

// TODO(oe): Find out why this test fails on Travis CI.
/*
func TestIndicesOpenAndClose(t *testing.T) {
	client := setupTestClient(t)

	// Create index
	createIndex, err := client.CreateIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !createIndex.Acknowledged {
		t.Errorf("expected CreateIndexResult.Acknowledged %v; got %v", true, createIndex.Acknowledged)
	}
	defer func() {
		// Delete index
		deleteIndex, err := client.DeleteIndex(testIndexName).Do()
		if err != nil {
			t.Fatal(err)
		}
		if !deleteIndex.Acknowledged {
			t.Errorf("expected DeleteIndexResult.Acknowledged %v; got %v", true, deleteIndex.Acknowledged)
		}
	}()

	waitForYellow := func() {
		// Wait for status yellow
		res, err := client.ClusterHealth().WaitForStatus("yellow").Timeout("15s").Do()
		if err != nil {
			t.Fatal(err)
		}
		if res != nil && res.TimedOut {
			t.Fatalf("cluster time out waiting for status %q", "yellow")
		}
	}

	// Wait for cluster
	waitForYellow()

	// Close index
	cresp, err := client.CloseIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !cresp.Acknowledged {
		t.Fatalf("expected close index of %q to be acknowledged\n", testIndexName)
	}

	// Wait for cluster
	waitForYellow()

	// Open index again
	oresp, err := client.OpenIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !oresp.Acknowledged {
		t.Fatalf("expected open index of %q to be acknowledged\n", testIndexName)
	}
}
*/

func TestIndicesCloseValidate(t *testing.T) {
	client := setupTestClient(t)

	// No index name -> fail with error
	res, err := NewIndicesCloseService(client).Do()
	if err == nil {
		t.Fatalf("expected IndicesClose to fail without index name")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}
}
