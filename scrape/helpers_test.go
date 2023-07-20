// Copyright 2013 The Prometheus Authors
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

package scrape

import (
	"context"
	"fmt"
	"math/rand"
	"strings"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

type nopAppendable struct{}

func (a nopAppendable) Appender(_ context.Context) storage.Appender {
	return nopAppender{}
}

type nopAppender struct{}

func (a nopAppender) Append(storage.SeriesRef, labels.Labels, int64, float64) (storage.SeriesRef, error) {
	return 0, nil
}

func (a nopAppender) AppendExemplar(storage.SeriesRef, labels.Labels, exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, nil
}

func (a nopAppender) AppendHistogram(storage.SeriesRef, labels.Labels, int64, *histogram.Histogram, *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, nil
}

func (a nopAppender) UpdateMetadata(storage.SeriesRef, labels.Labels, metadata.Metadata) (storage.SeriesRef, error) {
	return 0, nil
}

func (a nopAppender) Commit() error   { return nil }
func (a nopAppender) Rollback() error { return nil }

type floatSample struct {
	metric labels.Labels
	t      int64
	f      float64
}

type histogramSample struct {
	t  int64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

// collectResultAppender records all samples that were added through the appender.
// It can be used as its zero value or be backed by another appender it writes samples through.
type collectResultAppender struct {
	next                 storage.Appender
	resultFloats         []floatSample
	pendingFloats        []floatSample
	rolledbackFloats     []floatSample
	resultHistograms     []histogramSample
	pendingHistograms    []histogramSample
	rolledbackHistograms []histogramSample
	resultExemplars      []exemplar.Exemplar
	pendingExemplars     []exemplar.Exemplar
	resultMetadata       []metadata.Metadata
	pendingMetadata      []metadata.Metadata
}

func (a *collectResultAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	a.pendingFloats = append(a.pendingFloats, floatSample{
		metric: lset,
		t:      t,
		f:      v,
	})

	if ref == 0 {
		ref = storage.SeriesRef(rand.Uint64())
	}
	if a.next == nil {
		return ref, nil
	}

	ref, err := a.next.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, err
}

func (a *collectResultAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	a.pendingExemplars = append(a.pendingExemplars, e)
	if a.next == nil {
		return 0, nil
	}

	return a.next.AppendExemplar(ref, l, e)
}

func (a *collectResultAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	a.pendingHistograms = append(a.pendingHistograms, histogramSample{h: h, fh: fh, t: t})
	if a.next == nil {
		return 0, nil
	}

	return a.next.AppendHistogram(ref, l, t, h, fh)
}

func (a *collectResultAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	a.pendingMetadata = append(a.pendingMetadata, m)
	if ref == 0 {
		ref = storage.SeriesRef(rand.Uint64())
	}
	if a.next == nil {
		return ref, nil
	}

	return a.next.UpdateMetadata(ref, l, m)
}

func (a *collectResultAppender) Commit() error {
	a.resultFloats = append(a.resultFloats, a.pendingFloats...)
	a.resultExemplars = append(a.resultExemplars, a.pendingExemplars...)
	a.resultHistograms = append(a.resultHistograms, a.pendingHistograms...)
	a.resultMetadata = append(a.resultMetadata, a.pendingMetadata...)
	a.pendingFloats = nil
	a.pendingExemplars = nil
	a.pendingHistograms = nil
	a.pendingMetadata = nil
	if a.next == nil {
		return nil
	}
	return a.next.Commit()
}

func (a *collectResultAppender) Rollback() error {
	a.rolledbackFloats = a.pendingFloats
	a.rolledbackHistograms = a.pendingHistograms
	a.pendingFloats = nil
	a.pendingHistograms = nil
	if a.next == nil {
		return nil
	}
	return a.next.Rollback()
}

func (a *collectResultAppender) String() string {
	var sb strings.Builder
	for _, s := range a.resultFloats {
		sb.WriteString(fmt.Sprintf("committed: %s %f %d\n", s.metric, s.f, s.t))
	}
	for _, s := range a.pendingFloats {
		sb.WriteString(fmt.Sprintf("pending: %s %f %d\n", s.metric, s.f, s.t))
	}
	for _, s := range a.rolledbackFloats {
		sb.WriteString(fmt.Sprintf("rolledback: %s %f %d\n", s.metric, s.f, s.t))
	}
	return sb.String()
}
