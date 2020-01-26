package foo

import "context"
import "encoding/json"
import "errors"
import "net/http"
import "github.com/go-kit/kit/endpoint"
import httptransport "github.com/go-kit/kit/transport/http"

type Service struct {
}

func (s Service) Concat(ctx context.Context, a string, b string) (string, error) {
	panic(errors.New("not implemented"))
}

type ConcatRequest struct {
	A string
	B string
}
type ConcatResponse struct {
	S   string
	Err error
}

func MakeConcatEndpoint(s Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(ConcatRequest)
		string1, err := s.Concat(ctx, req.A, req.B)
		return ConcatResponse{S: string1, Err: err}, nil
	}
}
func (s Service) Count(ctx context.Context, string1 string) int {
	panic(errors.New("not implemented"))
}

type CountRequest struct {
	S string
}
type CountResponse struct {
	Count int
}

func MakeCountEndpoint(s Service) endpoint.Endpoint {
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

func NewHTTPHandler(endpoints Endpoints) http.Handler {
	m := http.NewServeMux()
	m.Handle("/concat", httptransport.NewServer(endpoints.Concat, DecodeConcatRequest, EncodeConcatResponse))
	m.Handle("/count", httptransport.NewServer(endpoints.Count, DecodeCountRequest, EncodeCountResponse))
	return m
}
func DecodeConcatRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req ConcatRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeConcatResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
func DecodeCountRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var req CountRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}
func EncodeCountResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	return json.NewEncoder(w).Encode(response)
}
