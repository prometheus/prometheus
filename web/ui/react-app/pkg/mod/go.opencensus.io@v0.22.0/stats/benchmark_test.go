// Copyright 2017, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package stats_test

import (
	"context"
	"testing"

	"go.opencensus.io/stats"
	_ "go.opencensus.io/stats/view" // enable collection
	"go.opencensus.io/tag"
)

var m = makeMeasure()

func BenchmarkRecord0(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats.Record(ctx)
	}
}

func BenchmarkRecord1(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats.Record(ctx, m.M(1))
	}
}

func BenchmarkRecord8(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats.Record(ctx, m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1))
	}
}

func BenchmarkRecord8_Parallel(b *testing.B) {
	ctx := context.Background()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			stats.Record(ctx, m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1))
		}
	})
}

func BenchmarkRecord8_8Tags(b *testing.B) {
	ctx := context.Background()
	key1, _ := tag.NewKey("key1")
	key2, _ := tag.NewKey("key2")
	key3, _ := tag.NewKey("key3")
	key4, _ := tag.NewKey("key4")
	key5, _ := tag.NewKey("key5")
	key6, _ := tag.NewKey("key6")
	key7, _ := tag.NewKey("key7")
	key8, _ := tag.NewKey("key8")

	tag.New(ctx,
		tag.Insert(key1, "value"),
		tag.Insert(key2, "value"),
		tag.Insert(key3, "value"),
		tag.Insert(key4, "value"),
		tag.Insert(key5, "value"),
		tag.Insert(key6, "value"),
		tag.Insert(key7, "value"),
		tag.Insert(key8, "value"),
	)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stats.Record(ctx, m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1), m.M(1))
	}
}

func makeMeasure() *stats.Int64Measure {
	return stats.Int64("m", "test measure", "")
}
