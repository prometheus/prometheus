// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"testing"
)

func TestDelete(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "olivere", Message: "Another unrelated topic."}
	tweet3 := tweet{User: "sandrae", Message: "Cycling is fun."}

	// Add all documents
	_, err := client.Index().Index(testIndexName).Type("tweet").Id("1").BodyJson(&tweet1).Do()
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
		t.Errorf("expected Count = %d; got %d", 3, count)
	}

	// Delete document 1
	res, err := client.Delete().Index(testIndexName).Type("tweet").Id("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != true {
		t.Errorf("expected Found = true; got %v", res.Found)
	}
	_, err = client.Flush().Index(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	count, err = client.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected Count = %d; got %d", 2, count)
	}

	// Delete non existent document 99
	res, err = client.Delete().Index(testIndexName).Type("tweet").Id("99").Refresh(true).Do()
	if err == nil {
		t.Fatalf("expected error; got: %v", err)
	}
	if !IsNotFound(err) {
		t.Errorf("expected NotFound error; got %v", err)
	}
	if res != nil {
		t.Fatalf("expected no response; got: %v", res)
	}

	count, err = client.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected Count = %d; got %d", 2, count)
	}
}

func TestDeleteValidate(t *testing.T) {
	client := setupTestClientAndCreateIndexAndAddDocs(t)

	// No index name -> fail with error
	res, err := NewDeleteService(client).Type("tweet").Id("1").Do()
	if err == nil {
		t.Fatalf("expected Delete to fail without index name")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}

	// No type -> fail with error
	res, err = NewDeleteService(client).Index(testIndexName).Id("1").Do()
	if err == nil {
		t.Fatalf("expected Delete to fail without type")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}

	// No id -> fail with error
	res, err = NewDeleteService(client).Index(testIndexName).Type("tweet").Do()
	if err == nil {
		t.Fatalf("expected Delete to fail without id")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}
}
