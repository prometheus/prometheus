package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
)

const DEFAULT_TIMEOUT = 2 * time.Minute

type HTTPClientConfig struct {
	ServerURL string
}

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, []byte, error)
	URLJoin(path string) string
}

func NewHTTPClient(cfg HTTPClientConfig) (HTTPClient, error) {
	hc, err := api.NewClient(api.Config{
		Address: cfg.ServerURL,
	})
	if err != nil {
		return nil, err
	}
	return &PrometheusHTTPClient{
		RequestTimeout: DEFAULT_TIMEOUT,
		HTTPClient:     hc,
	}, nil
}

type PrometheusHTTPClient struct {
	RequestTimeout time.Duration
	HTTPClient     api.Client
	ServerURL      string
}

func (c *PrometheusHTTPClient) Do(req *http.Request) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.RequestTimeout)
	defer cancel()
	return c.HTTPClient.Do(ctx, req)
}

func (c *PrometheusHTTPClient) URLJoin(path string) string {
	return c.HTTPClient.URL(path, nil).String()
}
