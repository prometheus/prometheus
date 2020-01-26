package foo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
)

type ExampleService struct {
}

type ExampleRequest struct {
	I int
	S string
}
type ExampleResponse struct {
	S   string
	Err error
}

type Endpoints struct {
	ExampleEndpoint endpoint.Endpoint
}

func (f ExampleService) ExampleEndpoint(ctx context.Context, i int, s string) (string, error) {
	panic(errors.New("not implemented"))
}

func makeExampleEndpoint(f ExampleService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(ExampleRequest)
		s, err := f.ExampleEndpoint(ctx, req.I, req.S)
		return ExampleResponse{S: s, Err: err}, nil
	}
}

func inlineHandlerBuilder(m *http.ServeMux, endpoints Endpoints) {
	m.Handle("/bar", httptransport.NewServer(endpoints.ExampleEndpoint, DecodeExampleRequest, EncodeExampleResponse))
}

func NewHTTPHandler(endpoints Endpoints) http.Handler {
	m := http.NewServeMux()
	inlineHandlerBuilder(m, endpoints)
	return m
}

func DecodeExampleRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req ExampleRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}

func EncodeExampleResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
