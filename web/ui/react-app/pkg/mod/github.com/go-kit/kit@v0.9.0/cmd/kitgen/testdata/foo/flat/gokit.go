package foo

import "context"
import "encoding/json"
import "errors"
import "net/http"
import "github.com/go-kit/kit/endpoint"
import httptransport "github.com/go-kit/kit/transport/http"

type FooService struct {
}

func (f FooService) Bar(ctx context.Context, i int, s string) (string, error) {
	panic(errors.New("not implemented"))
}

type BarRequest struct {
	I int
	S string
}
type BarResponse struct {
	S   string
	Err error
}

func MakeBarEndpoint(f FooService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(BarRequest)
		s, err := f.Bar(ctx, req.I, req.S)
		return BarResponse{S: s, Err: err}, nil
	}
}

type Endpoints struct {
	Bar endpoint.Endpoint
}

func NewHTTPHandler(endpoints Endpoints) http.Handler {
	m := http.NewServeMux()
	m.Handle("/bar", httptransport.NewServer(endpoints.Bar, DecodeBarRequest, EncodeBarResponse))
	return m
}
func DecodeBarRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req BarRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeBarResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
