package main

import (
	"context"
	"flag"
	"net/http"
	"os"

	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/go-kit/kit/log"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	httptransport "github.com/go-kit/kit/transport/http"
)

func main() {
	var (
		listen = flag.String("listen", ":8080", "HTTP listen address")
		proxy  = flag.String("proxy", "", "Optional comma-separated list of URLs to proxy uppercase requests")
	)
	flag.Parse()

	var logger log.Logger
	logger = log.NewLogfmtLogger(os.Stderr)
	logger = log.With(logger, "listen", *listen, "caller", log.DefaultCaller)

	fieldKeys := []string{"method", "error"}
	requestCount := kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
		Namespace: "my_group",
		Subsystem: "string_service",
		Name:      "request_count",
		Help:      "Number of requests received.",
	}, fieldKeys)
	requestLatency := kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "my_group",
		Subsystem: "string_service",
		Name:      "request_latency_microseconds",
		Help:      "Total duration of requests in microseconds.",
	}, fieldKeys)
	countResult := kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
		Namespace: "my_group",
		Subsystem: "string_service",
		Name:      "count_result",
		Help:      "The result of each count method.",
	}, []string{})

	var svc StringService
	svc = stringService{}
	svc = proxyingMiddleware(context.Background(), *proxy, logger)(svc)
	svc = loggingMiddleware(logger)(svc)
	svc = instrumentingMiddleware(requestCount, requestLatency, countResult)(svc)

	uppercaseHandler := httptransport.NewServer(
		makeUppercaseEndpoint(svc),
		decodeUppercaseRequest,
		encodeResponse,
	)
	countHandler := httptransport.NewServer(
		makeCountEndpoint(svc),
		decodeCountRequest,
		encodeResponse,
	)

	http.Handle("/uppercase", uppercaseHandler)
	http.Handle("/count", countHandler)
	http.Handle("/metrics", promhttp.Handler())
	logger.Log("msg", "HTTP", "addr", *listen)
	logger.Log("err", http.ListenAndServe(*listen, nil))
}
