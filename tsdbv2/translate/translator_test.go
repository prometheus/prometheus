// Copyright 2024 The Prometheus Authors
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

package translate

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdbv2/data"
)

func TestTranslte_NextSeq(t *testing.T) {
	db := data.Test(t)
	tr := New(db)
	id, err := tr.NextSeq(1, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), id, " all sequence start from 1")

	id2, err := tr.NextSeq(2, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), id2, " all sequence start from 1")

	t.Run("increments per tenant field", func(t *testing.T) {
		next, err := tr.NextSeq(1, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(2), next)

		next2, err := tr.NextSeq(2, 1)
		require.NoError(t, err)
		require.Equal(t, uint64(2), next2)
	})
	t.Run("handle multiple fields per tenant", func(t *testing.T) {
		next, err := tr.NextSeq(1, 2)
		require.NoError(t, err)
		require.Equal(t, uint64(1), next)

		next2, err := tr.NextSeq(2, 2)
		require.NoError(t, err)
		require.Equal(t, uint64(1), next2)
	})
}

func BenchmarkTrenaslate_NextSeq(b *testing.B) {
	db := data.Test(b)
	tr := New(db)
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			for n := range uint64(10) {
				_, err := tr.NextSeq(n, n)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

func TestTranslate_TranslateText(t *testing.T) {
	db := data.Test(t)
	tr := New(db)
	type T struct {
		data   string
		tenant uint64
		field  uint64
		want   uint64
	}
	sample := []T{
		{"aa", 0, 0, 1},
		{"bb", 0, 0, 2},
		{"cc", 0, 2, 1},
		{"dd", 0, 2, 2},
	}
	for _, x := range sample {
		got, err := tr.TranslateText(x.tenant, x.field, []byte(x.data))
		require.NoError(t, err)
		require.Equal(t, x.want, got)
	}

	// runting multiple times should yield the same results
	for _, x := range sample {
		got, err := tr.TranslateText(x.tenant, x.field, []byte(x.data))
		require.NoError(t, err)
		require.Equal(t, x.want, got)
	}
}

func BenchmarkTranslate_TranslateText(b *testing.B) {
	db := data.Test(b)
	tr := New(db)

	data := []byte("hello,world")
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			n, err := tr.TranslateText(0, 0, data)
			if err != nil {
				b.Fatal(err)
			}
			if n != 1 {
				b.Fatalf("expected 1 got %d", n)
			}
		}
	})
}
