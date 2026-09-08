// Copyright The Prometheus Authors
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

//go:build dedupelabels

package record

import (
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// TestNewDecoderSharesSymbolTable verifies that a *labels.SymbolTable passed
// to NewDecoder is actually used as the symbol store for decoded labels.
// Under the FIXME-ignored implementation the table stayed empty, which
// caused symbol fragmentation when multiple decoders ran against the same
// WAL (one private table per decoder). The parallel-WAL-decode path relies
// on this fix to share one table across the decoder pool.
func TestNewDecoderSharesSymbolTable(t *testing.T) {
	syms := labels.NewSymbolTable()
	require.Equal(t, 0, syms.Len(), "fresh symbol table should be empty")

	enc := Encoder{}
	rec := enc.Series([]RefSeries{
		{Ref: chunks.HeadSeriesRef(1), Labels: labels.FromStrings("__name__", "metric_a", "job", "j1")},
		{Ref: chunks.HeadSeriesRef(2), Labels: labels.FromStrings("__name__", "metric_b", "job", "j1")},
	}, nil)

	d1 := NewDecoder(syms, promslog.NewNopLogger())
	out, err := d1.Series(rec, nil)
	require.NoError(t, err)
	require.Len(t, out, 2)

	afterFirst := syms.Len()
	require.Positive(t, afterFirst, "decoder should populate the shared symbol table")

	// A second decoder constructed against the same table must reuse the
	// existing symbols rather than allocate new ones.
	d2 := NewDecoder(syms, promslog.NewNopLogger())
	out2, err := d2.Series(rec, nil)
	require.NoError(t, err)
	require.Len(t, out2, 2)
	require.Equal(t, afterFirst, syms.Len(), "second decoder must reuse symbols, not duplicate them")
}

// TestNewDecoderNilSymbolTable verifies that passing nil still works
// (callers in tests rely on this fallback path).
func TestNewDecoderNilSymbolTable(t *testing.T) {
	enc := Encoder{}
	rec := enc.Series([]RefSeries{
		{Ref: chunks.HeadSeriesRef(1), Labels: labels.FromStrings("__name__", "metric_a")},
	}, nil)

	d := NewDecoder(nil, promslog.NewNopLogger())
	out, err := d.Series(rec, nil)
	require.NoError(t, err)
	require.Len(t, out, 1)
}
