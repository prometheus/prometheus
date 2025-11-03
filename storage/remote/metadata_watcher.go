// Copyright 2020 The Prometheus Authors
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

package remote

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/scrape"
)

// MetadataAppender is an interface used by the Metadata Watcher to send metadata, It is read from the scrape manager, on to somewhere else.
type MetadataAppender interface {
	AppendWatcherMetadata(context.Context, []scrape.MetricMetadata)
}

// Watchable represents from where we fetch active targets for metadata.
type Watchable interface {
	TargetsActive() map[string][]*scrape.Target
}

type noopScrapeManager struct{}

func (*noopScrapeManager) Get() (*scrape.Manager, error) {
	return nil, errors.New("scrape manager not ready")
}

// MetadataWatcher watches the Scrape Manager for a given WriteMetadataTo.
type MetadataWatcher struct {
	name   string
	logger *slog.Logger

	managerGetter ReadyScrapeManager
	manager       Watchable
	writer        MetadataAppender

	interval model.Duration
	deadline time.Duration

	done chan struct{}

	softShutdownCtx    context.Context
	softShutdownCancel context.CancelFunc
	hardShutdownCancel context.CancelFunc
	hardShutdownCtx    context.Context
}

// NewMetadataWatcher builds a new MetadataWatcher.
func NewMetadataWatcher(l *slog.Logger, mg ReadyScrapeManager, name string, w MetadataAppender, interval model.Duration, deadline time.Duration) *MetadataWatcher {
	if l == nil {
		l = promslog.NewNopLogger()
	}

	if mg == nil {
		mg = &noopScrapeManager{}
	}

	return &MetadataWatcher{
		name:   name,
		logger: l,

		managerGetter: mg,
		writer:        w,

		interval: interval,
		deadline: deadline,

		done: make(chan struct{}),
	}
}

// Start the MetadataWatcher.
func (mw *MetadataWatcher) Start() {
	mw.logger.Info("Starting scraped metadata watcher")
	mw.hardShutdownCtx, mw.hardShutdownCancel = context.WithCancel(context.Background())
	mw.softShutdownCtx, mw.softShutdownCancel = context.WithCancel(mw.hardShutdownCtx)
	go mw.loop()
}

// Stop the MetadataWatcher.
func (mw *MetadataWatcher) Stop() {
	mw.logger.Info("Stopping metadata watcher...")
	defer mw.logger.Info("Scraped metadata watcher stopped")

	mw.softShutdownCancel()
	select {
	case <-mw.done:
		return
	case <-time.After(mw.deadline):
		mw.logger.Error("Failed to flush metadata")
	}

	mw.hardShutdownCancel()
	<-mw.done
}

func (mw *MetadataWatcher) loop() {
	ticker := time.NewTicker(time.Duration(mw.interval))
	defer ticker.Stop()
	defer close(mw.done)

	for {
		select {
		case <-mw.softShutdownCtx.Done():
			return
		case <-ticker.C:
			mw.collect()
		}
	}
}

func (mw *MetadataWatcher) collect() {
	if !mw.ready() {
		return
	}

	// We create a set of the metadata to help deduplicating based on the attributes of a
	// scrape.MetricMetadata. In this case, a combination of metric name, help, type, and unit.
	metadataSet := map[scrape.MetricMetadata]struct{}{}
	metadata := []scrape.MetricMetadata{}
	for _, tset := range mw.manager.TargetsActive() {
		for _, target := range tset {
			for _, entry := range target.ListMetadata() {
				if _, ok := metadataSet[entry]; !ok {
					metadata = append(metadata, entry)
					metadataSet[entry] = struct{}{}
				}
			}
		}
	}

	// Blocks until the metadata is sent to the remote write endpoint or hardShutdownContext is expired.
	mw.writer.AppendWatcherMetadata(mw.hardShutdownCtx, metadata)
}

func (mw *MetadataWatcher) ready() bool {
	if mw.manager != nil {
		return true
	}

	m, err := mw.managerGetter.Get()
	if err != nil {
		return false
	}

	mw.manager = m
	return true
}

// GetMetadataForMetric retrieves metadata for a specific metric family from the scrape cache.
// This is used by Remote Write v2 to get metadata (especially Help strings) when not available
// from WAL records. Returns nil if scrape manager is not ready or metadata is not found.
func (mw *MetadataWatcher) GetMetadataForMetric(metricFamily string) *scrape.MetricMetadata {
	if !mw.ready() {
		return nil
	}

	// First try: exact match.
	if meta := mw.lookupMetadata(metricFamily); meta != nil {
		return meta
	}

	// Second try: strip suffix after last underscore and try again.
	// This handles histogram suffixes like _bucket, _count, _sum and counter _total.
	if idx := strings.LastIndexByte(metricFamily, '_'); idx > 0 {
		baseName := metricFamily[:idx]
		if meta := mw.lookupMetadata(baseName); meta != nil {
			return meta
		}
	}

	return nil
}

// lookupMetadata performs the actual metadata lookup from active targets.
func (mw *MetadataWatcher) lookupMetadata(metricFamily string) *scrape.MetricMetadata {
	for _, tset := range mw.manager.TargetsActive() {
		for _, target := range tset {
			if meta, ok := target.GetMetadata(metricFamily); ok {
				return &scrape.MetricMetadata{
					MetricFamily: meta.MetricFamily,
					Type:         meta.Type,
					Help:         meta.Help,
					Unit:         meta.Unit,
				}
			}
		}
	}
	return nil
}
