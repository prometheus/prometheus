package addtransport

import (
	"context"
	"time"

	"golang.org/x/time/rate"

	"github.com/sony/gobreaker"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/ratelimit"

	"github.com/go-kit/kit/examples/addsvc/pkg/addendpoint"
	"github.com/go-kit/kit/examples/addsvc/pkg/addservice"
	addthrift "github.com/go-kit/kit/examples/addsvc/thrift/gen-go/addsvc"
)

type thriftServer struct {
	ctx       context.Context
	endpoints addendpoint.Set
}

// NewThriftServer makes a set of endpoints available as a Thrift service.
func NewThriftServer(endpoints addendpoint.Set) addthrift.AddService {
	return &thriftServer{
		endpoints: endpoints,
	}
}

func (s *thriftServer) Sum(ctx context.Context, a int64, b int64) (*addthrift.SumReply, error) {
	request := addendpoint.SumRequest{A: int(a), B: int(b)}
	response, err := s.endpoints.SumEndpoint(ctx, request)
	if err != nil {
		return nil, err
	}
	resp := response.(addendpoint.SumResponse)
	return &addthrift.SumReply{Value: int64(resp.V), Err: err2str(resp.Err)}, nil
}

func (s *thriftServer) Concat(ctx context.Context, a string, b string) (*addthrift.ConcatReply, error) {
	request := addendpoint.ConcatRequest{A: a, B: b}
	response, err := s.endpoints.ConcatEndpoint(ctx, request)
	if err != nil {
		return nil, err
	}
	resp := response.(addendpoint.ConcatResponse)
	return &addthrift.ConcatReply{Value: resp.V, Err: err2str(resp.Err)}, nil
}

// NewThriftClient returns an AddService backed by a Thrift server described by
// the provided client. The caller is responsible for constructing the client,
// and eventually closing the underlying transport. We bake-in certain middlewares,
// implementing the client library pattern.
func NewThriftClient(client *addthrift.AddServiceClient) addservice.Service {
	// We construct a single ratelimiter middleware, to limit the total outgoing
	// QPS from this client to all methods on the remote instance. We also
	// construct per-endpoint circuitbreaker middlewares to demonstrate how
	// that's done, although they could easily be combined into a single breaker
	// for the entire remote instance, too.
	limiter := ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100))

	// Each individual endpoint is an http/transport.Client (which implements
	// endpoint.Endpoint) that gets wrapped with various middlewares. If you
	// could rely on a consistent set of client behavior.
	var sumEndpoint endpoint.Endpoint
	{
		sumEndpoint = MakeThriftSumEndpoint(client)
		sumEndpoint = limiter(sumEndpoint)
		sumEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Sum",
			Timeout: 30 * time.Second,
		}))(sumEndpoint)
	}

	// The Concat endpoint is the same thing, with slightly different
	// middlewares to demonstrate how to specialize per-endpoint.
	var concatEndpoint endpoint.Endpoint
	{
		concatEndpoint = MakeThriftConcatEndpoint(client)
		concatEndpoint = limiter(concatEndpoint)
		concatEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Concat",
			Timeout: 10 * time.Second,
		}))(concatEndpoint)
	}

	// Returning the endpoint.Set as a service.Service relies on the
	// endpoint.Set implementing the Service methods. That's just a simple bit
	// of glue code.
	return addendpoint.Set{
		SumEndpoint:    sumEndpoint,
		ConcatEndpoint: concatEndpoint,
	}
}

// MakeThriftSumEndpoint returns an endpoint that invokes the passed Thrift client.
// Useful only in clients, and only until a proper transport/thrift.Client exists.
func MakeThriftSumEndpoint(client *addthrift.AddServiceClient) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(addendpoint.SumRequest)
		reply, err := client.Sum(ctx, int64(req.A), int64(req.B))
		if err == addservice.ErrIntOverflow {
			return nil, err // special case; see comment on ErrIntOverflow
		}
		return addendpoint.SumResponse{V: int(reply.Value), Err: err}, nil
	}
}

// MakeThriftConcatEndpoint returns an endpoint that invokes the passed Thrift
// client. Useful only in clients, and only until a proper
// transport/thrift.Client exists.
func MakeThriftConcatEndpoint(client *addthrift.AddServiceClient) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(addendpoint.ConcatRequest)
		reply, err := client.Concat(ctx, req.A, req.B)
		return addendpoint.ConcatResponse{V: reply.Value, Err: err}, nil
	}
}
