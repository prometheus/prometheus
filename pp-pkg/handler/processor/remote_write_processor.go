package processor

import (
	"context"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp/go/util"
)

type RemoteWriteProcessor struct {
	receiver Receiver

	responseStatusCodeCount *prometheus.CounterVec
}

func NewRemoteWriteProcessor(
	receiver Receiver,
	registerer prometheus.Registerer,
) *RemoteWriteProcessor {
	factory := util.NewUnconflictRegisterer(registerer)

	return &RemoteWriteProcessor{
		receiver: receiver,
		responseStatusCodeCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_response_status_code",
			Help: "Number of 200/400 status codes responded with.",
		}, []string{"processor_type", "status_code"}),
	}
}

func (p *RemoteWriteProcessor) Process(ctx context.Context, remoteWrite RemoteWrite) error {
	status := model.RemoteWriteProcessingStatus{Code: http.StatusOK}
	defer func() {
		p.responseStatusCodeCount.With(
			prometheus.Labels{"processor_type": "remote_write", "status_code": strconv.Itoa(status.Code)},
		).Inc()
		_ = remoteWrite.Write(ctx, status)
	}()

	rwb, err := remoteWrite.Read(ctx)
	if err != nil {
		status.Code = http.StatusBadRequest
		status.Message = err.Error()
		return err
	}

	if err := p.receiver.AppendSnappyProtobuf(ctx, rwb, remoteWrite.Metadata().RelabelerID, true); err != nil {
		status.Code = http.StatusBadRequest
		status.Message = err.Error()
		return err
	}

	return nil
}
