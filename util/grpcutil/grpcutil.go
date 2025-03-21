package grpcutil

import (
	"errors"

	"google.golang.org/grpc/status"
)

// StatusCodeFromErrorChain retrieves the first status code from the chain of gRPC error
func StatusCodeFromErrorChain(err error) (int32, bool) {
	if err == nil || len(err.Error()) == 0 {
		return 0, false
	}

	code, ok := statusCodeFromError(err)
	if ok {
		return code, true
	}
	return StatusCodeFromErrorChain(errors.Unwrap(err))
}

// statusCodeFromError retrieves the status code from the gRPC error
func statusCodeFromError(err error) (int32, bool) {
	s, ok := status.FromError(err)
	if !ok {
		return 0, false
	}

	return s.Proto().GetCode(), true
}
