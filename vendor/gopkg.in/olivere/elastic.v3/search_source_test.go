// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestSearchSourceMatchAllQuery(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceNoFields(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ).NoFields()
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"fields":[],"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceFields(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ).Fields("message", "tags")
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"fields":["message","tags"],"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceFetchSourceDisabled(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ).FetchSource(false)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_source":false,"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceFetchSourceByWildcards(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	fsc := NewFetchSourceContext(true).Include("obj1.*", "obj2.*").Exclude("*.description")
	builder := NewSearchSource().Query(matchAllQ).FetchSourceContext(fsc)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"_source":{"excludes":["*.description"],"includes":["obj1.*","obj2.*"]},"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceFieldDataFields(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ).FieldDataFields("test1", "test2")
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"fielddata_fields":["test1","test2"],"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceScriptFields(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	sf1 := NewScriptField("test1", NewScript("doc['my_field_name'].value * 2"))
	sf2 := NewScriptField("test2", NewScript("doc['my_field_name'].value * factor").Param("factor", 3.1415927))
	builder := NewSearchSource().Query(matchAllQ).ScriptFields(sf1, sf2)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"query":{"match_all":{}},"script_fields":{"test1":{"script":"doc['my_field_name'].value * 2"},"test2":{"script":{"inline":"doc['my_field_name'].value * factor","params":{"factor":3.1415927}}}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourcePostFilter(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	pf := NewTermQuery("tag", "important")
	builder := NewSearchSource().Query(matchAllQ).PostFilter(pf)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"post_filter":{"term":{"tag":"important"}},"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceHighlight(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	hl := NewHighlight().Field("content")
	builder := NewSearchSource().Query(matchAllQ).Highlight(hl)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"highlight":{"fields":{"content":{}}},"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceRescoring(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	rescorerQuery := NewMatchQuery("field1", "the quick brown fox").Type("phrase").Slop(2)
	rescorer := NewQueryRescorer(rescorerQuery)
	rescorer = rescorer.QueryWeight(0.7)
	rescorer = rescorer.RescoreQueryWeight(1.2)
	rescore := NewRescore().WindowSize(50).Rescorer(rescorer)
	builder := NewSearchSource().Query(matchAllQ).Rescorer(rescore)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"query":{"match_all":{}},"rescore":{"query":{"query_weight":0.7,"rescore_query":{"match":{"field1":{"query":"the quick brown fox","slop":2,"type":"phrase"}}},"rescore_query_weight":1.2},"window_size":50}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceIndexBoost(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ).IndexBoost("index1", 1.4).IndexBoost("index2", 1.3)
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"indices_boost":{"index1":1.4,"index2":1.3},"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceMixDifferentSorters(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ).
		Sort("a", false).
		SortWithInfo(SortInfo{Field: "b", Ascending: true}).
		SortBy(NewScriptSort(NewScript("doc['field_name'].value * factor").Param("factor", 1.1), "number"))
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"query":{"match_all":{}},"sort":[{"a":{"order":"desc"}},{"b":{"order":"asc"}},{"_script":{"script":{"inline":"doc['field_name'].value * factor","params":{"factor":1.1}},"type":"number"}}]}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}

func TestSearchSourceInnerHits(t *testing.T) {
	matchAllQ := NewMatchAllQuery()
	builder := NewSearchSource().Query(matchAllQ).
		InnerHit("comments", NewInnerHit().Type("comment").Query(NewMatchQuery("user", "olivere"))).
		InnerHit("views", NewInnerHit().Path("view"))
	src, err := builder.Source()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshaling to JSON failed: %v", err)
	}
	got := string(data)
	expected := `{"inner_hits":{"comments":{"type":{"comment":{"query":{"match":{"user":{"query":"olivere"}}}}}},"views":{"path":{"view":{}}}},"query":{"match_all":{}}}`
	if got != expected {
		t.Errorf("expected\n%s\n,got:\n%s", expected, got)
	}
}
