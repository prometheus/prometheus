package handler

import (
	"net/http"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/net/websocket"

	"github.com/prometheus/prometheus/pp-pkg/handler/adapter"
	"github.com/prometheus/prometheus/pp-pkg/handler/decoder/opcore"
	"github.com/prometheus/prometheus/pp-pkg/handler/middleware"
	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage/block"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/util/pool"
)

// OpHandler service for remote write via opprotocol.
type OpHandler struct {
	receiver    Receiver
	logger      log.Logger
	stream      StreamProcessor
	refill      RefillProcessor
	remoteWrite RemoteWriteProcessor
	buffers     *pool.Pool
	stop        *atomic.Bool
	// stats
	activeConnections *prometheus.GaugeVec
}

// NewOpHandler init new OpHandler.
func NewOpHandler(
	receiver Receiver,
	logger log.Logger,
	registerer prometheus.Registerer,
) *OpHandler {
	// TODO const or config parameter?
	opLocalStoragePath := "opdata/"
	opBlockStorage := block.NewStorage(opLocalStoragePath)
	factory := util.NewUnconflictRegisterer(registerer)
	h := &OpHandler{
		receiver:    receiver,
		logger:      log.With(logger, "component", "op_handler"),
		stream:      processor.NewStreamProcessor(opcore.NewBuilder(opBlockStorage), receiver, registerer),
		refill:      processor.NewRefillProcessor(opcore.NewReplayDecoderBuilder(opBlockStorage), receiver, registerer),
		remoteWrite: processor.NewRemoteWriteProcessor(receiver, registerer),
		buffers:     pool.New(4e3, 1e6, 3, func(sz int) interface{} { return make([]byte, 0, sz) }),
		stop:        new(atomic.Bool),
		// stats
		activeConnections: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "remote_write_opprotocol_active_connections_count",
				Help: "Number of opprotocol active connections.",
			},
			[]string{"type"},
		),
	}

	level.Info(h.logger).Log("msg", "created")

	return h
}

// Websocket handler for websocket stream.
func (h *OpHandler) Websocket(middlewares ...middleware.Middleware) http.HandlerFunc {
	hf := h.metadataValidator(websocket.Handler(h.websocketHandler).ServeHTTP)
	for _, mw := range middlewares {
		hf = mw(hf)
	}
	return h.measure(hf, "stream")
}

// Refill handler for refill.
func (h *OpHandler) Refill(middlewares ...middleware.Middleware) http.HandlerFunc {
	hf := h.metadataValidator(h.refillHandler())
	for _, mw := range middlewares {
		hf = mw(hf)
	}
	return h.measure(hf, "refill")
}

// RemoteWrite handler for RemoteWrite.
func (h *OpHandler) RemoteWrite(middlewares ...middleware.Middleware) http.HandlerFunc {
	hf := h.metadataValidator(h.remoteWriteHandler())
	for _, mw := range middlewares {
		hf = mw(hf)
	}
	return h.measure(hf, "remote_write")
}

// measure middleware for metrics.
func (h *OpHandler) measure(next http.Handler, typeHandler string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if h.stop.Load() {
			rw.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		h.activeConnections.With(prometheus.Labels{"type": typeHandler}).Inc()
		defer h.activeConnections.With(prometheus.Labels{"type": typeHandler}).Dec()

		next.ServeHTTP(rw, r)
	}
}

// metadataValidator validate metadata.
func (h *OpHandler) metadataValidator(next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		metadata := middleware.MetadataFromContext(r.Context())
		if ok := h.receiver.RelabelerIDIsExist(metadata.RelabelerID); !ok {
			level.Error(h.logger).Log("msg", "relabeler id not found", "relabeler_id", metadata.RelabelerID)
			rw.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		next.ServeHTTP(rw, r)
	}
}

// websocketHandler handler for websocket.
func (h *OpHandler) websocketHandler(wconn *websocket.Conn) {
	defer func() { _ = wconn.Close() }()
	wconn.PayloadType = websocket.BinaryFrame
	ctx := wconn.Request().Context()
	metadata := middleware.MetadataFromContext(ctx)
	if err := h.stream.Process(ctx, adapter.NewStream(wconn, &metadata)); err != nil {
		level.Error(h.logger).Log("msg", "failed processing stream", "err", err)
		return
	}
}

// refillHandler handler for refill.
func (h *OpHandler) refillHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		metadata := middleware.MetadataFromContext(ctx)
		if err := h.refill.Process(ctx, adapter.NewRefill(request.Body, writer, &metadata)); err != nil {
			level.Error(h.logger).Log("msg", "failed processing refill", "err", err)
			return
		}
	}
}

// remoteWriteHandler handler for RemoteWrite.
func (h *OpHandler) remoteWriteHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		metadata := middleware.MetadataFromContext(ctx)
		contentLength, _ := strconv.Atoi(request.Header.Get("content-length"))
		if err := h.remoteWrite.Process(
			ctx,
			adapter.NewRemoteWrite(
				request.Body,
				writer,
				&metadata,
				h.buffers,
				contentLength,
			),
		); err != nil {
			level.Error(h.logger).Log("msg", "failed processing remote_write", "err", err)
			return
		}
	}
}

// Shutdown set the stop flag and reject all incoming requests.
func (h *OpHandler) Shutdown() {
	h.stop.Store(true)
	level.Info(h.logger).Log("msg", "stopped")
}
