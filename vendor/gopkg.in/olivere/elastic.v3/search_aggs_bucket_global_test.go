// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestGlobalAggregation(t *testing.T) {
	agg := NewGlobalAggregation()
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"global":{}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestGlobalAggregationWithMetaData(t *testing.T) {
	agg := NewGlobalAggregation().Meta(map[string]interface{}{"name": "Oliver"})
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"global":{},"meta":{"name":"Oliver"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
