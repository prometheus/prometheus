// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestIndicesQuery(t *testing.T) {
	q := NewIndicesQuery(NewTermQuery("tag", "wow"), "index1", "index2")
	q = q.NoMatchQuery(NewTermQuery("tag", "kow"))
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"indices":{"indices":["index1","index2"],"no_match_query":{"term":{"tag":"kow"}},"query":{"term":{"tag":"wow"}}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestIndicesQueryWithNoMatchQueryType(t *testing.T) {
	q := NewIndicesQuery(NewTermQuery("tag", "wow"), "index1", "index2")
	q = q.NoMatchQueryType("all")
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"indices":{"indices":["index1","index2"],"no_match_query":"all","query":{"term":{"tag":"wow"}}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
