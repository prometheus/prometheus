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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"gopkg.in/fsnotify.v1"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
)

const fileSDFilepathLabel = model.MetaLabelPrefix + "filepath"

var (
	fileSDScanDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_file_scan_duration_seconds",
			Help: "The duration of the File-SD scan in seconds.",
		})
	fileSDReadErrorsCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_file_read_errors_total",
			Help: "The number of File-SD read errors.",
		})
)

func init() {
	prometheus.MustRegister(fileSDScanDuration)
	prometheus.MustRegister(fileSDReadErrorsCount)
}

// Discovery provides service discovery functionality based
// on files that contain target groups in JSON or YAML format. Refreshing
// happens using file watches and periodic refreshes.
type Discovery struct {
	paths    []string
	watcher  *fsnotify.Watcher
	interval time.Duration

	// lastRefresh stores which files were found during the last refresh
	// and how many target groups they contained.
	// This is used to detect deleted target groups.
	lastRefresh map[string]int
}

// NewDiscovery returns a new file discovery for the given paths.
func NewDiscovery(conf *config.FileSDConfig) *Discovery {
	return &Discovery{
		paths:    conf.Files,
		interval: time.Duration(conf.RefreshInterval),
	}
}

// listFiles returns a list of all files that match the configured patterns.
func (d *Discovery) listFiles() []string {
	var paths []string
	for _, p := range d.paths {
		files, err := filepath.Glob(p)
		if err != nil {
			log.Errorf("Error expanding glob %q: %s", p, err)
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
			log.Errorf("Error adding file watch for %q: %s", p, err)
		}
	}
}

// Run implements the TargetProvider interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer d.stop()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Errorf("Error creating file watcher: %s", err)
		return
	}
	d.watcher = watcher

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
				log.Errorf("Error on file watch: %s", err)
			}
		}
	}
}

// stop shuts down the file watcher.
func (d *Discovery) stop() {
	log.Debugf("Stopping file discovery for %s...", d.paths)

	done := make(chan struct{})
	defer close(done)

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
		log.Errorf("Error closing file watcher for %s: %s", d.paths, err)
	}

	log.Debugf("File discovery for %s stopped.", d.paths)
}

// refresh reads all files matching the discovery's patterns and sends the respective
// updated target groups through the channel.
func (d *Discovery) refresh(ctx context.Context, ch chan<- []*config.TargetGroup) {
	t0 := time.Now()
	defer func() {
		fileSDScanDuration.Observe(time.Since(t0).Seconds())
	}()

	ref := map[string]int{}
	for _, p := range d.listFiles() {
		tgroups, err := readFile(p)
		if err != nil {
			fileSDReadErrorsCount.Inc()
			log.Errorf("Error reading file %q: %s", p, err)
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
			for i := m; i < n; i++ {
				select {
				case ch <- []*config.TargetGroup{{Source: fileSource(f, i)}}:
				case <-ctx.Done():
					return
				}
			}
		}
	}
	d.lastRefresh = ref

	d.watchFiles()
}

// fileSource returns a source ID for the i-th target group in the file.
func fileSource(filename string, i int) string {
	return fmt.Sprintf("%s:%d", filename, i)
}

// readFile reads a JSON or YAML list of targets groups from the file, depending on its
// file extension. It returns full configuration target groups.
func readFile(filename string) ([]*config.TargetGroup, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var targetGroups []*config.TargetGroup

	switch ext := filepath.Ext(filename); strings.ToLower(ext) {
	case ".json":
		if err := json.Unmarshal(content, &targetGroups); err != nil {
			return nil, err
		}
	case ".yml", ".yaml":
		if err := yaml.Unmarshal(content, &targetGroups); err != nil {
			return nil, err
		}
	default:
		panic(fmt.Errorf("retrieval.FileDiscovery.readFile: unhandled file extension %q", ext))
	}

	for i, tg := range targetGroups {
		tg.Source = fileSource(filename, i)
		if tg.Labels == nil {
			tg.Labels = model.LabelSet{}
		}
		tg.Labels[fileSDFilepathLabel] = model.LabelValue(filename)
	}
	return targetGroups, nil
}
