// Copyright 2012-present Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"encoding/json"
	"testing"
)

func TestReindexSourceWithSourceIndexAndDestinationIndex(t *testing.T) {
	client := setupTestClient(t)
	out, err := client.ReindexTask().SourceIndex("twitter").DestinationIndex("new_twitter").body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"new_twitter"},"source":{"index":"twitter"}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithSourceAndDestinationAndVersionType(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter")
	dst := NewReindexDestination().Index("new_twitter").VersionType("external")
	out, err := client.ReindexTask().Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"new_twitter","version_type":"external"},"source":{"index":"twitter"}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithSourceAndDestinationAndOpType(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter")
	dst := NewReindexDestination().Index("new_twitter").OpType("create")
	out, err := client.ReindexTask().Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"new_twitter","op_type":"create"},"source":{"index":"twitter"}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithConflictsProceed(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter")
	dst := NewReindexDestination().Index("new_twitter").OpType("create")
	out, err := client.ReindexTask().Conflicts("proceed").Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"conflicts":"proceed","dest":{"index":"new_twitter","op_type":"create"},"source":{"index":"twitter"}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithProceedOnVersionConflict(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter")
	dst := NewReindexDestination().Index("new_twitter").OpType("create")
	out, err := client.ReindexTask().ProceedOnVersionConflict().Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"conflicts":"proceed","dest":{"index":"new_twitter","op_type":"create"},"source":{"index":"twitter"}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithQuery(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter").Type("tweet").Query(NewTermQuery("user", "olivere"))
	dst := NewReindexDestination().Index("new_twitter")
	out, err := client.ReindexTask().Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"new_twitter"},"source":{"index":"twitter","query":{"term":{"user":"olivere"}},"type":"tweet"}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithMultipleSourceIndicesAndTypes(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter", "blog").Type("tweet", "post")
	dst := NewReindexDestination().Index("all_together")
	out, err := client.ReindexTask().Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"all_together"},"source":{"index":["twitter","blog"],"type":["tweet","post"]}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithSourceAndSize(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter").Sort("date", false)
	dst := NewReindexDestination().Index("new_twitter")
	out, err := client.ReindexTask().Size(10000).Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"new_twitter"},"size":10000,"source":{"index":"twitter","sort":[{"date":{"order":"desc"}}]}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithScript(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("twitter")
	dst := NewReindexDestination().Index("new_twitter").VersionType("external")
	scr := NewScriptInline("if (ctx._source.foo == 'bar') {ctx._version++; ctx._source.remove('foo')}")
	out, err := client.ReindexTask().Source(src).Destination(dst).Script(scr).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"new_twitter","version_type":"external"},"script":{"inline":"if (ctx._source.foo == 'bar') {ctx._version++; ctx._source.remove('foo')}"},"source":{"index":"twitter"}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindexSourceWithRouting(t *testing.T) {
	client := setupTestClient(t)
	src := NewReindexSource().Index("source").Query(NewMatchQuery("company", "cat"))
	dst := NewReindexDestination().Index("dest").Routing("=cat")
	out, err := client.ReindexTask().Source(src).Destination(dst).body()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := `{"dest":{"index":"dest","routing":"=cat"},"source":{"index":"source","query":{"match":{"company":{"query":"cat"}}}}}`
	if got != want {
		t.Fatalf("\ngot  %s\nwant %s", got, want)
	}
}

func TestReindex(t *testing.T) {
	client := setupTestClientAndCreateIndexAndAddDocs(t) // , SetTraceLog(log.New(os.Stdout, "", 0)))
	esversion, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}
	if esversion < "2.3.0" {
		t.Skipf("Elasticsearch %v does not support Reindex API yet", esversion)
	}

	sourceCount, err := client.Count(testIndexName).Do()
	if err != nil {
		t.Fatal(err)
	}
	if sourceCount <= 0 {
		t.Fatalf("expected more than %d documents; got: %d", 0, sourceCount)
	}

	targetCount, err := client.Count(testIndexName2).Do()
	if err != nil {
		t.Fatal(err)
	}
	if targetCount != 0 {
		t.Fatalf("expected %d documents; got: %d", 0, targetCount)
	}

	// Simple copying
	src := NewReindexSource().Index(testIndexName)
	dst := NewReindexDestination().Index(testIndexName2)
	res, err := client.ReindexTask().Source(src).Destination(dst).Refresh(true).Do()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected result != nil")
	}
	if res.Total != sourceCount {
		t.Errorf("expected %d, got %d", sourceCount, res.Total)
	}
	if res.Updated != 0 {
		t.Errorf("expected %d, got %d", 0, res.Updated)
	}
	if res.Created != sourceCount {
		t.Errorf("expected %d, got %d", sourceCount, res.Created)
	}

	targetCount, err = client.Count(testIndexName2).Do()
	if err != nil {
		t.Fatal(err)
	}
	if targetCount != sourceCount {
		t.Fatalf("expected %d documents; got: %d", sourceCount, targetCount)
	}
}
