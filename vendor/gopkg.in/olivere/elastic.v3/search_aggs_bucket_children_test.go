// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestChildrenAggregation(t *testing.T) {
	agg := NewChildrenAggregation().Type("answer")
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"children":{"type":"answer"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestChildrenAggregationWithSubAggregation(t *testing.T) {
	subAgg := NewTermsAggregation().Field("owner.display_name").Size(10)
	agg := NewChildrenAggregation().Type("answer")
	agg = agg.SubAggregation("top-names", subAgg)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"aggregations":{"top-names":{"terms":{"field":"owner.display_name","size":10}}},"children":{"type":"answer"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
