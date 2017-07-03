// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestSignificantTermsAggregation(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"field":"crime_type"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithArgs(t *testing.T) {
	agg := NewSignificantTermsAggregation().
		Field("crime_type").
		ExecutionHint("map").
		ShardSize(5).
		MinDocCount(10).
		BackgroundFilter(NewTermQuery("city", "London"))
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"background_filter":{"term":{"city":"London"}},"execution_hint":"map","field":"crime_type","min_doc_count":10,"shard_size":5}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationSubAggregation(t *testing.T) {
	crimeTypesAgg := NewSignificantTermsAggregation().Field("crime_type")
	agg := NewTermsAggregation().Field("force")
	agg = agg.SubAggregation("significantCrimeTypes", crimeTypesAgg)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"aggregations":{"significantCrimeTypes":{"significant_terms":{"field":"crime_type"}}},"terms":{"field":"force"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithMetaData(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	agg = agg.Meta(map[string]interface{}{"name": "Oliver"})
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"meta":{"name":"Oliver"},"significant_terms":{"field":"crime_type"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithChiSquare(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	agg = agg.SignificanceHeuristic(
		NewChiSquareSignificanceHeuristic().
			BackgroundIsSuperset(true).
			IncludeNegatives(false),
	)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"chi_square":{"background_is_superset":true,"include_negatives":false},"field":"crime_type"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithGND(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	agg = agg.SignificanceHeuristic(
		NewGNDSignificanceHeuristic(),
	)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"field":"crime_type","gnd":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithJLH(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	agg = agg.SignificanceHeuristic(
		NewJLHScoreSignificanceHeuristic(),
	)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"field":"crime_type","jlh":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithMutualInformation(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	agg = agg.SignificanceHeuristic(
		NewMutualInformationSignificanceHeuristic().
			BackgroundIsSuperset(false).
			IncludeNegatives(true),
	)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"field":"crime_type","mutual_information":{"background_is_superset":false,"include_negatives":true}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithPercentageScore(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	agg = agg.SignificanceHeuristic(
		NewPercentageScoreSignificanceHeuristic(),
	)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"field":"crime_type","percentage":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSignificantTermsAggregationWithScript(t *testing.T) {
	agg := NewSignificantTermsAggregation().Field("crime_type")
	agg = agg.SignificanceHeuristic(
		NewScriptSignificanceHeuristic().
			Script(NewScript("_subset_freq/(_superset_freq - _subset_freq + 1)")),
	)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"significant_terms":{"field":"crime_type","script_heuristic":{"script":"_subset_freq/(_superset_freq - _subset_freq + 1)"}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
