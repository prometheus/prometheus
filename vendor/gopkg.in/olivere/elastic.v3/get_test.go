// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestGet(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	_, err := client.Index().Index(testIndexName).Type("tweet").Id("1").BodyJson(&tweet1).Do()
	if err != nil {
		t.Fatal(err)
	}

	// Get document 1
	res, err := client.Get().Index(testIndexName).Type("tweet").Id("1").Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != true {
		t.Errorf("expected Found = true; got %v", res.Found)
	}
	if res.Source == nil {
		t.Errorf("expected Source != nil; got %v", res.Source)
	}

	// Get non existent document 99
	res, err = client.Get().Index(testIndexName).Type("tweet").Id("99").Do()
	if err == nil {
		t.Fatalf("expected error; got: %v", err)
	}
	if !IsNotFound(err) {
		t.Errorf("expected NotFound error; got: %v", err)
	}
	if res != nil {
		t.Errorf("expected no response; got: %v", res)
	}
}

func TestGetWithSourceFiltering(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	_, err := client.Index().Index(testIndexName).Type("tweet").Id("1").BodyJson(&tweet1).Do()
	if err != nil {
		t.Fatal(err)
	}

	// Get document 1, without source
	res, err := client.Get().Index(testIndexName).Type("tweet").Id("1").FetchSource(false).Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != true {
		t.Errorf("expected Found = true; got %v", res.Found)
	}
	if res.Source != nil {
		t.Errorf("expected Source == nil; got %v", res.Source)
	}

	// Get document 1, exclude Message field
	fsc := NewFetchSourceContext(true).Exclude("message")
	res, err = client.Get().Index(testIndexName).Type("tweet").Id("1").FetchSourceContext(fsc).Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != true {
		t.Errorf("expected Found = true; got %v", res.Found)
	}
	if res.Source == nil {
		t.Errorf("expected Source != nil; got %v", res.Source)
	}
	var tw tweet
	err = json.Unmarshal(*res.Source, &tw)
	if err != nil {
		t.Fatal(err)
	}
	if tw.User != "olivere" {
		t.Errorf("expected user %q; got: %q", "olivere", tw.User)
	}
	if tw.Message != "" {
		t.Errorf("expected message %q; got: %q", "", tw.Message)
	}
}

func TestGetWithFields(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	_, err := client.Index().Index(testIndexName).Type("tweet").Id("1").BodyJson(&tweet1).Do()
	if err != nil {
		t.Fatal(err)
	}

	// Get document 1, specifying fields
	res, err := client.Get().Index(testIndexName).Type("tweet").Id("1").Fields("message").Do()
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != true {
		t.Errorf("expected Found = true; got: %v", res.Found)
	}

	// We must NOT have the "user" field
	_, ok := res.Fields["user"]
	if ok {
		t.Fatalf("expected no field %q in document", "user")
	}

	// We must have the "message" field
	messageField, ok := res.Fields["message"]
	if !ok {
		t.Fatalf("expected field %q in document", "message")
	}

	// Depending on the version of elasticsearch the message field will be returned
	// as a string or a slice of strings. This test works in both cases.

	messageString, ok := messageField.(string)
	if !ok {
		messageArray, ok := messageField.([]interface{})
		if !ok {
			t.Fatalf("expected field %q to be a string or a slice of strings; got: %T", "message", messageField)
		} else {
			messageString, ok = messageArray[0].(string)
			if !ok {
				t.Fatalf("expected field %q to be a string or a slice of strings; got: %T", "message", messageField)
			}
		}
	}

	if messageString != tweet1.Message {
		t.Errorf("expected message %q; got: %q", tweet1.Message, messageString)
	}
}

func TestGetValidate(t *testing.T) {
	// Mitigate against http://stackoverflow.com/questions/27491738/elasticsearch-go-index-failures-no-feature-for-name
	client := setupTestClientAndCreateIndex(t)

	if _, err := client.Get().Do(); err == nil {
		t.Fatal("expected Get to fail")
	}
	if _, err := client.Get().Index(testIndexName).Do(); err == nil {
		t.Fatal("expected Get to fail")
	}
	if _, err := client.Get().Type("tweet").Do(); err == nil {
		t.Fatal("expected Get to fail")
	}
	if _, err := client.Get().Id("1").Do(); err == nil {
		t.Fatal("expected Get to fail")
	}
	if _, err := client.Get().Index(testIndexName).Type("tweet").Do(); err == nil {
		t.Fatal("expected Get to fail")
	}
	if _, err := client.Get().Type("tweet").Id("1").Do(); err == nil {
		t.Fatal("expected Get to fail")
	}
}
