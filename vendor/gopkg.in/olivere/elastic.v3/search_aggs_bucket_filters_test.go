// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestFiltersAggregationFilters(t *testing.T) {
	f1 := NewRangeQuery("stock").Gt(0)
	f2 := NewTermQuery("symbol", "GOOG")
	agg := NewFiltersAggregation().Filters(f1, f2)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"filters":{"filters":[{"range":{"stock":{"from":0,"include_lower":false,"include_upper":true,"to":null}}},{"term":{"symbol":"GOOG"}}]}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestFiltersAggregationFilterWithName(t *testing.T) {
	f1 := NewRangeQuery("stock").Gt(0)
	f2 := NewTermQuery("symbol", "GOOG")
	agg := NewFiltersAggregation().
		FilterWithName("f1", f1).
		FilterWithName("f2", f2)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"filters":{"filters":{"f1":{"range":{"stock":{"from":0,"include_lower":false,"include_upper":true,"to":null}}},"f2":{"term":{"symbol":"GOOG"}}}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestFiltersAggregationWithKeyedAndNonKeyedFilters(t *testing.T) {
	agg := NewFiltersAggregation().
		Filter(NewTermQuery("symbol", "MSFT")).               // unnamed
		FilterWithName("one", NewTermQuery("symbol", "GOOG")) // named filter
	_, err := agg.Source()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFiltersAggregationWithSubAggregation(t *testing.T) {
	avgPriceAgg := NewAvgAggregation().Field("price")
	f1 := NewRangeQuery("stock").Gt(0)
	f2 := NewTermQuery("symbol", "GOOG")
	agg := NewFiltersAggregation().Filters(f1, f2).SubAggregation("avg_price", avgPriceAgg)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"aggregations":{"avg_price":{"avg":{"field":"price"}}},"filters":{"filters":[{"range":{"stock":{"from":0,"include_lower":false,"include_upper":true,"to":null}}},{"term":{"symbol":"GOOG"}}]}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestFiltersAggregationWithMetaData(t *testing.T) {
	f1 := NewRangeQuery("stock").Gt(0)
	f2 := NewTermQuery("symbol", "GOOG")
	agg := NewFiltersAggregation().Filters(f1, f2).Meta(map[string]interface{}{"name": "Oliver"})
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"filters":{"filters":[{"range":{"stock":{"from":0,"include_lower":false,"include_upper":true,"to":null}}},{"term":{"symbol":"GOOG"}}]},"meta":{"name":"Oliver"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
