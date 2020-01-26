package addtransport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/examples/addsvc/pkg/addendpoint"
	"github.com/go-kit/kit/examples/addsvc/pkg/addservice"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/ratelimit"
	"github.com/go-kit/kit/tracing/opentracing"
	"github.com/go-kit/kit/transport/http/jsonrpc"
	stdopentracing "github.com/opentracing/opentracing-go"
	"github.com/sony/gobreaker"
)

// NewJSONRPCHandler returns a JSON RPC Server/Handler that can be passed to http.Handle()
func NewJSONRPCHandler(endpoints addendpoint.Set, logger log.Logger) *jsonrpc.Server {
	handler := jsonrpc.NewServer(
		makeEndpointCodecMap(endpoints),
		jsonrpc.ServerErrorLogger(logger),
	)
	return handler
}

// NewJSONRPCClient returns an addservice backed by a JSON RPC over HTTP server
// living at the remote instance. We expect instance to come from a service
// discovery system, so likely of the form "host:port". We bake-in certain
// middlewares, implementing the client library pattern.
func NewJSONRPCClient(instance string, tracer stdopentracing.Tracer, logger log.Logger) (addservice.Service, error) {
	// Quickly sanitize the instance string.
	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	u, err := url.Parse(instance)
	if err != nil {
		return nil, err
	}

	// We construct a single ratelimiter middleware, to limit the total outgoing
	// QPS from this client to all methods on the remote instance. We also
	// construct per-endpoint circuitbreaker middlewares to demonstrate how
	// that's done, although they could easily be combined into a single breaker
	// for the entire remote instance, too.
	limiter := ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100))

	var sumEndpoint endpoint.Endpoint
	{
		sumEndpoint = jsonrpc.NewClient(
			u,
			"sum",
			jsonrpc.ClientRequestEncoder(encodeSumRequest),
			jsonrpc.ClientResponseDecoder(decodeSumResponse),
		).Endpoint()
		sumEndpoint = opentracing.TraceClient(tracer, "Sum")(sumEndpoint)
		sumEndpoint = limiter(sumEndpoint)
		sumEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Sum",
			Timeout: 30 * time.Second,
		}))(sumEndpoint)
	}

	var concatEndpoint endpoint.Endpoint
	{
		concatEndpoint = jsonrpc.NewClient(
			u,
			"concat",
			jsonrpc.ClientRequestEncoder(encodeConcatRequest),
			jsonrpc.ClientResponseDecoder(decodeConcatResponse),
		).Endpoint()
		concatEndpoint = opentracing.TraceClient(tracer, "Concat")(concatEndpoint)
		concatEndpoint = limiter(concatEndpoint)
		concatEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:    "Concat",
			Timeout: 30 * time.Second,
		}))(concatEndpoint)
	}

	// Returning the endpoint.Set as a service.Service relies on the
	// endpoint.Set implementing the Service methods. That's just a simple bit
	// of glue code.
	return addendpoint.Set{
		SumEndpoint:    sumEndpoint,
		ConcatEndpoint: concatEndpoint,
	}, nil

}

// makeEndpointCodecMap returns a codec map configured for the addsvc.
func makeEndpointCodecMap(endpoints addendpoint.Set) jsonrpc.EndpointCodecMap {
	return jsonrpc.EndpointCodecMap{
		"sum": jsonrpc.EndpointCodec{
			Endpoint: endpoints.SumEndpoint,
			Decode:   decodeSumRequest,
			Encode:   encodeSumResponse,
		},
		"concat": jsonrpc.EndpointCodec{
			Endpoint: endpoints.ConcatEndpoint,
			Decode:   decodeConcatRequest,
			Encode:   encodeConcatResponse,
		},
	}
}

func decodeSumRequest(_ context.Context, msg json.RawMessage) (interface{}, error) {
	var req addendpoint.SumRequest
	err := json.Unmarshal(msg, &req)
	if err != nil {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("couldn't unmarshal body to sum request: %s", err),
		}
	}
	return req, nil
}

func encodeSumResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(addendpoint.SumResponse)
	if !ok {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("Asserting result to *SumResponse failed. Got %T, %+v", obj, obj),
		}
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("couldn't marshal response: %s", err)
	}
	return b, nil
}

func decodeSumResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var sumres addendpoint.SumResponse
	err := json.Unmarshal(res.Result, &sumres)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal body to SumResponse: %s", err)
	}
	return sumres, nil
}

func encodeSumRequest(_ context.Context, obj interface{}) (json.RawMessage, error) {
	req, ok := obj.(addendpoint.SumRequest)
	if !ok {
		return nil, fmt.Errorf("couldn't assert request as SumRequest, got %T", obj)
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't marshal request: %s", err)
	}
	return b, nil
}

func decodeConcatRequest(_ context.Context, msg json.RawMessage) (interface{}, error) {
	var req addendpoint.ConcatRequest
	err := json.Unmarshal(msg, &req)
	if err != nil {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("couldn't unmarshal body to concat request: %s", err),
		}
	}
	return req, nil
}

func encodeConcatResponse(_ context.Context, obj interface{}) (json.RawMessage, error) {
	res, ok := obj.(addendpoint.ConcatResponse)
	if !ok {
		return nil, &jsonrpc.Error{
			Code:    -32000,
			Message: fmt.Sprintf("Asserting result to *ConcatResponse failed. Got %T, %+v", obj, obj),
		}
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, fmt.Errorf("couldn't marshal response: %s", err)
	}
	return b, nil
}

func decodeConcatResponse(_ context.Context, res jsonrpc.Response) (interface{}, error) {
	if res.Error != nil {
		return nil, *res.Error
	}
	var concatres addendpoint.ConcatResponse
	err := json.Unmarshal(res.Result, &concatres)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal body to ConcatResponse: %s", err)
	}
	return concatres, nil
}

func encodeConcatRequest(_ context.Context, obj interface{}) (json.RawMessage, error) {
	req, ok := obj.(addendpoint.ConcatRequest)
	if !ok {
		return nil, fmt.Errorf("couldn't assert request as ConcatRequest, got %T", obj)
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("couldn't marshal request: %s", err)
	}
	return b, nil
}
