// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package frankenstein

import (
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/tomwilkie/go-mockaws"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/frankenstein/wire"
	"github.com/prometheus/prometheus/storage/metric"
)

func init() {
	spew.Config.SortKeys = true // :\
}

func c(id string) wire.Chunk {
	return wire.Chunk{ID: id}
}

func TestIntersect(t *testing.T) {
	for _, tc := range []struct {
		in   []wire.ChunksByID
		want wire.ChunksByID
	}{
		{nil, wire.ChunksByID{}},
		{[]wire.ChunksByID{{c("a"), c("b"), c("c")}}, []wire.Chunk{c("a"), c("b"), c("c")}},
		{[]wire.ChunksByID{{c("a"), c("b"), c("c")}, {c("a"), c("c")}}, wire.ChunksByID{c("a"), c("c")}},
		{[]wire.ChunksByID{{c("a"), c("b"), c("c")}, {c("a"), c("c")}, {c("b")}}, wire.ChunksByID{}},
		{[]wire.ChunksByID{{c("a"), c("b"), c("c")}, {c("a"), c("c")}, {c("a")}}, wire.ChunksByID{c("a")}},
	} {
		have := nWayIntersect(tc.in)
		if !reflect.DeepEqual(have, tc.want) {
			t.Errorf("%v != %v", have, tc.want)
		}
	}
}

func TestChunkStore(t *testing.T) {
	store := AWSChunkStore{
		dynamodb:   mockaws.NewMockDynamoDB(),
		s3:         mockaws.NewMockS3(),
		memcache:   nil,
		tableName:  "tablename",
		bucketName: "bucketname",
		cfg: ChunkStoreConfig{
			S3URL:          "",
			DynamoDBURL:    "",
			MemcacheClient: nil,
		},
	}
	store.CreateTables()

	ctx := context.WithValue(context.Background(), UserIDContextKey, "0")
	now := model.Now()

	chunk1 := wire.Chunk{
		ID:      "chunk1",
		From:    now.Add(-time.Hour),
		Through: now,
		Metric: model.Metric{
			model.MetricNameLabel: "foo",
			"bar":  "baz",
			"toms": "code",
		},
		Data: []byte{},
	}
	chunk2 := wire.Chunk{
		ID:      "chunk2",
		From:    now.Add(-time.Hour),
		Through: now,
		Metric: model.Metric{
			model.MetricNameLabel: "foo",
			"bar":  "beep",
			"toms": "code",
		},
		Data: []byte{},
	}

	err := store.Put(ctx, []wire.Chunk{chunk1, chunk2})
	if err != nil {
		t.Fatal(err)
	}

	test := func(name string, expect []wire.Chunk, matchers ...*metric.LabelMatcher) {
		log.Infof(">>> %s", name)
		chunks, err := store.Get(ctx, now.Add(-time.Hour), now, matchers...)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(expect, chunks) {
			t.Fatalf("wrong chunks - " + diff(expect, chunks))
		}
	}

	nameMatcher := mustNewLabelMatcher(metric.Equal, model.MetricNameLabel, "foo")

	test("Just name label", []wire.Chunk{chunk1, chunk2}, nameMatcher)
	test("Equal", []wire.Chunk{chunk1}, nameMatcher, mustNewLabelMatcher(metric.Equal, "bar", "baz"))
	test("Not equal", []wire.Chunk{chunk2}, nameMatcher, mustNewLabelMatcher(metric.NotEqual, "bar", "baz"))
	test("Regex match", []wire.Chunk{chunk1, chunk2}, nameMatcher, mustNewLabelMatcher(metric.RegexMatch, "bar", "beep|baz"))
	test("Multiple matchers", []wire.Chunk{chunk1, chunk2}, nameMatcher, mustNewLabelMatcher(metric.Equal, "toms", "code"), mustNewLabelMatcher(metric.RegexMatch, "bar", "beep|baz"))
	test("Multiple matchers II", []wire.Chunk{chunk1}, nameMatcher, mustNewLabelMatcher(metric.Equal, "toms", "code"), mustNewLabelMatcher(metric.Equal, "bar", "baz"))
}

func mustNewLabelMatcher(matchType metric.MatchType, name model.LabelName, value model.LabelValue) *metric.LabelMatcher {
	matcher, err := metric.NewLabelMatcher(matchType, name, value)
	if err != nil {
		panic(err)
	}
	return matcher
}

func diff(want, have interface{}) string {
	text, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(spew.Sdump(want)),
		B:        difflib.SplitLines(spew.Sdump(have)),
		FromFile: "want",
		ToFile:   "have",
		Context:  3,
	})
	return "\n" + text
}
