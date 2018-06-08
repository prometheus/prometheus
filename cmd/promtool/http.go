package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
)

const DEFAULT_TIMEOUT = 2 * time.Minute

type PrometheusHttpClientConfig struct {
	ServerURL string
}

type PrometheusHttpClient struct {
	RequestTimeout time.Duration
	HTTPClient     api.Client
}

func NewPrometheusHttpClient(cfg PrometheusHttpClientConfig) (*PrometheusHttpClient, error) {
	hc, err := api.NewClient(api.Config{
		Address: cfg.ServerURL,
	})
	if err != nil {
		return nil, err
	}
	return &PrometheusHttpClient{
		RequestTimeout: DEFAULT_TIMEOUT,
		HTTPClient:     hc,
	}, nil
}

func (c *PrometheusHttpClient) Do(req *http.Request) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.RequestTimeout)
	defer cancel()
	return c.HTTPClient.Do(ctx, req)
}
