package endpoints

import "context"

import "github.com/go-kit/kit/endpoint"

import "github.com/go-kit/kit/cmd/kitgen/testdata/foo/default/service"

type BarRequest struct {
	I int
	S string
}
type BarResponse struct {
	S   string
	Err error
}

func MakeBarEndpoint(f service.FooService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(BarRequest)
		s, err := f.Bar(ctx, req.I, req.S)
		return BarResponse{S: s, Err: err}, nil
	}
}

type Endpoints struct {
	Bar endpoint.Endpoint
}
