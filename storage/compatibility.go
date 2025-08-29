package storage

import (
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
)

// AsAppender exposes new combined appender implementation as old appender.
//
// Because old appender interface was not combined, you need extra appender pieces
// to handle old behaviour of disconnected exemplars, metadata and sample cts.
func AsAppender(appender CombinedAppender, exemplar ExemplarAppender, metadata MetadataUpdater, ct CreatedTimestampAppender) Appender {
	return &asAppender{app: appender, exemplar: exemplar, metadata: metadata, ct: ct}
}

type asAppender struct {
	opts CombinedAppendOptions
	app  CombinedAppender

	exemplar ExemplarAppender
	metadata MetadataUpdater
	ct       CreatedTimestampAppender
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

func (a *asAppender) AppendCTZeroSample(ref SeriesRef, l labels.Labels, t int64, ct int64) (SeriesRef, error) {
	return a.ct.AppendCTZeroSample(ref, l, t, ct)
}

func (a *asAppender) AppendHistogramCTZeroSample(ref SeriesRef, l labels.Labels, t int64, ct int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (SeriesRef, error) {
	a.opts.AppendCTAsZero = true
	a.app.SetOptions(&a.opts)
	return a.app.AppendHistogram(ref, l, metadata.Metadata{}, ct, t, h, fh, nil)
}

func (a *asAppender) AppendExemplar(ref SeriesRef, l labels.Labels, e exemplar.Exemplar) (SeriesRef, error) {
	return a.exemplar.AppendExemplar(ref, l, e)
}

func (a *asAppender) UpdateMetadata(ref SeriesRef, l labels.Labels, m metadata.Metadata) (SeriesRef, error) {
	return a.metadata.UpdateMetadata(ref, l, m)
}

// AsCombinedAppender exposes old appender implementation as a new combined appender.
func AsCombinedAppender(appender Appender) CombinedAppender {
	return &asCombinedAppender{app: appender}
}

type asCombinedAppender struct {
	app  Appender
	opts CombinedAppendOptions
}

func (a *asCombinedAppender) AppendSample(ref SeriesRef, ls labels.Labels, meta metadata.Metadata, ct, t int64, v float64, es []exemplar.Exemplar) (_ SeriesRef, err error) {
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

func (a *asCombinedAppender) AppendHistogram(ref SeriesRef, ls labels.Labels, meta metadata.Metadata, ct, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram, es []exemplar.Exemplar) (_ SeriesRef, err error) {
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

func (a *asCombinedAppender) Commit() error { return a.app.Commit() }

func (a *asCombinedAppender) Rollback() error { return a.app.Rollback() }

func (a *asCombinedAppender) SetOptions(opts *CombinedAppendOptions) {
	if opts == nil {
		return
	}
	a.opts = *opts
	a.app.SetOptions(&AppendOptions{DiscardOutOfOrder: opts.DiscardOutOfOrder})
}
