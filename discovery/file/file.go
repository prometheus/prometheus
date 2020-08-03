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

package file

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	fsnotify "gopkg.in/fsnotify/fsnotify.v1"
	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	patFileSDName = regexp.MustCompile(`^[^*]*(\*[^/]*)?\.(json|yml|yaml|JSON|YML|YAML)$`)

	// DefaultSDConfig is the default file SD configuration.
	DefaultSDConfig = SDConfig{
		RefreshInterval: model.Duration(5 * time.Minute),
	}
)

// SDConfig is the configuration for file based discovery.
type SDConfig struct {
	Files           []string       `yaml:"files"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if len(c.Files) == 0 {
		return errors.New("file service discovery config must contain at least one path name")
	}
	for _, name := range c.Files {
		if !patFileSDName.MatchString(name) {
			return errors.Errorf("path name %q is not valid for file discovery", name)
		}
	}
	return nil
}

const fileSDFilepathLabel = model.MetaLabelPrefix + "filepath"

// TimestampCollector is a Custom Collector for Timestamps of the files.
type TimestampCollector struct {
	Description *prometheus.Desc
	discoverers map[*Discovery]struct{}
	lock        sync.RWMutex
}

// Describe method sends the description to the channel.
func (t *TimestampCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- t.Description
}

// Collect creates constant metrics for each file with last modified time of the file.
func (t *TimestampCollector) Collect(ch chan<- prometheus.Metric) {
	// New map to dedup filenames.
	uniqueFiles := make(map[string]float64)
	t.lock.RLock()
	for fileSD := range t.discoverers {
		fileSD.lock.RLock()
		for filename, timestamp := range fileSD.timestamps {
			uniqueFiles[filename] = timestamp
		}
		fileSD.lock.RUnlock()
	}
	t.lock.RUnlock()
	for filename, timestamp := range uniqueFiles {
		ch <- prometheus.MustNewConstMetric(
			t.Description,
			prometheus.GaugeValue,
			timestamp,
			filename,
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
			"prometheus_sd_file_mtime_seconds",
			"Timestamp (mtime) of files read by FileSD. Timestamp is set at read time.",
			[]string{"filename"},
			nil,
		),
		discoverers: make(map[*Discovery]struct{}),
	}
}

var (
	fileSDScanDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name:       "prometheus_sd_file_scan_duration_seconds",
			Help:       "The duration of the File-SD scan in seconds.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		})
	fileSDReadErrorsCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_file_read_errors_total",
			Help: "The number of File-SD read errors.",
		})
	fileSDTimeStamp = NewTimestampCollector()
)

func init() {
	prometheus.MustRegister(fileSDScanDuration)
	prometheus.MustRegister(fileSDReadErrorsCount)
	prometheus.MustRegister(fileSDTimeStamp)
}

// Discovery provides service discovery functionality based
// on files that contain target groups in JSON or YAML format. Refreshing
// happens using file watches and periodic refreshes.
type Discovery struct {
	paths      []string
	watcher    *fsnotify.Watcher
	interval   time.Duration
	timestamps map[string]float64
	lock       sync.RWMutex

	// lastRefresh stores which files were found during the last refresh
	// and how many target groups they contained.
	// This is used to detect deleted target groups.
	lastRefresh map[string]int
	logger      log.Logger
}

// NewDiscovery returns a new file discovery for the given paths.
func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	disc := &Discovery{
		paths:      conf.Files,
		interval:   time.Duration(conf.RefreshInterval),
		timestamps: make(map[string]float64),
		logger:     logger,
	}
	fileSDTimeStamp.addDiscoverer(disc)
	return disc
}

// listFiles returns a list of all files that match the configured patterns.
func (d *Discovery) listFiles() []string {
	var paths []string
	for _, p := range d.paths {
		files, err := filepath.Glob(p)
		if err != nil {
			level.Error(d.logger).Log("msg", "Error expanding glob", "glob", p, "err", err)
			continue
		}
		paths = append(paths, files...)
	}
	return paths
}

// watchFiles sets watches on all full paths or directories that were configured for
// this file discovery.
func (d *Discovery) watchFiles() {
	if d.watcher == nil {
		panic("no watcher configured")
	}
	for _, p := range d.paths {
		if idx := strings.LastIndex(p, "/"); idx > -1 {
			p = p[:idx]
		} else {
			p = "./"
		}
		if err := d.watcher.Add(p); err != nil {
			level.Error(d.logger).Log("msg", "Error adding file watch", "path", p, "err", err)
		}
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		level.Error(d.logger).Log("msg", "Error adding file watcher", "err", err)
		return
	}
	d.watcher = watcher
	defer d.stop()

	d.refresh(ctx, ch)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case event := <-d.watcher.Events:
			// fsnotify sometimes sends a bunch of events without name or operation.
			// It's unclear what they are and why they are sent - filter them out.
			if len(event.Name) == 0 {
				break
			}
			// Everything but a chmod requires rereading.
			if event.Op^fsnotify.Chmod == 0 {
				break
			}
			// Changes to a file can spawn various sequences of events with
			// different combinations of operations. For all practical purposes
			// this is inaccurate.
			// The most reliable solution is to reload everything if anything happens.
			d.refresh(ctx, ch)

		case <-ticker.C:
			// Setting a new watch after an update might fail. Make sure we don't lose
			// those files forever.
			d.refresh(ctx, ch)

		case err := <-d.watcher.Errors:
			if err != nil {
				level.Error(d.logger).Log("msg", "Error watching file", "err", err)
			}
		}
	}
}

func (d *Discovery) writeTimestamp(filename string, timestamp float64) {
	d.lock.Lock()
	d.timestamps[filename] = timestamp
	d.lock.Unlock()
}

func (d *Discovery) deleteTimestamp(filename string) {
	d.lock.Lock()
	delete(d.timestamps, filename)
	d.lock.Unlock()
}

// stop shuts down the file watcher.
func (d *Discovery) stop() {
	level.Debug(d.logger).Log("msg", "Stopping file discovery...", "paths", fmt.Sprintf("%v", d.paths))

	done := make(chan struct{})
	defer close(done)

	fileSDTimeStamp.removeDiscoverer(d)

	// Closing the watcher will deadlock unless all events and errors are drained.
	go func() {
		for {
			select {
			case <-d.watcher.Errors:
			case <-d.watcher.Events:
				// Drain all events and errors.
			case <-done:
				return
			}
		}
	}()
	if err := d.watcher.Close(); err != nil {
		level.Error(d.logger).Log("msg", "Error closing file watcher", "paths", fmt.Sprintf("%v", d.paths), "err", err)
	}

	level.Debug(d.logger).Log("msg", "File discovery stopped")
}

// refresh reads all files matching the discovery's patterns and sends the respective
// updated target groups through the channel.
func (d *Discovery) refresh(ctx context.Context, ch chan<- []*targetgroup.Group) {
	t0 := time.Now()
	defer func() {
		fileSDScanDuration.Observe(time.Since(t0).Seconds())
	}()
	ref := map[string]int{}
	for _, p := range d.listFiles() {
		tgroups, err := d.readFile(p)
		if err != nil {
			fileSDReadErrorsCount.Inc()

			level.Error(d.logger).Log("msg", "Error reading file", "path", p, "err", err)
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
			level.Debug(d.logger).Log("msg", "file_sd refresh found file that should be removed", "file", f)
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

	d.watchFiles()
}

// readFile reads a JSON or YAML list of targets groups from the file, depending on its
// file extension. It returns full configuration target groups.
func (d *Discovery) readFile(filename string) ([]*targetgroup.Group, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	content, err := ioutil.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	info, err := fd.Stat()
	if err != nil {
		return nil, err
	}

	var targetGroups []*targetgroup.Group

	switch ext := filepath.Ext(filename); strings.ToLower(ext) {
	case ".json":
		if err := json.Unmarshal(content, &targetGroups); err != nil {
			return nil, err
		}
	case ".yml", ".yaml":
		if err := yaml.UnmarshalStrict(content, &targetGroups); err != nil {
			return nil, err
		}
	default:
		panic(errors.Errorf("discovery.File.readFile: unhandled file extension %q", ext))
	}

	for i, tg := range targetGroups {
		if tg == nil {
			err = errors.New("nil target group item found")
			return nil, err
		}

		tg.Source = fileSource(filename, i)
		if tg.Labels == nil {
			tg.Labels = model.LabelSet{}
		}
		tg.Labels[fileSDFilepathLabel] = model.LabelValue(filename)
	}

	d.writeTimestamp(filename, float64(info.ModTime().Unix()))

	return targetGroups, nil
}

// fileSource returns a source ID for the i-th target group in the file.
func fileSource(filename string, i int) string {
	return fmt.Sprintf("%s:%d", filename, i)
}
