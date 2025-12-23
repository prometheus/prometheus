// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MockExporter serves Prometheus metrics with TYPE/HELP/UNIT metadata,
// including a native histogram.
type MockExporter struct {
	server   *http.Server
	listener net.Listener
	addr     string

	mu       sync.Mutex // protects handler and registry swaps
	registry *prometheus.Registry
	handler  http.Handler

	// Metrics
	httpRequestsTotal  *prometheus.CounterVec
	temperatureCelsius *prometheus.GaugeVec
	requestDuration    prometheus.Histogram
	responseSizeBytes  *prometheus.SummaryVec
}

// NewMockExporter creates a new mock exporter that will listen on the given address.
func NewMockExporter(addr string) *MockExporter {
	registry := prometheus.NewRegistry()

	// Counter metric (no unit - counters typically don't have units)
	httpRequestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "demo_http_requests_total",
			Help: "Total number of HTTP requests received.",
		},
		[]string{"method", "status"},
	)

	// Gauge metric with unit suffix
	temperatureCelsius := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "demo_temperature_celsius",
			Help: "Current temperature in the demo environment.",
		},
		[]string{"location"},
	)

	// Native histogram with exponential buckets
	// NativeHistogramBucketFactor > 1.0 enables native histograms
	requestDuration := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:                            "demo_request_duration_seconds",
			Help:                            "Time spent processing requests.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
	)

	// Summary metric with unit suffix
	responseSizeBytes := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "demo_response_size_bytes",
			Help:       "Size of HTTP responses.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{},
	)

	// Register all metrics
	registry.MustRegister(httpRequestsTotal)
	registry.MustRegister(temperatureCelsius)
	registry.MustRegister(requestDuration)
	registry.MustRegister(responseSizeBytes)

	m := &MockExporter{
		addr:               addr,
		registry:           registry,
		httpRequestsTotal:  httpRequestsTotal,
		temperatureCelsius: temperatureCelsius,
		requestDuration:    requestDuration,
		responseSizeBytes:  responseSizeBytes,
	}
	m.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	return m
}

// Start begins serving metrics.
func (m *MockExporter) Start() error {
	listener, err := net.Listen("tcp", m.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	m.listener = listener
	m.addr = listener.Addr().String()

	// Seed the metrics with some initial data
	m.seedMetrics()

	mux := http.NewServeMux()
	// Delegate to the current handler so UpgradeMetrics can swap it.
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		h := m.handler
		m.mu.Unlock()
		h.ServeHTTP(w, r)
	})

	m.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() {
		if err := m.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("exporter server error: %v\n", err)
		}
	}()

	return nil
}

// seedMetrics populates metrics with sample data.
func (m *MockExporter) seedMetrics() {
	// Seed counter
	m.httpRequestsTotal.WithLabelValues("GET", "200").Add(100)
	m.httpRequestsTotal.WithLabelValues("POST", "200").Add(50)
	m.httpRequestsTotal.WithLabelValues("GET", "404").Add(10)

	// Seed gauge
	m.temperatureCelsius.WithLabelValues("server_room").Set(23.5)
	m.temperatureCelsius.WithLabelValues("office").Set(21.0)

	// Seed native histogram with random observations
	rng := rand.New(rand.NewSource(42))
	for range 1000 {
		// Generate realistic request durations (exponential distribution)
		duration := rng.ExpFloat64() * 0.1 // Mean ~100ms
		m.requestDuration.Observe(duration)
	}

	// Seed summary
	for range 500 {
		size := rng.Float64() * 10000 // 0-10KB
		m.responseSizeBytes.WithLabelValues().Observe(size)
	}
}

// UpgradeMetrics simulates an exporter binary upgrade by creating a fresh
// registry with changed help text for two metrics, then swapping the handler.
func (m *MockExporter) UpgradeMetrics() {
	registry := prometheus.NewRegistry()

	m.httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "demo_http_requests_total",
			Help: "Total HTTP requests processed by the server.",
		},
		[]string{"method", "status"},
	)
	m.temperatureCelsius = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "demo_temperature_celsius",
			Help: "Current temperature in the demo environment.",
		},
		[]string{"location"},
	)
	m.requestDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:                            "demo_request_duration_seconds",
			Help:                            "Latency of request processing in seconds.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: 1 * time.Hour,
		},
	)
	m.responseSizeBytes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "demo_response_size_bytes",
			Help:       "Size of HTTP responses.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{},
	)

	registry.MustRegister(m.httpRequestsTotal)
	registry.MustRegister(m.temperatureCelsius)
	registry.MustRegister(m.requestDuration)
	registry.MustRegister(m.responseSizeBytes)

	// Seed with fresh data.
	m.httpRequestsTotal.WithLabelValues("GET", "200").Add(200)
	m.httpRequestsTotal.WithLabelValues("POST", "200").Add(75)
	m.httpRequestsTotal.WithLabelValues("GET", "404").Add(15)
	m.temperatureCelsius.WithLabelValues("server_room").Set(24.1)
	m.temperatureCelsius.WithLabelValues("office").Set(21.5)

	rng := rand.New(rand.NewSource(99))
	for range 500 {
		m.requestDuration.Observe(rng.ExpFloat64() * 0.1)
	}
	for range 250 {
		m.responseSizeBytes.WithLabelValues().Observe(rng.Float64() * 10000)
	}

	m.mu.Lock()
	m.registry = registry
	m.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	m.mu.Unlock()
}

// Stop gracefully shuts down the exporter.
func (m *MockExporter) Stop() error {
	if m.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.server.Shutdown(ctx)
}

// Addr returns the address the exporter is listening on.
func (m *MockExporter) Addr() string {
	return m.addr
}
