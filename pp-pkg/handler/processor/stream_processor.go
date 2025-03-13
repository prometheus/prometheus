package processor

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/util"
)

type StreamProcessor struct {
	decoderBuilder DecoderBuilder
	receiver       Receiver

	criticalErrorCount      *prometheus.CounterVec
	rejectedSegmentCount    *prometheus.CounterVec
	decodedSampleCount      *prometheus.CounterVec
	decodedSeriesCount      *prometheus.CounterVec
	writtenSeriesCount      *prometheus.CounterVec
	writtenSampleCount      *prometheus.CounterVec
	responseStatusCodeCount *prometheus.CounterVec
}

func NewStreamProcessor(
	decoderBuilder DecoderBuilder,
	receiver Receiver,
	registerer prometheus.Registerer,
) *StreamProcessor {
	factory := util.NewUnconflictRegisterer(registerer)

	return &StreamProcessor{
		decoderBuilder: decoderBuilder,
		receiver:       receiver,
		criticalErrorCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_critical_error_count",
			Help: "Total number of critical errors occurred during serving metric stream.",
		}, []string{"error", "processor_type"}),
		rejectedSegmentCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_rejected_segment_count",
			Help: "Number of rejected segments",
		}, []string{"processor_type"}),
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

func (p *StreamProcessor) Process(ctx context.Context, stream MetricStream) error {
	meta := stream.Metadata()
	decoder := p.decoderBuilder.Build(meta)
	defer func() { _ = decoder.Close() }()

	var err error
	var encodedSegment model.Segment
	var decodedSegment cppbridge.HashdexContent

	for {
		encodedSegment, err = stream.Read(ctx)
		if err != nil {
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "stream"}).Inc()
			return fmt.Errorf("failed to read from stream: %w", err)
		}

		if !encodedSegment.IsValid() {
			err = errors.New("corrupted segment")
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "stream"}).Inc()
			return err
		}

		if len(encodedSegment.Body) == 0 {
			return decoder.Discard()
		}

		decodedSegment, err = decoder.DecodeToHashdex(ctx, encodedSegment)
		if err != nil {
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "stream"}).Inc()
			return fmt.Errorf("failed to decoded segment: %w", err)
		}

		p.decodedSeriesCount.With(prometheus.Labels{"processor_type": "stream"}).Add(float64(decodedSegment.Series()))
		p.decodedSampleCount.With(prometheus.Labels{"processor_type": "stream"}).Add(float64(decodedSegment.Samples()))

		processingStatus := model.SegmentProcessingStatus{
			SegmentID: decodedSegment.SegmentID(),
			Code:      model.ProcessingStatusOk,
			Message:   "ok",
			Timestamp: decodedSegment.CreatedAt(),
		}

		if err = p.receiver.AppendHashdex(
			ctx,
			decodedSegment.ShardedData(),
			meta.RelabelerID,
			true,
		); err != nil {
			processingStatus.Code = model.ProcessingStatusRejected
			processingStatus.Message = err.Error()
			p.rejectedSegmentCount.With(prometheus.Labels{"processor_type": "stream"}).Inc()
		} else {
			p.writtenSeriesCount.With(
				prometheus.Labels{"processor_type": "stream"},
			).Add(float64(decodedSegment.Series()))
			p.writtenSampleCount.With(
				prometheus.Labels{"processor_type": "stream"},
			).Add(float64(decodedSegment.Samples()))
		}

		p.responseStatusCodeCount.With(
			prometheus.Labels{"processor_type": "stream", "status_code": strconv.Itoa(int(processingStatus.Code))},
		).Inc()

		if writeErr := stream.Write(ctx, processingStatus); err != nil {
			return writeErr
		}
	}
}
