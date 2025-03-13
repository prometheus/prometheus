package processor

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp/go/util"
)

type RefillProcessor struct {
	decoderBuilder DecoderBuilder
	receiver       Receiver

	criticalErrorCount      *prometheus.CounterVec
	decodedSampleCount      *prometheus.CounterVec
	decodedSeriesCount      *prometheus.CounterVec
	writtenSeriesCount      *prometheus.CounterVec
	writtenSampleCount      *prometheus.CounterVec
	responseStatusCodeCount *prometheus.CounterVec
}

func NewRefillProcessor(
	decoderBuilder DecoderBuilder,
	receiver Receiver,
	registerer prometheus.Registerer,
) *RefillProcessor {
	factory := util.NewUnconflictRegisterer(registerer)
	return &RefillProcessor{
		decoderBuilder: decoderBuilder,
		receiver:       receiver,
		criticalErrorCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_critical_error_count",
			Help: "Total number of critical errors occurred during serving metric stream.",
		}, []string{"error", "processor_type"}),
		decodedSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_decoded_series_count",
			Help: "Number of series decoded.",
		}, []string{"processor_type"}),
		decodedSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_decoded_samples_count",
			Help: "Number of samples decoded.",
		}, []string{"processor_type"}),
		writtenSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_written_series_count",
			Help: "Number of series decoded and written to prometheus",
		}, []string{"processor_type"}),
		writtenSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_written_samples_count",
			Help: "Number of samples decoded and written to prometheus",
		}, []string{"processor_type"}),
		responseStatusCodeCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_response_status_code",
			Help: "Number of 200/400 status codes responded with.",
		}, []string{"processor_type", "status_code"}),
	}
}

func (p *RefillProcessor) Process(ctx context.Context, refill Refill) error {
	p.responseStatusCodeCount.With(
		prometheus.Labels{"processor_type": "refill", "status_code": "200"},
	).Inc()

	return refill.Write(ctx, model.RefillProcessingStatus{Code: http.StatusOK})
}
