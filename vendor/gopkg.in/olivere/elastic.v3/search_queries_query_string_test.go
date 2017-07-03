// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestQueryStringQuery(t *testing.T) {
	q := NewQueryStringQuery(`this AND that OR thus`)
	q = q.DefaultField("content")
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"query_string":{"default_field":"content","query":"this AND that OR thus"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestQueryStringQueryTimeZone(t *testing.T) {
	q := NewQueryStringQuery(`tweet_date:[2015-01-01 TO 2017-12-31]`)
	q = q.TimeZone("Europe/Berlin")
	src, err := q.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"query_string":{"query":"tweet_date:[2015-01-01 TO 2017-12-31]","time_zone":"Europe/Berlin"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
