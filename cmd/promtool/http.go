package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
)

const DEFAULT_TIMEOUT = 2 * time.Minute

var promClient *PrometheusHttpClient

type PrometheusHttpClientConfig struct {
	ServerAddress string
}

type PrometheusHttpClient struct {
	Server         *url.URL
	RequestTimeout time.Duration
	HTTPClient     api.Client
}

func NewPrometheusHttpClient(cfg PrometheusHttpClientConfig) (*PrometheusHttpClient, error) {
	u, err := url.Parse(cfg.ServerAddress)
	if err != nil {
		return nil, err
	}
	u.Path = strings.TrimRight(u.Path, "/")
	hc, err := api.NewClient(api.Config{
		Address: u.String(),
	})
	if err != nil {
		return nil, err
	}
	return &PrometheusHttpClient{
		Server:         u,
		RequestTimeout: DEFAULT_TIMEOUT,
		HTTPClient:     hc,
	}, nil
}

func (c *PrometheusHttpClient) Do(req *http.Request) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.RequestTimeout)
	defer cancel()
	return c.HTTPClient.Do(ctx, req)
}

func initPromClient(cfg PrometheusHttpClientConfig) {
	var err error
	promClient, err = NewPrometheusHttpClient(cfg)
	if err != nil {
		panic(err)
	}
}

type BodyGetterConfig struct {
	Path string
}

func NewBodyGetter(cfg BodyGetterConfig) *BodyGetter {
	req, err := http.NewRequest(http.MethodGet, promClient.Server.String()+cfg.Path, nil)
	if err != nil {
		panic(err)
	}
	return &BodyGetter{
		Request: req,
	}
}

type BodyGetter struct {
	Request *http.Request
}

func (c *BodyGetter) Get() ([]byte, error) {
	_, body, err := promClient.Do(c.Request)

	return body, err
}
