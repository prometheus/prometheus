// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestIndexLifecycle(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}

	// Add a document
	indexResult, err := client.Index().
		Index(testIndexName).
		Type("tweet").
		Id("1").
		BodyJson(&tweet1).
		Do()
	if err != nil {
		t.Fatal(err)
	}
	if indexResult == nil {
		t.Errorf("expected result to be != nil; got: %v", indexResult)
	}

	// Exists
	exists, err := client.Exists().Index(testIndexName).Type("tweet").Id("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("expected exists %v; got %v", true, exists)
	}

	// Get document
	getResult, err := client.Get().
		Index(testIndexName).
		Type("tweet").
		Id("1").
		Do()
	if err != nil {
		t.Fatal(err)
	}
	if getResult.Index != testIndexName {
		t.Errorf("expected GetResult.Index %q; got %q", testIndexName, getResult.Index)
	}
	if getResult.Type != "tweet" {
		t.Errorf("expected GetResult.Type %q; got %q", "tweet", getResult.Type)
	}
	if getResult.Id != "1" {
		t.Errorf("expected GetResult.Id %q; got %q", "1", getResult.Id)
	}
	if getResult.Source == nil {
		t.Errorf("expected GetResult.Source to be != nil; got nil")
	}

	// Decode the Source field
	var tweetGot tweet
	err = json.Unmarshal(*getResult.Source, &tweetGot)
	if err != nil {
		t.Fatal(err)
	}
	if tweetGot.User != tweet1.User {
		t.Errorf("expected Tweet.User to be %q; got %q", tweet1.User, tweetGot.User)
	}
	if tweetGot.Message != tweet1.Message {
		t.Errorf("expected Tweet.Message to be %q; got %q", tweet1.Message, tweetGot.Message)
	}

	// Delete document again
	deleteResult, err := client.Delete().Index(testIndexName).Type("tweet").Id("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if deleteResult == nil {
		t.Errorf("expected result to be != nil; got: %v", deleteResult)
	}

	// Exists
	exists, err = client.Exists().Index(testIndexName).Type("tweet").Id("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("expected exists %v; got %v", false, exists)
	}
}

func TestIndexLifecycleWithAutomaticIDGeneration(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}

	// Add a document
	indexResult, err := client.Index().
		Index(testIndexName).
		Type("tweet").
		BodyJson(&tweet1).
		Do()
	if err != nil {
		t.Fatal(err)
	}
	if indexResult == nil {
		t.Errorf("expected result to be != nil; got: %v", indexResult)
	}
	if indexResult.Id == "" {
		t.Fatalf("expected Es to generate an automatic ID, got: %v", indexResult.Id)
	}
	id := indexResult.Id

	// Exists
	exists, err := client.Exists().Index(testIndexName).Type("tweet").Id(id).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("expected exists %v; got %v", true, exists)
	}

	// Get document
	getResult, err := client.Get().
		Index(testIndexName).
		Type("tweet").
		Id(id).
		Do()
	if err != nil {
		t.Fatal(err)
	}
	if getResult.Index != testIndexName {
		t.Errorf("expected GetResult.Index %q; got %q", testIndexName, getResult.Index)
	}
	if getResult.Type != "tweet" {
		t.Errorf("expected GetResult.Type %q; got %q", "tweet", getResult.Type)
	}
	if getResult.Id != id {
		t.Errorf("expected GetResult.Id %q; got %q", id, getResult.Id)
	}
	if getResult.Source == nil {
		t.Errorf("expected GetResult.Source to be != nil; got nil")
	}

	// Decode the Source field
	var tweetGot tweet
	err = json.Unmarshal(*getResult.Source, &tweetGot)
	if err != nil {
		t.Fatal(err)
	}
	if tweetGot.User != tweet1.User {
		t.Errorf("expected Tweet.User to be %q; got %q", tweet1.User, tweetGot.User)
	}
	if tweetGot.Message != tweet1.Message {
		t.Errorf("expected Tweet.Message to be %q; got %q", tweet1.Message, tweetGot.Message)
	}

	// Delete document again
	deleteResult, err := client.Delete().Index(testIndexName).Type("tweet").Id(id).Do()
	if err != nil {
		t.Fatal(err)
	}
	if deleteResult == nil {
		t.Errorf("expected result to be != nil; got: %v", deleteResult)
	}

	// Exists
	exists, err = client.Exists().Index(testIndexName).Type("tweet").Id(id).Do()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("expected exists %v; got %v", false, exists)
	}
}

func TestIndexValidate(t *testing.T) {
	client := setupTestClient(t)

	tweet := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}

	// No index name -> fail with error
	res, err := NewIndexService(client).Type("tweet").Id("1").BodyJson(&tweet).Do()
	if err == nil {
		t.Fatalf("expected Index to fail without index name")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}

	// No index name -> fail with error
	res, err = NewIndexService(client).Index(testIndexName).Id("1").BodyJson(&tweet).Do()
	if err == nil {
		t.Fatalf("expected Index to fail without type")
	}
	if res != nil {
		t.Fatalf("expected result to be == nil; got: %v", res)
	}
}

func TestIndexCreateExistsOpenCloseDelete(t *testing.T) {
	// TODO: Find out how to make these test robust
	t.Skip("test fails regularly with 409 (Conflict): " +
		"IndexPrimaryShardNotAllocatedException[[elastic-test] " +
		"primary not allocated post api... skipping")

	client := setupTestClient(t)

	// Create index
	createIndex, err := client.CreateIndex(testIndexName).Body(testMapping).Do()
	if err != nil {
		t.Fatal(err)
	}
	if createIndex == nil {
		t.Fatalf("expected response; got: %v", createIndex)
	}
	if !createIndex.Acknowledged {
		t.Errorf("expected ack for creating index; got: %v", createIndex.Acknowledged)
	}

	// Exists
	indexExists, err := client.IndexExists(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if !indexExists {
		t.Fatalf("expected index exists=%v; got %v", true, indexExists)
	}

	// Flush
	_, err = client.Flush().Index(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}

	// Close index
	closeIndex, err := client.CloseIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if closeIndex == nil {
		t.Fatalf("expected response; got: %v", closeIndex)
	}
	if !closeIndex.Acknowledged {
		t.Errorf("expected ack for closing index; got: %v", closeIndex.Acknowledged)
	}

	// Open index
	openIndex, err := client.OpenIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if openIndex == nil {
		t.Fatalf("expected response; got: %v", openIndex)
	}
	if !openIndex.Acknowledged {
		t.Errorf("expected ack for opening index; got: %v", openIndex.Acknowledged)
	}

	// Flush
	_, err = client.Flush().Index(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}

	// Delete index
	deleteIndex, err := client.DeleteIndex(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if deleteIndex == nil {
		t.Fatalf("expected response; got: %v", deleteIndex)
	}
	if !deleteIndex.Acknowledged {
		t.Errorf("expected ack for deleting index; got %v", deleteIndex.Acknowledged)
	}
}
