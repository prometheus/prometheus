// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"testing"
)

func TestMultiTermVectorsValidateAndBuildURL(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tests := []struct {
		Index                 string
		Type                  string
		Expected              string
		ExpectValidateFailure bool
	}{
		// #0: No index, no type
		{
			"",
			"",
			"/_mtermvectors",
			false,
		},
		// #1: Index only
		{
			"twitter",
			"",
			"/twitter/_mtermvectors",
			false,
		},
		// #2: Type without index
		{
			"",
			"tweet",
			"",
			true,
		},
		// #3: Both index and type
		{
			"twitter",
			"tweet",
			"/twitter/tweet/_mtermvectors",
			false,
		},
	}

	for i, test := range tests {
		builder := client.MultiTermVectors().Index(test.Index).Type(test.Type)
		// Validate
		err := builder.Validate()
		if err != nil {
			if !test.ExpectValidateFailure {
				t.Errorf("#%d: expected no error, got: %v", i, err)
				continue
			}
		} else {
			if test.ExpectValidateFailure {
				t.Errorf("#%d: expected error, got: nil", i)
				continue
			}
			// Build
			path, _, err := builder.buildURL()
			if err != nil {
				t.Errorf("#%d: expected no error, got: %v", i, err)
				continue
			}
			if path != test.Expected {
				t.Errorf("#%d: expected %q; got: %q", i, test.Expected, path)
			}
		}
	}
}

func TestMultiTermVectorsWithIds(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)

	tweet1 := tweet{User: "olivere", Message: "Welcome to Golang and Elasticsearch."}
	tweet2 := tweet{User: "olivere", Message: "Another unrelated topic."}
	tweet3 := tweet{User: "sandrae", Message: "Cycling is fun."}

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

	// MultiTermVectors by specifying ID by 1 and 3
	field := "Message"
	res, err := client.MultiTermVectors().
		Index(testIndexName).
		Type("tweet").
		Add(NewMultiTermvectorItem().Index(testIndexName).Type("tweet").Id("1").Fields(field)).
		Add(NewMultiTermvectorItem().Index(testIndexName).Type("tweet").Id("3").Fields(field)).
		Do()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected to return information and statistics")
	}
	if res.Docs == nil {
		t.Fatal("expected result docs to be != nil; got nil")
	}
	if len(res.Docs) != 2 {
		t.Fatalf("expected to have 2 docs; got %d", len(res.Docs))
	}
}
