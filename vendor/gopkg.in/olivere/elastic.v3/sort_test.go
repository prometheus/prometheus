// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestSortInfo(t *testing.T) {
	builder := SortInfo{Field: "grade", Ascending: false}
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"grade":{"order":"desc"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSortInfoComplex(t *testing.T) {
	builder := SortInfo{
		Field:        "price",
		Ascending:    false,
		Missing:      "_last",
		SortMode:     "avg",
		NestedFilter: NewTermQuery("product.color", "blue"),
		NestedPath:   "variant",
	}
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"price":{"missing":"_last","mode":"avg","nested_filter":{"term":{"product.color":"blue"}},"nested_path":"variant","order":"desc"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestScoreSort(t *testing.T) {
	builder := NewScoreSort()
	if builder.ascending != false {
		t.Error("expected score sorter to be ascending by default")
	}
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_score":{}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestScoreSortOrderAscending(t *testing.T) {
	builder := NewScoreSort().Asc()
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_score":{"reverse":true}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestScoreSortOrderDescending(t *testing.T) {
	builder := NewScoreSort().Desc()
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_score":{}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestFieldSort(t *testing.T) {
	builder := NewFieldSort("grade")
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"grade":{"order":"asc"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestFieldSortOrderDesc(t *testing.T) {
	builder := NewFieldSort("grade").Desc()
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"grade":{"order":"desc"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestFieldSortComplex(t *testing.T) {
	builder := NewFieldSort("price").Desc().
		SortMode("avg").
		Missing("_last").
		UnmappedType("product").
		NestedFilter(NewTermQuery("product.color", "blue")).
		NestedPath("variant")
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"price":{"missing":"_last","mode":"avg","nested_filter":{"term":{"product.color":"blue"}},"nested_path":"variant","order":"desc","unmapped_type":"product"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestGeoDistanceSort(t *testing.T) {
	builder := NewGeoDistanceSort("pin.location").
		Point(-70, 40).
		Order(true).
		Unit("km").
		SortMode("min").
		GeoDistance("sloppy_arc")
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_geo_distance":{"distance_type":"sloppy_arc","mode":"min","pin.location":[{"lat":-70,"lon":40}],"unit":"km"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestGeoDistanceSortOrderDesc(t *testing.T) {
	builder := NewGeoDistanceSort("pin.location").
		Point(-70, 40).
		Unit("km").
		SortMode("min").
		GeoDistance("sloppy_arc").
		Desc()
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_geo_distance":{"distance_type":"sloppy_arc","mode":"min","pin.location":[{"lat":-70,"lon":40}],"reverse":true,"unit":"km"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
func TestScriptSort(t *testing.T) {
	builder := NewScriptSort(NewScript("doc['field_name'].value * factor").Param("factor", 1.1), "number").Order(true)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_script":{"script":{"inline":"doc['field_name'].value * factor","params":{"factor":1.1}},"type":"number"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestScriptSortOrderDesc(t *testing.T) {
	builder := NewScriptSort(NewScript("doc['field_name'].value * factor").Param("factor", 1.1), "number").Desc()
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_script":{"reverse":true,"script":{"inline":"doc['field_name'].value * factor","params":{"factor":1.1}},"type":"number"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
