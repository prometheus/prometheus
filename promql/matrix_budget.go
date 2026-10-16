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

package promql

import (
	"context"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// A matrixIterator guards cancellation and input cost before its buffer grows.
type matrixIterator struct {
	*storage.BufferedSeriesIterator
	guard *queryIterator
}

func (ev *evaluator) newMatrixIterator(ctx context.Context, delta int64) *matrixIterator {
	it := &matrixIterator{BufferedSeriesIterator: storage.NewBuffer(delta)}
	if ev.budget != nil {
		it.guard = ev.newQueryIterator(ctx, nil)
	}
	return it
}

func (it *matrixIterator) Reset(inner chunkenc.Iterator) {
	if it.guard != nil {
		it.guard.beginFrame(-1)
		it.guard.Iterator = inner
		inner = it.guard
	}
	it.BufferedSeriesIterator.Reset(inner)
}

func (it *matrixIterator) beginFrame(ctx context.Context, step int) {
	if it.guard != nil {
		it.guard.ctx = ctx
		it.guard.beginFrame(step)
		it.guard.checkContext()
	}
}

func (it *matrixIterator) checkContext() {
	if it.guard != nil {
		it.guard.checkContext()
	}
}

func (it *matrixIterator) seek(mintFloats, mintHistograms, maxt int64) chunkenc.ValueType {
	if it.guard == nil {
		return it.Seek(maxt)
	}
	g := it.guard
	g.mintFloats, g.mintHistograms, g.maxt = mintFloats, mintHistograms, maxt
	g.project = g.step >= 0
	defer func() { g.project = false }()
	return it.Seek(maxt)
}

// A queryIterator checks cancellation throughout a read. Matrix seeks also
// project new input cost; matrixIterSlice charges successful consumption.
type queryIterator struct {
	chunkenc.Iterator
	ctx                              context.Context
	ev                               *evaluator
	mintFloats, mintHistograms, maxt int64
	step                             int
	pending, readBase                int64
	project                          bool
	floatHistogram                   histogram.FloatHistogram
}

func (ev *evaluator) newQueryIterator(ctx context.Context, inner chunkenc.Iterator) *queryIterator {
	return &queryIterator{Iterator: inner, ctx: ctx, ev: ev, step: -1}
}

func (g *queryIterator) beginFrame(step int) {
	g.step, g.pending, g.project = step, 0, false
	g.readBase = g.ev.budget.samplesRead
}

func (g *queryIterator) flushPending() {
	if g.step < 0 {
		return
	}
	// Buffered points may already have been charged after Seek. Only commit
	// projected input that is still unbilled, including cancellation mid-drain.
	n := addSaturatingInt64(g.readBase, g.pending) - g.ev.budget.samplesRead
	if n > 0 {
		g.ev.consumeMatrixSamples(n, g.step)
	}
}

func (g *queryIterator) checkContext() {
	if err := contextDone(g.ctx, env); err != nil {
		g.flushPending()
		g.ev.error(err)
	}
}

func (g *queryIterator) Next() chunkenc.ValueType {
	g.checkContext()
	vt := g.Iterator.Next()
	g.checkContext()
	return vt
}

func (g *queryIterator) Seek(t int64) chunkenc.ValueType {
	g.checkContext()
	vt := g.Iterator.Seek(t)
	g.checkContext()
	return vt
}

func (g *queryIterator) observe(t, mint, n int64) {
	if t <= mint || t > g.maxt {
		return
	}
	g.pending = addSaturatingInt64(g.pending, n)
	if g.ev.budget.maxSamplesRead > 0 && addSaturatingInt64(g.readBase, g.pending) > g.ev.budget.maxSamplesRead {
		g.flushPending()
	}
}

func (g *queryIterator) At() (int64, float64) {
	t, f := g.Iterator.At()
	if g.project && !value.IsStaleNaN(f) {
		g.observe(t, g.mintFloats, 1)
	}
	return t, f
}

func (g *queryIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	t, h := g.Iterator.AtHistogram(h)
	if g.project && !value.IsStaleNaN(h.Sum) && t > g.mintHistograms && t <= g.maxt {
		// Match HPoint's float histogram weighting without retaining the borrowed
		// histogram or allocating conversion buffers for each point.
		fh := h.ToFloat(&g.floatHistogram)
		g.observe(t, g.mintHistograms, int64((fh.Size()+8)/16))
	}
	return t, h
}

func (g *queryIterator) AtFloatHistogram(h *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	t, h := g.Iterator.AtFloatHistogram(h)
	if g.project && !value.IsStaleNaN(h.Sum) {
		g.observe(t, g.mintHistograms, int64((h.Size()+8)/16))
	}
	return t, h
}
