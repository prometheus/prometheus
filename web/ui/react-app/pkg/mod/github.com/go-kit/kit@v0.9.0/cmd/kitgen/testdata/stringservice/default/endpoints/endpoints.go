package endpoints

import "context"

import "github.com/go-kit/kit/endpoint"

import "github.com/go-kit/kit/cmd/kitgen/testdata/stringservice/default/service"

type ConcatRequest struct {
	A string
	B string
}
type ConcatResponse struct {
	S   string
	Err error
}

func MakeConcatEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(ConcatRequest)
		string1, err := s.Concat(ctx, req.A, req.B)
		return ConcatResponse{S: string1, Err: err}, nil
	}
}

type CountRequest struct {
	S string
}
type CountResponse struct {
	Count int
}

func MakeCountEndpoint(s service.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(CountRequest)
		count := s.Count(ctx, req.S)
		return CountResponse{Count: count}, nil
	}
}

type Endpoints struct {
	Concat endpoint.Endpoint
	Count  endpoint.Endpoint
}
