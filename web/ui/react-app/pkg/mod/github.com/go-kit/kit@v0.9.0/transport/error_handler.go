package transport

import (
	"context"

	"github.com/go-kit/kit/log"
)

// ErrorHandler receives a transport error to be processed for diagnostic purposes.
// Usually this means logging the error.
type ErrorHandler interface {
	Handle(ctx context.Context, err error)
}

// LogErrorHandler is a transport error handler implementation which logs an error.
type LogErrorHandler struct {
	logger log.Logger
}

func NewLogErrorHandler(logger log.Logger) *LogErrorHandler {
	return &LogErrorHandler{
		logger: logger,
	}
}

func (h *LogErrorHandler) Handle(ctx context.Context, err error) {
	h.logger.Log("err", err)
}
