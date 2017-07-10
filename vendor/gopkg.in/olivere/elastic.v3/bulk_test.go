// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestBulk(t *testing.T) {
	//client := setupTestClientAndCreateIndex(t, SetTraceLog(log.New(os.Stdout, "", log.LstdFlags)))
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "sandrae", Message: "Dancing all night long. Yeah."}

	index1Req := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("1").Doc(tweet1)
	index2Req := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("2").Doc(tweet2)
	delete1Req := NewBulkDeleteRequest().Index(testIndexName).Type("tweet").Id("1")

	bulkRequest := client.Bulk()
	bulkRequest = bulkRequest.Add(index1Req)
	bulkRequest = bulkRequest.Add(index2Req)
	bulkRequest = bulkRequest.Add(delete1Req)

	if bulkRequest.NumberOfActions() != 3 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 3, bulkRequest.NumberOfActions())
	}

	bulkResponse, err := bulkRequest.Do()
	if err != nil {
		t.Fatal(err)
	}
	if bulkResponse == nil {
		t.Errorf("expected bulkResponse to be != nil; got nil")
	}

	if bulkRequest.NumberOfActions() != 0 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 0, bulkRequest.NumberOfActions())
	}

	// Document with Id="1" should not exist
	exists, err := client.Exists().Index(testIndexName).Type("tweet").Id("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("expected exists %v; got %v", false, exists)
	}

	// Document with Id="2" should exist
	exists, err = client.Exists().Index(testIndexName).Type("tweet").Id("2").Do()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("expected exists %v; got %v", true, exists)
	}

	// Update
	updateDoc := struct {
		Retweets int `json:"retweets"`
	}{
		42,
	}
	update1Req := NewBulkUpdateRequest().Index(testIndexName).Type("tweet").Id("2").Doc(&updateDoc)
	bulkRequest = client.Bulk()
	bulkRequest = bulkRequest.Add(update1Req)

	if bulkRequest.NumberOfActions() != 1 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 1, bulkRequest.NumberOfActions())
	}

	bulkResponse, err = bulkRequest.Do()
	if err != nil {
		t.Fatal(err)
	}
	if bulkResponse == nil {
		t.Errorf("expected bulkResponse to be != nil; got nil")
	}

	if bulkRequest.NumberOfActions() != 0 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 0, bulkRequest.NumberOfActions())
	}

	// Document with Id="1" should have a retweets count of 42
	doc, err := client.Get().Index(testIndexName).Type("tweet").Id("2").Do()
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Fatal("expected doc to be != nil; got nil")
	}
	if !doc.Found {
		t.Fatalf("expected doc to be found; got found = %v", doc.Found)
	}
	if doc.Source == nil {
		t.Fatal("expected doc source to be != nil; got nil")
	}
	var updatedTweet tweet
	err = json.Unmarshal(*doc.Source, &updatedTweet)
	if err != nil {
		t.Fatal(err)
	}
	if updatedTweet.Retweets != 42 {
		t.Errorf("expected updated tweet retweets = %v; got %v", 42, updatedTweet.Retweets)
	}

	// Update with script
	update2Req := NewBulkUpdateRequest().Index(testIndexName).Type("tweet").Id("2").
		RetryOnConflict(3).
		Script(NewScript("ctx._source.retweets += v").Param("v", 1))
	bulkRequest = client.Bulk()
	bulkRequest = bulkRequest.Add(update2Req)
	if bulkRequest.NumberOfActions() != 1 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 1, bulkRequest.NumberOfActions())
	}
	bulkResponse, err = bulkRequest.Do()
	if err != nil {
		t.Fatal(err)
	}
	if bulkResponse == nil {
		t.Errorf("expected bulkResponse to be != nil; got nil")
	}

	if bulkRequest.NumberOfActions() != 0 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 0, bulkRequest.NumberOfActions())
	}

	// Document with Id="1" should have a retweets count of 43
	doc, err = client.Get().Index(testIndexName).Type("tweet").Id("2").Do()
	if err != nil {
		t.Fatal(err)
	}
	if doc == nil {
		t.Fatal("expected doc to be != nil; got nil")
	}
	if !doc.Found {
		t.Fatalf("expected doc to be found; got found = %v", doc.Found)
	}
	if doc.Source == nil {
		t.Fatal("expected doc source to be != nil; got nil")
	}
	err = json.Unmarshal(*doc.Source, &updatedTweet)
	if err != nil {
		t.Fatal(err)
	}
	if updatedTweet.Retweets != 43 {
		t.Errorf("expected updated tweet retweets = %v; got %v", 43, updatedTweet.Retweets)
	}
}

func TestBulkWithIndexSetOnClient(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "sandrae", Message: "Dancing all night long. Yeah."}

	index1Req := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("1").Doc(tweet1)
	index2Req := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("2").Doc(tweet2)
	delete1Req := NewBulkDeleteRequest().Index(testIndexName).Type("tweet").Id("1")

	bulkRequest := client.Bulk().Index(testIndexName).Type("tweet")
	bulkRequest = bulkRequest.Add(index1Req)
	bulkRequest = bulkRequest.Add(index2Req)
	bulkRequest = bulkRequest.Add(delete1Req)

	if bulkRequest.NumberOfActions() != 3 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 3, bulkRequest.NumberOfActions())
	}

	bulkResponse, err := bulkRequest.Do()
	if err != nil {
		t.Fatal(err)
	}
	if bulkResponse == nil {
		t.Errorf("expected bulkResponse to be != nil; got nil")
	}

	// Document with Id="1" should not exist
	exists, err := client.Exists().Index(testIndexName).Type("tweet").Id("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Errorf("expected exists %v; got %v", false, exists)
	}

	// Document with Id="2" should exist
	exists, err = client.Exists().Index(testIndexName).Type("tweet").Id("2").Do()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("expected exists %v; got %v", true, exists)
	}
}

func TestBulkRequestsSerialization(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "sandrae", Message: "Dancing all night long. Yeah."}

	index1Req := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("1").Doc(tweet1)
	index2Req := NewBulkIndexRequest().OpType("create").Index(testIndexName).Type("tweet").Id("2").Doc(tweet2)
	delete1Req := NewBulkDeleteRequest().Index(testIndexName).Type("tweet").Id("1")
	update2Req := NewBulkUpdateRequest().Index(testIndexName).Type("tweet").Id("2").
		Doc(struct {
			Retweets int `json:"retweets"`
		}{
			Retweets: 42,
		})

	bulkRequest := client.Bulk()
	bulkRequest = bulkRequest.Add(index1Req)
	bulkRequest = bulkRequest.Add(index2Req)
	bulkRequest = bulkRequest.Add(delete1Req)
	bulkRequest = bulkRequest.Add(update2Req)

	if bulkRequest.NumberOfActions() != 4 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 4, bulkRequest.NumberOfActions())
	}

	expected := `{"index":{"_id":"1","_index":"` + testIndexName + `","_type":"tweet"}}
{"user":"olivere","message":"Welcome to Golang and Elasticsearch.","retweets":0,"created":"0001-01-01T00:00:00Z"}
{"create":{"_id":"2","_index":"` + testIndexName + `","_type":"tweet"}}
{"user":"sandrae","message":"Dancing all night long. Yeah.","retweets":0,"created":"0001-01-01T00:00:00Z"}
{"delete":{"_id":"1","_index":"` + testIndexName + `","_type":"tweet"}}
{"update":{"_id":"2","_index":"` + testIndexName + `","_type":"tweet"}}
{"doc":{"retweets":42}}
`
	got, err := bulkRequest.bodyAsString()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got != expected {
		t.Errorf("expected\n%s\ngot:\n%s", expected, got)
	}

	// Run the bulk request
	bulkResponse, err := bulkRequest.Do()
	if err != nil {
		t.Fatal(err)
	}
	if bulkResponse == nil {
		t.Errorf("expected bulkResponse to be != nil; got nil")
	}
	if bulkResponse.Took == 0 {
		t.Errorf("expected took to be > 0; got %d", bulkResponse.Took)
	}
	if bulkResponse.Errors {
		t.Errorf("expected errors to be %v; got %v", false, bulkResponse.Errors)
	}
	if len(bulkResponse.Items) != 4 {
		t.Fatalf("expected 4 result items; got %d", len(bulkResponse.Items))
	}

	// Indexed actions
	indexed := bulkResponse.Indexed()
	if indexed == nil {
		t.Fatal("expected indexed to be != nil; got nil")
	}
	if len(indexed) != 1 {
		t.Fatalf("expected len(indexed) == %d; got %d", 1, len(indexed))
	}
	if indexed[0].Id != "1" {
		t.Errorf("expected indexed[0].Id == %s; got %s", "1", indexed[0].Id)
	}
	if indexed[0].Status != 201 {
		t.Errorf("expected indexed[0].Status == %d; got %d", 201, indexed[0].Status)
	}

	// Created actions
	created := bulkResponse.Created()
	if created == nil {
		t.Fatal("expected created to be != nil; got nil")
	}
	if len(created) != 1 {
		t.Fatalf("expected len(created) == %d; got %d", 1, len(created))
	}
	if created[0].Id != "2" {
		t.Errorf("expected created[0].Id == %s; got %s", "2", created[0].Id)
	}
	if created[0].Status != 201 {
		t.Errorf("expected created[0].Status == %d; got %d", 201, created[0].Status)
	}

	// Deleted actions
	deleted := bulkResponse.Deleted()
	if deleted == nil {
		t.Fatal("expected deleted to be != nil; got nil")
	}
	if len(deleted) != 1 {
		t.Fatalf("expected len(deleted) == %d; got %d", 1, len(deleted))
	}
	if deleted[0].Id != "1" {
		t.Errorf("expected deleted[0].Id == %s; got %s", "1", deleted[0].Id)
	}
	if deleted[0].Status != 200 {
		t.Errorf("expected deleted[0].Status == %d; got %d", 200, deleted[0].Status)
	}
	if !deleted[0].Found {
		t.Errorf("expected deleted[0].Found == %v; got %v", true, deleted[0].Found)
	}

	// Updated actions
	updated := bulkResponse.Updated()
	if updated == nil {
		t.Fatal("expected updated to be != nil; got nil")
	}
	if len(updated) != 1 {
		t.Fatalf("expected len(updated) == %d; got %d", 1, len(updated))
	}
	if updated[0].Id != "2" {
		t.Errorf("expected updated[0].Id == %s; got %s", "2", updated[0].Id)
	}
	if updated[0].Status != 200 {
		t.Errorf("expected updated[0].Status == %d; got %d", 200, updated[0].Status)
	}
	if updated[0].Version != 2 {
		t.Errorf("expected updated[0].Version == %d; got %d", 2, updated[0].Version)
	}

	// Succeeded actions
	succeeded := bulkResponse.Succeeded()
	if succeeded == nil {
		t.Fatal("expected succeeded to be != nil; got nil")
	}
	if len(succeeded) != 4 {
		t.Fatalf("expected len(succeeded) == %d; got %d", 4, len(succeeded))
	}

	// ById
	id1Results := bulkResponse.ById("1")
	if id1Results == nil {
		t.Fatal("expected id1Results to be != nil; got nil")
	}
	if len(id1Results) != 2 {
		t.Fatalf("expected len(id1Results) == %d; got %d", 2, len(id1Results))
	}
	if id1Results[0].Id != "1" {
		t.Errorf("expected id1Results[0].Id == %s; got %s", "1", id1Results[0].Id)
	}
	if id1Results[0].Status != 201 {
		t.Errorf("expected id1Results[0].Status == %d; got %d", 201, id1Results[0].Status)
	}
	if id1Results[0].Version != 1 {
		t.Errorf("expected id1Results[0].Version == %d; got %d", 1, id1Results[0].Version)
	}
	if id1Results[1].Id != "1" {
		t.Errorf("expected id1Results[1].Id == %s; got %s", "1", id1Results[1].Id)
	}
	if id1Results[1].Status != 200 {
		t.Errorf("expected id1Results[1].Status == %d; got %d", 200, id1Results[1].Status)
	}
	if id1Results[1].Version != 2 {
		t.Errorf("expected id1Results[1].Version == %d; got %d", 2, id1Results[1].Version)
	}
}

func TestFailedBulkRequests(t *testing.T) {
	js := `{
  "took" : 2,
  "errors" : true,
  "items" : [ {
    "index" : {
      "_index" : "elastic-test",
      "_type" : "tweet",
      "_id" : "1",
      "_version" : 1,
      "status" : 201
    }
  }, {
    "create" : {
      "_index" : "elastic-test",
      "_type" : "tweet",
      "_id" : "2",
      "_version" : 1,
      "status" : 423,
      "error" : {
      	"type":"routing_missing_exception",
      	"reason":"routing is required for [elastic-test2]/[comment]/[1]"
      }
    }
  }, {
    "delete" : {
      "_index" : "elastic-test",
      "_type" : "tweet",
      "_id" : "1",
      "_version" : 2,
      "status" : 404,
      "found" : false
    }
  }, {
    "update" : {
      "_index" : "elastic-test",
      "_type" : "tweet",
      "_id" : "2",
      "_version" : 2,
      "status" : 200
    }
  } ]
}`

	var resp BulkResponse
	err := json.Unmarshal([]byte(js), &resp)
	if err != nil {
		t.Fatal(err)
	}
	failed := resp.Failed()
	if len(failed) != 2 {
		t.Errorf("expected %d failed items; got: %d", 2, len(failed))
	}
}

func TestBulkEstimatedSizeInBytes(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "sandrae", Message: "Dancing all night long. Yeah."}

	index1Req := NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("1").Doc(tweet1)
	index2Req := NewBulkIndexRequest().OpType("create").Index(testIndexName).Type("tweet").Id("2").Doc(tweet2)
	delete1Req := NewBulkDeleteRequest().Index(testIndexName).Type("tweet").Id("1")
	update2Req := NewBulkUpdateRequest().Index(testIndexName).Type("tweet").Id("2").
		Doc(struct {
			Retweets int `json:"retweets"`
		}{
			Retweets: 42,
		})

	bulkRequest := client.Bulk()
	bulkRequest = bulkRequest.Add(index1Req)
	bulkRequest = bulkRequest.Add(index2Req)
	bulkRequest = bulkRequest.Add(delete1Req)
	bulkRequest = bulkRequest.Add(update2Req)

	if bulkRequest.NumberOfActions() != 4 {
		t.Errorf("expected bulkRequest.NumberOfActions %d; got %d", 4, bulkRequest.NumberOfActions())
	}

	// The estimated size of the bulk request in bytes must be at least
	// the length of the body request.
	raw, err := bulkRequest.bodyAsString()
	if err != nil {
		t.Fatal(err)
	}
	rawlen := int64(len([]byte(raw)))

	if got, want := bulkRequest.EstimatedSizeInBytes(), rawlen; got < want {
		t.Errorf("expected an EstimatedSizeInBytes = %d; got: %v", want, got)
	}

	// Reset should also reset the calculated estimated byte size
	bulkRequest.reset()

	if got, want := bulkRequest.EstimatedSizeInBytes(), int64(0); got != want {
		t.Errorf("expected an EstimatedSizeInBytes = %d; got: %v", want, got)
	}
}

func TestBulkEstimateSizeInBytesLength(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)
	s := client.Bulk()
	r := NewBulkDeleteRequest().Index(testIndexName).Type("tweet").Id("1")
	s = s.Add(r)
	if got, want := s.estimateSizeInBytes(r), int64(1+len(r.String())); got != want {
		t.Fatalf("expected %d; got: %d", want, got)
	}
}

var benchmarkBulkEstimatedSizeInBytes int64

func BenchmarkBulkEstimatedSizeInBytesWith1Request(b *testing.B) {
	client := setupTestClientAndCreateIndex(b)
	s := client.Bulk()
	var result int64
	for n := 0; n < b.N; n++ {
		s = s.Add(NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("1").Doc(struct{ A string }{"1"}))
		s = s.Add(NewBulkUpdateRequest().Index(testIndexName).Type("tweet").Id("1").Doc(struct{ A string }{"2"}))
		s = s.Add(NewBulkDeleteRequest().Index(testIndexName).Type("tweet").Id("1"))
		result = s.EstimatedSizeInBytes()
		s.reset()
	}
	b.ReportAllocs()
	benchmarkBulkEstimatedSizeInBytes = result // ensure the compiler doesn't optimize
}

func BenchmarkBulkEstimatedSizeInBytesWith100Requests(b *testing.B) {
	client := setupTestClientAndCreateIndex(b)
	s := client.Bulk()
	var result int64
	for n := 0; n < b.N; n++ {
		for i := 0; i < 100; i++ {
			s = s.Add(NewBulkIndexRequest().Index(testIndexName).Type("tweet").Id("1").Doc(struct{ A string }{"1"}))
			s = s.Add(NewBulkUpdateRequest().Index(testIndexName).Type("tweet").Id("1").Doc(struct{ A string }{"2"}))
			s = s.Add(NewBulkDeleteRequest().Index(testIndexName).Type("tweet").Id("1"))
		}
		result = s.EstimatedSizeInBytes()
		s.reset()
	}
	b.ReportAllocs()
	benchmarkBulkEstimatedSizeInBytes = result // ensure the compiler doesn't optimize
}
