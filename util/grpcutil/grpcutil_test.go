package grpcutil

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	spb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusCodeFromErrorChain(t *testing.T) {
	err1 := status.Error(codes.ResourceExhausted, "resource exhausted")
	err2 := errors.Wrapf(err1, "some error")

	code, ok := StatusCodeFromErrorChain(err2)
	require.True(t, ok)
	require.Equal(t, int32(codes.ResourceExhausted), code)

	err1 = status.ErrorProto(&spb.Status{
		Code:    429,
		Message: "resource exhausted",
	})
	err2 = errors.Wrapf(err1, "some error")

	code, ok = StatusCodeFromErrorChain(err2)
	require.True(t, ok)
	require.Equal(t, int32(429), code)

	err1 = errors.New("error without code")
	err2 = errors.Wrapf(err1, "some error")

	code, ok = StatusCodeFromErrorChain(err2)
	require.False(t, ok)
}
