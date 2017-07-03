// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import "testing"

func TestDeleteByQuery(t *testing.T) {
	client := setupTestClientAndCreateIndex(t) //, SetTraceLog(log.New(os.Stdout, "", 0)))

	found, err := client.HasPlugin("delete-by-query")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Skip("DeleteByQuery in 2.0 is now a plugin (delete-by-query) and must be " +
			"loaded in the configuration")
	}

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "olivere", Message: "Another unrelated topic."}
	tweet3 := tweet{User: "sandrae", Message: "Cycling is fun."}

	// Add all documents
	_, err = client.Index().Index(testIndexName).Type("tweet").Id("1").BodyJson(&tweet1).Do()
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Index().Index(testIndexName).Type("tweet").Id("2").BodyJson(&tweet2).Do()
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Index().Index(testIndexName).Type("tweet").Id("3").BodyJson(&tweet3).Do()
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Flush().Index(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}

	// Count documents
	count, err := client.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected count = %d; got: %d", 3, count)
	}

	// Delete all documents by sandrae
	q := NewTermQuery("user", "sandrae")
	res, err := client.DeleteByQuery().Index(testIndexName).Type("tweet").Query(q).Do()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatalf("expected response != nil; got: %v", res)
	}

	// Check response
	if got, want := len(res.IndexNames()), 2; got != want {
		t.Fatalf("expected %d indices; got: %d", want, got)
	}
	idx, found := res.Indices["_all"]
	if !found {
		t.Fatalf("expected to find index %q", "_all")
	}
	if got, want := idx.Found, 1; got != want {
		t.Fatalf("expected Found = %v; got: %v", want, got)
	}
	if got, want := idx.Deleted, 1; got != want {
		t.Fatalf("expected Deleted = %v; got: %v", want, got)
	}
	if got, want := idx.Missing, 0; got != want {
		t.Fatalf("expected Missing = %v; got: %v", want, got)
	}
	if got, want := idx.Failed, 0; got != want {
		t.Fatalf("expected Failed = %v; got: %v", want, got)
	}
	idx, found = res.Indices[testIndexName]
	if !found {
		t.Errorf("expected Found = true; got: %v", found)
	}
	if got, want := idx.Found, 1; got != want {
		t.Fatalf("expected Found = %v; got: %v", want, got)
	}
	if got, want := idx.Deleted, 1; got != want {
		t.Fatalf("expected Deleted = %v; got: %v", want, got)
	}
	if got, want := idx.Missing, 0; got != want {
		t.Fatalf("expected Missing = %v; got: %v", want, got)
	}
	if got, want := idx.Failed, 0; got != want {
		t.Fatalf("expected Failed = %v; got: %v", want, got)
	}

	// Flush and check count
	_, err = client.Flush().Index(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	count, err = client.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected Count = %d; got: %d", 2, count)
	}
}
