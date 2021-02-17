// Copyright 2015 The Prometheus Authors
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
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	patFileSDName = regexp.MustCompile(`^[^*]*(\*[^/]*)?\.(json|yml|yaml|JSON|YML|YAML)$`)

	// DefaultSDConfig is the default http SD configuration.
	DefaultSDConfig = SDConfig{
		RefreshInterval: model.Duration(5 * time.Minute),
	}
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for http based discovery.
type SDConfig struct {
	Paths           []string       `yaml:"paths"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "file" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger), nil
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	for i, file := range c.Paths {
		c.Paths[i] = config.JoinDir(dir, file)
	}
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if len(c.Paths) == 0 {
		return errors.New("file service discovery config must contain at least one path name")
	}
	for _, name := range c.Paths {
		if !patFileSDName.MatchString(name) {
			return errors.Errorf("path name %q is not valid for file discovery", name)
		}
	}
	return nil
}

const httpSDFilepathLabel = model.MetaLabelPrefix + "filepath"

// TimestampCollector is a Custom Collector for Timestamps of the remote files.
type TimestampCollector struct {
	Description *prometheus.Desc
	discoverers map[*Discovery]struct{}
	lock        sync.RWMutex
}

// Describe method sends the description to the channel.
func (t *TimestampCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- t.Description
}

// Collect creates constant metrics for each path with last modified time of the remote file.
func (t *TimestampCollector) Collect(ch chan<- prometheus.Metric) {
	// New map to dedup paths.
	uniquePaths := make(map[string]float64)
	t.lock.RLock()
	for httpSD := range t.discoverers {
		httpSD.lock.RLock()
		for path, timestamp := range httpSD.timestamps {
			uniquePaths[path] = timestamp
		}
		httpSD.lock.RUnlock()
	}
	t.lock.RUnlock()
	for path, timestamp := range uniquePaths {
		ch <- prometheus.MustNewConstMetric(
			t.Description,
			prometheus.GaugeValue,
			timestamp,
			path,
		)
	}
}

func (t *TimestampCollector) addDiscoverer(disc *Discovery) {
	t.lock.Lock()
	t.discoverers[disc] = struct{}{}
	t.lock.Unlock()
}

func (t *TimestampCollector) removeDiscoverer(disc *Discovery) {
	t.lock.Lock()
	delete(t.discoverers, disc)
	t.lock.Unlock()
}

// NewTimestampCollector creates a TimestampCollector.
func NewTimestampCollector() *TimestampCollector {
	return &TimestampCollector{
		Description: prometheus.NewDesc(
			"prometheus_sd_http_mtime_seconds",
			"Timestamp (mtime) of files read by FileSD. Timestamp is set at read time.",
			[]string{"filename"},
			nil,
		),
		discoverers: make(map[*Discovery]struct{}),
	}
}

var (
	httpSDScanDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name:       "prometheus_sd_http_scan_duration_seconds",
			Help:       "The duration of the HTTP-SD scan in seconds.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		})
	httpSDReadErrorsCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_http_read_errors_total",
			Help: "The number of HTTP-SD read errors.",
		})
	httpSDTimeStamp = NewTimestampCollector()
)

func init() {
	prometheus.MustRegister(httpSDScanDuration)
	prometheus.MustRegister(httpSDReadErrorsCount)
	prometheus.MustRegister(httpSDTimeStamp)
}

// Discovery provides service discovery functionality based
// on paths that contain target groups in JSON or YAML format. Refreshing
// happens using periodic refreshes.
type Discovery struct {
	paths      []string
	interval   time.Duration
	timestamps map[string]float64
	lock       sync.RWMutex

	// lastRefresh stores which paths were retrievable during the last refresh
	// and how many target groups they contained.
	// This is used to detect deleted target groups.
	lastRefresh map[string]int
	logger      log.Logger
}

// NewDiscovery returns a new http discovery for the given paths.
func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	disc := &Discovery{
		paths:      conf.Paths,
		interval:   time.Duration(conf.RefreshInterval),
		timestamps: make(map[string]float64),
		logger:     logger,
	}
	httpSDTimeStamp.addDiscoverer(disc)
	return disc
}

// listPaths returns a list of all paths that are supported.
func (d *Discovery) listPaths() []string {
	var paths []string
	for _, p := range d.paths {
		// Add paths.  The suffix will be tested later.
		if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
			paths = append(paths, p)
			continue
		}
		level.Error(d.logger).Log("msg", "Invalid http path", p)
	}
	return paths
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	d.refresh(ctx, ch)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			// Refresh remote http endpoints
			d.refresh(ctx, ch)
		}
	}
}

func (d *Discovery) writeTimestamp(path string, timestamp float64) {
	d.lock.Lock()
	d.timestamps[path] = timestamp
	d.lock.Unlock()
}

func (d *Discovery) deleteTimestamp(path string) {
	d.lock.Lock()
	delete(d.timestamps, path)
	d.lock.Unlock()
}

// refresh reads all paths matching the discovery's patterns and sends the respective
// updated target groups through the channel.
func (d *Discovery) refresh(ctx context.Context, ch chan<- []*targetgroup.Group) {
	t0 := time.Now()
	defer func() {
		httpSDScanDuration.Observe(time.Since(t0).Seconds())
	}()
	ref := map[string]int{}
	for _, p := range d.listPaths() {
		tgroups, err := d.readFile(p)
		if err != nil {
			httpSDReadErrorsCount.Inc()

			level.Error(d.logger).Log("msg", "Error reading http", "path", p, "err", err)
			// Prevent deletion down below.
			ref[p] = d.lastRefresh[p]
			continue
		}
		select {
		case ch <- tgroups:
		case <-ctx.Done():
			return
		}

		ref[p] = len(tgroups)
	}
	// Send empty updates for sources that disappeared.
	for f, n := range d.lastRefresh {
		m, ok := ref[f]
		if !ok || n > m {
			level.Debug(d.logger).Log("msg", "http_sd refresh found paths that should be removed", "path", f)
			d.deleteTimestamp(f)
			for i := m; i < n; i++ {
				select {
				case ch <- []*targetgroup.Group{{Source: fileSource(f, i)}}:
				case <-ctx.Done():
					return
				}
			}
		}
	}
	d.lastRefresh = ref

	client.CloseIdleConnections()
}

var client = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
		DisableCompression: false,
	},
}

// readFile reads a JSON or YAML list of targets groups from the path, depending on its
// extension. It returns full configuration target groups.
func (d *Discovery) readFile(path string) ([]*targetgroup.Group, error) {
	var content []byte
	var modTime time.Time
	var ext string
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		resp, err := client.Get(path)
		defer resp.Body.Close()
		if err != nil {
			return nil, err
		}

		LastModified := resp.Header.Get("Last-Modified")
		if len(LastModified) > 24 {
			modTime, err = time.Parse(time.RFC1123, LastModified)
			if err != nil {
				modTime = time.Now()
			}
		} else {
			modTime = time.Now()
		}

		content, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		u, err := url.Parse(path)
		if err != nil {
			err := errors.New("malformed http path")
			return nil, err
		}

		ext_i := strings.LastIndex(u.Path, ".")
		if ext_i <= 1 {
			err := errors.New("malformed http path, missing file extension")
			return nil, err
		}
		ext = u.Path[ext_i:]

	} else {
		err := errors.New("unsupported path")
		return nil, err
	}

	var targetGroups []*targetgroup.Group

	switch strings.ToLower(ext) {
	case ".json":
		if err := json.Unmarshal(content, &targetGroups); err != nil {
			return nil, err
		}
	case ".yml", ".yaml":
		if err := yaml.UnmarshalStrict(content, &targetGroups); err != nil {
			return nil, err
		}
	default:
		panic(errors.Errorf("discovery.http.readFile: unhandled extension %q", ext))
	}

	for i, tg := range targetGroups {
		if tg == nil {
			err := errors.New("nil target group item found")
			return nil, err
		}

		tg.Source = fileSource(path, i)
		if tg.Labels == nil {
			tg.Labels = model.LabelSet{}
		}
		tg.Labels[httpSDFilepathLabel] = model.LabelValue(path)
	}

	d.writeTimestamp(path, float64(modTime.Unix()))

	return targetGroups, nil
}

// fileSource returns a source ID for the i-th target group in the file.
func fileSource(filename string, i int) string {
	return fmt.Sprintf("%s:%d", filename, i)
}
