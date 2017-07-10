// Copyright 2012-2016 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestSamplerAggregation(t *testing.T) {
	keywordsAgg := NewSignificantTermsAggregation().Field("text")
	agg := NewSamplerAggregation().
		Field("user.id").
		ShardSize(200).
		SubAggregation("keywords", keywordsAgg)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"aggregations":{"keywords":{"significant_terms":{"field":"text"}}},"sampler":{"field":"user.id","shard_size":200}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSamplerAggregationWithMissing(t *testing.T) {
	keywordsAgg := NewSignificantTermsAggregation().Field("text")
	agg := NewSamplerAggregation().
		Field("user.id").
		Missing("n/a").
		SubAggregation("keywords", keywordsAgg)
	src, err := agg.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"aggregations":{"keywords":{"significant_terms":{"field":"text"}}},"sampler":{"field":"user.id","missing":"n/a"}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
