package underscores

import "context"
import "encoding/json"
import "errors"
import "net/http"
import "github.com/go-kit/kit/endpoint"
import httptransport "github.com/go-kit/kit/transport/http"

type Service struct {
}

func (s Service) Foo(ctx context.Context, i int) (int, error) {
	panic(errors.New("not implemented"))
}

type FooRequest struct {
	I int
}
type FooResponse struct {
	I   int
	Err error
}

func MakeFooEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(FooRequest)
		i, err := s.Foo(ctx, req.I)
		return FooResponse{I: i, Err: err}, nil
	}
}

type Endpoints struct {
	Foo endpoint.Endpoint
}

func NewHTTPHandler(endpoints Endpoints) http.Handler {
	m := http.NewServeMux()
	m.Handle("/foo", httptransport.NewServer(endpoints.Foo, DecodeFooRequest, EncodeFooResponse))
	return m
}
func DecodeFooRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req FooRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeFooResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
