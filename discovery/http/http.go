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

package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// DefaultSDConfig is the default HTTP SD configuration.
	DefaultSDConfig = SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
		RefreshInterval:  model.Duration(60 * time.Second),
		MinBackoff:       model.Duration(0), // No retries by default
	}
	userAgent        = version.PrometheusUserAgent()
	matchContentType = regexp.MustCompile(`^(?i:application\/json(;\s*charset=("utf-8"|utf-8))?)$`)
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for HTTP based discovery.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	URL              string                  `yaml:"url"`
	// MinBackoff is the minimum backoff duration for retries.
	// Set to 0 to disable retries (default behavior).
	// Retries use exponential backoff, doubling on each attempt,
	// with refresh_interval as the hard upper limit.
	MinBackoff model.Duration `yaml:"min_backoff,omitempty"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return newDiscovererMetrics(reg, rmi)
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "http" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts)
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

	// Validate min_backoff
	if c.MinBackoff < 0 {
		return errors.New("min_backoff must not be negative")
	}
	if c.MinBackoff > c.RefreshInterval {
		return errors.New("min_backoff must not be greater than refresh_interval")
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
	minBackoff      time.Duration
	tgLastLength    int
	metrics         *httpMetrics
	logger          *slog.Logger
}

// NewDiscovery returns a new HTTP discovery for the given config.
func NewDiscovery(conf *SDConfig, opts discovery.DiscovererOptions) (*Discovery, error) {
	m, ok := opts.Metrics.(*httpMetrics)
	if !ok {
		return nil, errors.New("invalid discovery metrics type")
	}

	if opts.Logger == nil {
		opts.Logger = promslog.NewNopLogger()
	}

	client, err := config.NewClientFromConfig(conf.HTTPClientConfig, "http", opts.HTTPClientOptions...)
	if err != nil {
		return nil, err
	}
	client.Timeout = time.Duration(conf.RefreshInterval)

	d := &Discovery{
		url:             conf.URL,
		client:          client,
		refreshInterval: time.Duration(conf.RefreshInterval), // Stored to be sent as headers.
		minBackoff:      time.Duration(conf.MinBackoff),
		metrics:         m,
		logger:          logger,
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              opts.Logger,
			Mech:                "http",
			SetName:             opts.SetName,
			Interval:            time.Duration(conf.RefreshInterval),
			RefreshF:            d.Refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)
	return d, nil
}

// retryableHTTPError wraps an HTTP status code error that should be retried.
type retryableHTTPError struct {
	statusCode int
	status     string
}

func (e *retryableHTTPError) Error() string {
	return fmt.Sprintf("server returned HTTP status %s", e.status)
}

// doRefresh performs a single HTTP SD refresh attempt without retry logic.
func (d *Discovery) doRefresh(ctx context.Context) ([]*targetgroup.Group, error) {
	req, err := http.NewRequest(http.MethodGet, d.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Prometheus-Refresh-Interval-Seconds", strconv.FormatFloat(d.refreshInterval.Seconds(), 'f', -1, 64))

	resp, err := d.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		// Wrap retryable status codes in retryableHTTPError
		if isRetryableStatusCode(resp.StatusCode) {
			return nil, &retryableHTTPError{
				statusCode: resp.StatusCode,
				status:     resp.Status,
			}
		}
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	if !matchContentType.MatchString(strings.TrimSpace(resp.Header.Get("Content-Type"))) {
		return nil, fmt.Errorf("unsupported content type %q", resp.Header.Get("Content-Type"))
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var targetGroups []*targetgroup.Group

	if err := json.Unmarshal(b, &targetGroups); err != nil {
		return nil, err
	}

	for i, tg := range targetGroups {
		if tg == nil {
			return nil, errors.New("nil target group item found")
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

	return targetGroups, nil
}

// Refresh performs HTTP SD refresh with retry logic.
func (d *Discovery) Refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	// If min_backoff is 0, retries are disabled
	if d.minBackoff == 0 {
		tgs, err := d.doRefresh(ctx)
		if err != nil {
			d.metrics.failuresCount.Inc()
		}
		return tgs, err
	}

	// Retry logic enabled - use refresh_interval as total timeout
	retryCtx, cancel := context.WithTimeout(ctx, d.refreshInterval)
	defer cancel()

	var lastErr error
	backoff := d.minBackoff
	attempt := 0

	for {
		attempt++

		// Attempt refresh - use retryCtx so HTTP request respects total timeout
		tgs, err := d.doRefresh(retryCtx)
		if err == nil {
			// Success
			if attempt > 1 {
				d.logger.Info("HTTP SD refresh succeeded after retry",
					"attempt", attempt,
				)
			}
			return tgs, nil
		}

		lastErr = err

		// Check if timeout/cancellation occurred during request
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			d.metrics.failuresCount.Inc()
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("HTTP SD refresh failed after %d attempts within %v timeout: %w", attempt, d.refreshInterval, lastErr)
			}
			return nil, lastErr
		}

		// Check if we should retry
		shouldRetry := false
		var httpErr *retryableHTTPError
		if errors.As(err, &httpErr) {
			shouldRetry = true
		} else {
			shouldRetry = isRetryableError(err)
		}

		// If error is not retryable, stop immediately
		if !shouldRetry {
			d.metrics.failuresCount.Inc()
			return nil, lastErr
		}

		// Log retry decision
		d.logger.Warn("HTTP SD refresh failed, will retry",
			"attempt", attempt,
			"backoff", backoff,
			"err", err,
		)
		d.metrics.retriesCount.WithLabelValues(strconv.Itoa(attempt)).Inc()

		// Wait before retrying with backoff
		select {
		case <-time.After(backoff):
			// Double the backoff for next attempt, capped at refresh_interval
			backoff *= 2
			if backoff > d.refreshInterval {
				backoff = d.refreshInterval
			}
		case <-retryCtx.Done():
			// Timeout or cancellation during backoff
			d.metrics.failuresCount.Inc()
			if errors.Is(retryCtx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("HTTP SD refresh failed after %d attempts within %v timeout: %w", attempt, d.refreshInterval, lastErr)
			}
			return nil, retryCtx.Err()
		}
	}
}

// urlSource returns a source ID for the i-th target group per URL.
func urlSource(url string, i int) string {
	return fmt.Sprintf("%s:%d", url, i)
}

// isRetryableStatusCode returns true if the HTTP status code indicates a retryable error.
func isRetryableStatusCode(statusCode int) bool {
	// Retry on 5xx server errors and 429 rate limit
	return statusCode >= 500 || statusCode == http.StatusTooManyRequests
}

// isRetryableError returns true if the error is a network error that should be retried.
func isRetryableError(err error) bool {
	// Do not retry on context cancellation or deadline exceeded
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for url.Error which wraps network errors
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		// Timeout errors are retryable
		if urlErr.Timeout() {
			return true
		}
		// Temporary network errors are retryable
		type temporary interface{ Temporary() bool }
		if te, ok := urlErr.Err.(temporary); ok {
			return te.Temporary()
		}
	}

	// Do not retry on other errors (e.g., HTTP 4xx, parsing errors, etc.)
	return false
}
