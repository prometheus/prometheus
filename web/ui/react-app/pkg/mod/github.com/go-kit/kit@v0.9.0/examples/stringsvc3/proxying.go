package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/sony/gobreaker"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/ratelimit"
	"github.com/go-kit/kit/sd"
	"github.com/go-kit/kit/sd/lb"
	httptransport "github.com/go-kit/kit/transport/http"
)

func proxyingMiddleware(ctx context.Context, instances string, logger log.Logger) ServiceMiddleware {
	// If instances is empty, don't proxy.
	if instances == "" {
		logger.Log("proxy_to", "none")
		return func(next StringService) StringService { return next }
	}

	// Set some parameters for our client.
	var (
		qps         = 100                    // beyond which we will return an error
		maxAttempts = 3                      // per request, before giving up
		maxTime     = 250 * time.Millisecond // wallclock time, before giving up
	)

	// Otherwise, construct an endpoint for each instance in the list, and add
	// it to a fixed set of endpoints. In a real service, rather than doing this
	// by hand, you'd probably use package sd's support for your service
	// discovery system.
	var (
		instanceList = split(instances)
		endpointer   sd.FixedEndpointer
	)
	logger.Log("proxy_to", fmt.Sprint(instanceList))
	for _, instance := range instanceList {
		var e endpoint.Endpoint
		e = makeUppercaseProxy(ctx, instance)
		e = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(e)
		e = ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), qps))(e)
		endpointer = append(endpointer, e)
	}

	// Now, build a single, retrying, load-balancing endpoint out of all of
	// those individual endpoints.
	balancer := lb.NewRoundRobin(endpointer)
	retry := lb.Retry(maxAttempts, maxTime, balancer)

	// And finally, return the ServiceMiddleware, implemented by proxymw.
	return func(next StringService) StringService {
		return proxymw{ctx, next, retry}
	}
}

// proxymw implements StringService, forwarding Uppercase requests to the
// provided endpoint, and serving all other (i.e. Count) requests via the
// next StringService.
type proxymw struct {
	ctx       context.Context
	next      StringService     // Serve most requests via this service...
	uppercase endpoint.Endpoint // ...except Uppercase, which gets served by this endpoint
}

func (mw proxymw) Count(s string) int {
	return mw.next.Count(s)
}

func (mw proxymw) Uppercase(s string) (string, error) {
	response, err := mw.uppercase(mw.ctx, uppercaseRequest{S: s})
	if err != nil {
		return "", err
	}

	resp := response.(uppercaseResponse)
	if resp.Err != "" {
		return resp.V, errors.New(resp.Err)
	}
	return resp.V, nil
}

func makeUppercaseProxy(ctx context.Context, instance string) endpoint.Endpoint {
	if !strings.HasPrefix(instance, "http") {
		instance = "http://" + instance
	}
	u, err := url.Parse(instance)
	if err != nil {
		panic(err)
	}
	if u.Path == "" {
		u.Path = "/uppercase"
	}
	return httptransport.NewClient(
		"GET",
		u,
		encodeRequest,
		decodeUppercaseResponse,
	).Endpoint()
}

func split(s string) []string {
	a := strings.Split(s, ",")
	for i := range a {
		a[i] = strings.TrimSpace(a[i])
	}
	return a
}
