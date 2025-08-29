// Copyright 2025 The Prometheus Authors
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

package storage

import (
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
)

// AsAppender exposes new V2 appender implementation as old appender.
//
// Because old appender interface allow exemplar, metadata and CT updates to
// be disjointed from sample and histogram updates, you need extra appender pieces
// to handle the old behaviour.
func AsAppender(appender AppenderV2, exemplar ExemplarAppender, metadata MetadataUpdater, ct combinedCTAppender) Appender {
	return &asAppender{app: appender, exemplar: exemplar, metadata: metadata, ct: ct}
}

// For historical reason this interface is not consistent
// (HistogramAppender has CTZero but Appender does not have for Sample).
// No need to fix this elsewhere, given we plan to deprecate old interface and those methods.
type combinedCTAppender interface {
	CreatedTimestampAppender
	AppendHistogramCTZeroSample(ref SeriesRef, l labels.Labels, t, ct int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error)
}

type asAppender struct {
	opts AppendV2Options
	app  AppenderV2

	exemplar ExemplarAppender
	metadata MetadataUpdater
	ct       combinedCTAppender
}

func (a *asAppender) Append(ref SeriesRef, l labels.Labels, t int64, v float64) (SeriesRef, error) {
	return a.app.AppendSample(ref, l, metadata.Metadata{}, 0, t, v, nil)
}

func (a *asAppender) AppendHistogram(ref SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error) {
	return a.app.AppendHistogram(ref, l, metadata.Metadata{}, 0, t, h, fh, nil)
}

func (a *asAppender) Commit() error {
	return a.app.Commit()
}

func (a *asAppender) Rollback() error {
	return a.app.Rollback()
}

func (a *asAppender) SetOptions(opts *AppendOptions) {
	if opts == nil {
		return
	}

	a.opts.DiscardOutOfOrder = opts.DiscardOutOfOrder
	a.app.SetOptions(&a.opts)
}

func (a *asAppender) AppendCTZeroSample(ref SeriesRef, l labels.Labels, t, ct int64) (SeriesRef, error) {
	// We could consider using AppendSample with AppendCTAsZero true, but this option is stateful,
	// so use compatibility ct interface.
	return a.ct.AppendCTZeroSample(ref, l, t, ct)
}

func (a *asAppender) AppendHistogramCTZeroSample(ref SeriesRef, l labels.Labels, t, ct int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error) {
	// We could consider using AppendHistogram with AppendCTAsZero true, but this option is stateful,
	// so use compatibility ct interface.
	return a.ct.AppendHistogramCTZeroSample(ref, l, t, ct, h, fh)
}

func (a *asAppender) AppendExemplar(ref SeriesRef, l labels.Labels, e exemplar.Exemplar) (SeriesRef, error) {
	return a.exemplar.AppendExemplar(ref, l, e)
}

func (a *asAppender) UpdateMetadata(ref SeriesRef, l labels.Labels, m metadata.Metadata) (SeriesRef, error) {
	return a.metadata.UpdateMetadata(ref, l, m)
}

// AsAppenderV2 exposes old appender implementation as a new v2 appender.
func AsAppenderV2(appender Appender) AppenderV2 {
	return &asAppenderV2{app: appender}
}

type asAppenderV2 struct {
	app  Appender
	opts AppendV2Options
}

func (a *asAppenderV2) AppendSample(ref SeriesRef, ls labels.Labels, meta metadata.Metadata, ct, t int64, v float64, es []exemplar.Exemplar) (_ SeriesRef, err error) {
	if ct != 0 && a.opts.AppendCTAsZero {
		ref, err = a.app.AppendCTZeroSample(ref, ls, t, ct)
		if err != nil {
			return ref, err
		}
	}
	ref, err = a.app.Append(ref, ls, t, v)
	if err != nil {
		return ref, err
	}

	// Check feature flag?
	for _, e := range es {
		ref, err = a.app.AppendExemplar(ref, ls, e)
		if err != nil {
			return ref, err
		}
	}
	// Check feature flag?
	if !meta.IsEmpty() {
		ref, err = a.app.UpdateMetadata(ref, ls, meta)
		if err != nil {
			return ref, err
		}
	}
	return ref, err
}

func (a *asAppenderV2) AppendHistogram(ref SeriesRef, ls labels.Labels, meta metadata.Metadata, ct, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram, es []exemplar.Exemplar) (_ SeriesRef, err error) {
	if ct != 0 && a.opts.AppendCTAsZero {
		ref, err = a.app.AppendHistogramCTZeroSample(ref, ls, t, ct, h, fh)
		if err != nil {
			return ref, err
		}
	}
	ref, err = a.app.AppendHistogram(ref, ls, t, h, fh)
	if err != nil {
		return ref, err
	}
	// Check feature flag?
	for _, e := range es {
		ref, err = a.app.AppendExemplar(ref, ls, e)
		if err != nil {
			return ref, err
		}
	}
	// Check feature flag?
	if !meta.IsEmpty() {
		ref, err = a.app.UpdateMetadata(ref, ls, meta)
		if err != nil {
			return ref, err
		}
	}
	return ref, err
}

func (a *asAppenderV2) Commit() error { return a.app.Commit() }

func (a *asAppenderV2) Rollback() error { return a.app.Rollback() }

func (a *asAppenderV2) SetOptions(opts *AppendV2Options) {
	if opts == nil {
		return
	}
	a.opts = *opts
	a.app.SetOptions(&AppendOptions{DiscardOutOfOrder: opts.DiscardOutOfOrder})
}
