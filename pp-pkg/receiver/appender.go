package receiver

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

type timeSeriesData struct {
	timeSeries []model.TimeSeries
}

func (d *timeSeriesData) TimeSeries() []model.TimeSeries {
	return d.timeSeries
}

func (d *timeSeriesData) Destroy() {
	d.timeSeries = nil
}

type promAppender struct {
	ctx         context.Context
	receiver    *Receiver
	relabelerID string
	data        *timeSeriesData
}

func newPromAppender(ctx context.Context, receiver *Receiver, relabelerID string) *promAppender {
	return &promAppender{
		ctx:         ctx,
		receiver:    receiver,
		relabelerID: relabelerID,
		data:        &timeSeriesData{},
	}
}

func (a *promAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	lsb := model.NewLabelSetBuilder()
	l.Range(func(label labels.Label) {
		lsb.Add(label.Name, label.Value)
	})

	a.data.timeSeries = append(a.data.timeSeries, model.TimeSeries{
		LabelSet:  lsb.Build(),
		Timestamp: uint64(t),
		Value:     v,
	})
	return 0, nil
}

func (a *promAppender) Commit() error {
	if len(a.data.timeSeries) == 0 {
		return nil
	}

	_, err := a.receiver.AppendTimeSeries(a.ctx, a.data, nil, a.relabelerID)
	return err
}

func (a *promAppender) Rollback() error {
	return nil
}

func (a *promAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, nil
}

func (a *promAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, nil
}

func (a *promAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	return 0, nil
}

func (a *promAppender) AppendCTZeroSample(ref storage.SeriesRef, l labels.Labels, t, ct int64) (storage.SeriesRef, error) {
	return 0, nil
}
