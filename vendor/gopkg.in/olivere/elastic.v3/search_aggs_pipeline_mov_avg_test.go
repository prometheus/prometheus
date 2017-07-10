// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestMovAvgAggregation(t *testing.T) {
	agg := NewMovAvgAggregation().BucketsPath("the_sum")
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"moving_avg":{"buckets_path":"the_sum"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestMovAvgAggregationWithSimpleModel(t *testing.T) {
	agg := NewMovAvgAggregation().BucketsPath("the_sum").Window(30).Model(NewSimpleMovAvgModel())
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"moving_avg":{"buckets_path":"the_sum","model":"simple","window":30}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestMovAvgAggregationWithLinearModel(t *testing.T) {
	agg := NewMovAvgAggregation().BucketsPath("the_sum").Window(30).Model(NewLinearMovAvgModel())
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"moving_avg":{"buckets_path":"the_sum","model":"linear","window":30}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestMovAvgAggregationWithEWMAModel(t *testing.T) {
	agg := NewMovAvgAggregation().BucketsPath("the_sum").Window(30).Model(NewEWMAMovAvgModel().Alpha(0.5))
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"moving_avg":{"buckets_path":"the_sum","model":"ewma","settings":{"alpha":0.5},"window":30}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestMovAvgAggregationWithHoltLinearModel(t *testing.T) {
	agg := NewMovAvgAggregation().BucketsPath("the_sum").Window(30).
		Model(NewHoltLinearMovAvgModel().Alpha(0.5).Beta(0.4))
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"moving_avg":{"buckets_path":"the_sum","model":"holt","settings":{"alpha":0.5,"beta":0.4},"window":30}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestMovAvgAggregationWithHoltWintersModel(t *testing.T) {
	agg := NewMovAvgAggregation().BucketsPath("the_sum").Window(30).Predict(10).Minimize(true).
		Model(NewHoltWintersMovAvgModel().Alpha(0.5).Beta(0.4).Gamma(0.3).Period(7).Pad(true))
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"moving_avg":{"buckets_path":"the_sum","minimize":true,"model":"holt_winters","predict":10,"settings":{"alpha":0.5,"beta":0.4,"gamma":0.3,"pad":true,"period":7},"window":30}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestMovAvgAggregationWithSubAggs(t *testing.T) {
	agg := NewMovAvgAggregation().BucketsPath("the_sum")
	agg = agg.SubAggregation("avg_sum", NewAvgAggregation().Field("height"))
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"aggregations":{"avg_sum":{"avg":{"field":"height"}}},"moving_avg":{"buckets_path":"the_sum"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
