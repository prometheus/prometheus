package grpcutil

import (
	"errors"
	"net/http"

	"google.golang.org/grpc/status"
)

// HTTPStatusCodeFromErrorChain retrieves the first HTTP status code from the chain of gRPC error
func HTTPStatusCodeFromErrorChain(err error) (int32, bool) {
	if err == nil || len(err.Error()) == 0 {
		return 0, false
	}

	code, ok := statusCodeFromError(err)
	if ok && http.StatusText(int(code)) != "" {
		return code, true
	}
	return HTTPStatusCodeFromErrorChain(errors.Unwrap(err))
}

// statusCodeFromError retrieves the status code from the gRPC error
func statusCodeFromError(err error) (int32, bool) {
	s, ok := status.FromError(err)
	if !ok {
		return 0, false
	}

	return s.Proto().GetCode(), true
}
