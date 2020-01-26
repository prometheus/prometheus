package server

import (
	"context"

	examples "github.com/grpc-ecosystem/grpc-gateway/examples/proto/examplepb"
)

// Implements of ResponseBodyServiceServer

type responseBodyServer struct{}

func newResponseBodyServer() examples.ResponseBodyServiceServer {
	return new(responseBodyServer)
}

func (s *responseBodyServer) GetResponseBody(ctx context.Context, req *examples.ResponseBodyIn) (*examples.ResponseBodyOut, error) {
	return &examples.ResponseBodyOut{
		Response: &examples.ResponseBodyOut_Response{
			Data: req.Data,
		},
	}, nil
}

func (s *responseBodyServer) ListResponseBodies(ctx context.Context, req *examples.ResponseBodyIn) (*examples.RepeatedResponseBodyOut, error) {
	return &examples.RepeatedResponseBodyOut{
		Response: []*examples.RepeatedResponseBodyOut_Response{
			&examples.RepeatedResponseBodyOut_Response{
				Data: req.Data,
			},
		},
	}, nil
}

func (s *responseBodyServer) ListResponseStrings(ctx context.Context, req *examples.ResponseBodyIn) (*examples.RepeatedResponseStrings, error) {
	if req.Data == "empty" {
		return &examples.RepeatedResponseStrings{
			Values: []string{},
		}, nil
	}
	return &examples.RepeatedResponseStrings{
		Values: []string{"hello", req.Data},
	}, nil
}
