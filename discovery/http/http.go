// Copyright 2021 The Prometheus Authors
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

package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/regexp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	// DefaultRetryConfig is the default retry configuration.
	DefaultRetryConfig = RetryConfig{
		MaxAttempts:     3,
		InitialInterval: model.Duration(1 * time.Second),
		MaxInterval:     model.Duration(30 * time.Second),
		Multiplier:      2.0,
	}

	// DefaultSDConfig is the default HTTP SD configuration.
	DefaultSDConfig = SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		RefreshInterval:  model.Duration(60 * time.Second),
		RetryConfig:      DefaultRetryConfig,
	}
	userAgent        = version.PrometheusUserAgent()
	matchContentType = regexp.MustCompile(`^(?i:application\/json(;\s*charset=("utf-8"|utf-8))?)$`)
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// RetryConfig configures retry behavior for HTTP SD.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the initial request).
	// Must be at least 1.
	MaxAttempts int `yaml:"max_attempts,omitempty"`
	// InitialInterval is the initial backoff duration between retries.
	InitialInterval model.Duration `yaml:"initial_interval,omitempty"`
	// MaxInterval is the maximum backoff duration between retries.
	MaxInterval model.Duration `yaml:"max_interval,omitempty"`
	// Multiplier is the factor by which the backoff duration is multiplied after each retry.
	Multiplier float64 `yaml:"multiplier,omitempty"`
}

// SDConfig is the configuration for HTTP based discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	URL              string                  `yaml:"url"`
	RetryConfig      RetryConfig             `yaml:"retry_config,omitempty"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return newDiscovererMetrics(reg, rmi)
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "http" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.HTTPClientOptions, opts.Metrics)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(any) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.URL == "" {
		return errors.New("URL is missing")
	}
	parsedURL, err := url.Parse(c.URL)
	if err != nil {
		return err
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("URL scheme must be 'http' or 'https'")
	}
	if parsedURL.Host == "" {
		return errors.New("host is missing in URL")
	}
	// Apply defaults to each retry config field individually if zero (not specified)
	if c.RetryConfig.MaxAttempts == 0 {
		c.RetryConfig.MaxAttempts = DefaultRetryConfig.MaxAttempts
	}
	if c.RetryConfig.InitialInterval == 0 {
		c.RetryConfig.InitialInterval = DefaultRetryConfig.InitialInterval
	}
	if c.RetryConfig.MaxInterval == 0 {
		c.RetryConfig.MaxInterval = DefaultRetryConfig.MaxInterval
	}
	if c.RetryConfig.Multiplier == 0 {
		c.RetryConfig.Multiplier = DefaultRetryConfig.Multiplier
	}
	// Validate retry config
	if c.RetryConfig.MaxAttempts < 1 {
		return errors.New("retry_config max_attempts must be at least 1")
	}
	if c.RetryConfig.InitialInterval <= 0 {
		return errors.New("retry_config initial_interval must be greater than 0")
	}
	if c.RetryConfig.MaxInterval <= 0 {
		return errors.New("retry_config max_interval must be greater than 0")
	}
	if c.RetryConfig.Multiplier <= 0 {
		return errors.New("retry_config multiplier must be greater than 0")
	}
	return c.HTTPClientConfig.Validate()
}

const httpSDURLLabel = model.MetaLabelPrefix + "url"

// Discovery provides service discovery functionality based
// on HTTP endpoints that return target groups in JSON format.
type Discovery struct {
	*refresh.Discovery
	url             string
	client          *http.Client
	refreshInterval time.Duration
	tgLastLength    int
	metrics         *httpMetrics
	logger          *slog.Logger
	retryConfig     RetryConfig
}

// NewDiscovery returns a new HTTP discovery for the given config.
func NewDiscovery(conf *SDConfig, logger *slog.Logger, clientOpts []config.HTTPClientOption, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*httpMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if logger == nil {
		logger = promslog.NewNopLogger()
	}

	client, err := config.NewClientFromConfig(conf.HTTPClientConfig, "http", clientOpts...)
	if err != nil {
		return nil, err
	}
	client.Timeout = time.Duration(conf.RefreshInterval)

	// Apply default retry config if not set
	retryConfig := conf.RetryConfig
	if retryConfig.MaxAttempts == 0 {
		retryConfig = DefaultRetryConfig
	}

	d := &Discovery{
		url:             conf.URL,
		client:          client,
		refreshInterval: time.Duration(conf.RefreshInterval), // Stored to be sent as headers.
		metrics:         m,
		logger:          logger,
		retryConfig:     retryConfig,
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "http",
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.Refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

func (d *Discovery) Refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	// Get retry config
	maxAttempts := d.retryConfig.MaxAttempts
	initialInterval := time.Duration(d.retryConfig.InitialInterval)
	maxInterval := time.Duration(d.retryConfig.MaxInterval)
	multiplier := d.retryConfig.Multiplier

	var lastErr error
	backoff := initialInterval

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodGet, d.url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Prometheus-Refresh-Interval-Seconds", strconv.FormatFloat(d.refreshInterval.Seconds(), 'f', -1, 64))

		resp, err := d.client.Do(req.WithContext(ctx))
		if err != nil {
			lastErr = err
			// Only retry if this is retryable and we have attempts left
			if attempt < maxAttempts && isRetryableError(err) {
				d.metrics.retriesCount.WithLabelValues(strconv.Itoa(attempt)).Inc()
				d.logger.Debug("HTTP SD refresh attempt failed, will retry",
					"attempt", attempt,
					"max_attempts", maxAttempts,
					"backoff", backoff,
					"err", err)
				select {
				case <-time.After(backoff):
					backoff = time.Duration(float64(backoff) * multiplier)
					if backoff > maxInterval {
						backoff = maxInterval
					}
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			// Non-retryable error or last attempt failed
			d.metrics.failuresCount.Inc()
			if maxAttempts > 1 && isRetryableError(err) {
				return nil, fmt.Errorf("all %d attempts failed, last error: %w", maxAttempts, lastErr)
			}
			return nil, err
		}
		defer func() {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("server returned HTTP status %s", resp.Status)
			resp.Body.Close()
			// Only retry if this is retryable and we have attempts left
			if attempt < maxAttempts && isRetryableStatusCode(resp.StatusCode) {
				d.metrics.retriesCount.WithLabelValues(strconv.Itoa(attempt)).Inc()
				d.logger.Debug("HTTP SD refresh attempt failed, will retry",
					"attempt", attempt,
					"max_attempts", maxAttempts,
					"backoff", backoff,
					"status_code", resp.StatusCode)
				select {
				case <-time.After(backoff):
					backoff = time.Duration(float64(backoff) * multiplier)
					if backoff > maxInterval {
						backoff = maxInterval
					}
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			// Non-retryable status code or last attempt failed
			if !isRetryableStatusCode(resp.StatusCode) || attempt == maxAttempts {
				d.metrics.failuresCount.Inc()
				if maxAttempts > 1 && isRetryableStatusCode(resp.StatusCode) {
					return nil, fmt.Errorf("all %d attempts failed, last error: %w", maxAttempts, lastErr)
				}
				return nil, lastErr
			}
		}

		if !matchContentType.MatchString(strings.TrimSpace(resp.Header.Get("Content-Type"))) {
			d.metrics.failuresCount.Inc()
			return nil, fmt.Errorf("unsupported content type %q", resp.Header.Get("Content-Type"))
		}

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			d.metrics.failuresCount.Inc()
			return nil, err
		}

		var targetGroups []*targetgroup.Group

		if err := json.Unmarshal(b, &targetGroups); err != nil {
			d.metrics.failuresCount.Inc()
			return nil, err
		}

		for i, tg := range targetGroups {
			if tg == nil {
				d.metrics.failuresCount.Inc()
				err = errors.New("nil target group item found")
				return nil, err
			}

			tg.Source = urlSource(d.url, i)
			if tg.Labels == nil {
				tg.Labels = model.LabelSet{}
			}
			tg.Labels[httpSDURLLabel] = model.LabelValue(d.url)
		}

		// Generate empty updates for sources that disappeared.
		l := len(targetGroups)
		for i := l; i < d.tgLastLength; i++ {
			targetGroups = append(targetGroups, &targetgroup.Group{Source: urlSource(d.url, i)})
		}
		d.tgLastLength = l

		if attempt > 1 {
			d.logger.Info("HTTP SD refresh succeeded after retry",
				"attempt", attempt)
		}

		return targetGroups, nil
	}

	// Should never reach here, but just in case
	d.metrics.failuresCount.Inc()
	return nil, fmt.Errorf("all %d attempts failed, last error: %w", maxAttempts, lastErr)
}

// isRetryableError returns true if the error is a network error that should be retried.
func isRetryableError(err error) bool {
	// Do not retry on context cancellation or deadline exceeded.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Do not retry on URL parsing errors.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// If the error is due to a network issue, retry. Otherwise, do not retry.
		// url.Error may wrap context errors, which we've already checked above.
		// If the error is a permanent error (e.g., invalid URL), do not retry.
		// If the error is a temporary network error, retry.
		if urlErr.Timeout() {
			return true
		}
		// If the underlying error is a net.OpError, check if it's temporary.
		type temporary interface{ Temporary() bool }
		if te, ok := urlErr.Err.(temporary); ok {
			return te.Temporary()
		}
		// For other url.Error, do not retry.
		return false
	}
	// For other errors, assume retryable (e.g., network errors).
	return true
}

// isRetryableStatusCode returns true if the HTTP status code indicates a retryable error.
func isRetryableStatusCode(statusCode int) bool {
	// Retry on 5xx server errors and 429 rate limit
	return statusCode >= 500 || statusCode == http.StatusTooManyRequests
}

// urlSource returns a source ID for the i-th target group per URL.
func urlSource(url string, i int) string {
	return fmt.Sprintf("%s:%d", url, i)
}
