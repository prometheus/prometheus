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

package remote

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb/seriesmetadata"
)

// MetadataAppender is an interface used by the Metadata Watcher to send metadata, It is read from the scrape manager, on to somewhere else.
type MetadataAppender interface {
	AppendWatcherMetadata(context.Context, []scrape.MetricMetadata)
}

// Watchable represents from where we fetch active targets for metadata.
type Watchable interface {
	TargetsActive() map[string][]*scrape.Target
}

// MetadataReader provides read access to TSDB series metadata.
// Used to avoid importing the tsdb package directly.
type MetadataReader interface {
	SeriesMetadata() (seriesmetadata.Reader, error)
}

type noopScrapeManager struct{}

func (*noopScrapeManager) Get() (*scrape.Manager, error) {
	return nil, errors.New("scrape manager not ready")
}

// MetadataWatcher watches the Scrape Manager for a given WriteMetadataTo.
type MetadataWatcher struct {
	name   string
	logger *slog.Logger

	managerGetter  ReadyScrapeManager
	manager        Watchable
	metadataReader MetadataReader
	writer         MetadataAppender

	interval model.Duration
	deadline time.Duration

	done chan struct{}

	softShutdownCtx    context.Context
	softShutdownCancel context.CancelFunc
	hardShutdownCancel context.CancelFunc
	hardShutdownCtx    context.Context
}

// NewMetadataWatcher builds a new MetadataWatcher.
// If metadataReader is non-nil, metadata will be read from TSDB instead of scrape targets.
func NewMetadataWatcher(l *slog.Logger, mg ReadyScrapeManager, name string, w MetadataAppender, interval model.Duration, deadline time.Duration, metadataReader MetadataReader) *MetadataWatcher {
	if l == nil {
		l = promslog.NewNopLogger()
	}

	if mg == nil {
		mg = &noopScrapeManager{}
	}

	return &MetadataWatcher{
		name:   name,
		logger: l,

		managerGetter:  mg,
		metadataReader: metadataReader,
		writer:         w,

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
	// When a TSDB metadata reader is available, read from TSDB directly.
	if mw.metadataReader != nil {
		mw.collectFromTSDB()
		return
	}

	if !mw.ready() {
		return
	}

	// We create a set of the metadata to help deduplicating based on the attributes of a
	// scrape.MetricMetadata. In this case, a combination of metric name, help, type, and unit.
	metadataSet := map[scrape.MetricMetadata]struct{}{}
	md := []scrape.MetricMetadata{}
	for _, tset := range mw.manager.TargetsActive() {
		for _, target := range tset {
			for _, entry := range target.ListMetadata() {
				if _, ok := metadataSet[entry]; !ok {
					md = append(md, entry)
					metadataSet[entry] = struct{}{}
				}
			}
		}
	}

	// Blocks until the metadata is sent to the remote write endpoint or hardShutdownContext is expired.
	mw.writer.AppendWatcherMetadata(mw.hardShutdownCtx, md)
}

func (mw *MetadataWatcher) collectFromTSDB() {
	mr, err := mw.metadataReader.SeriesMetadata()
	if err != nil {
		mw.logger.Error("error reading series metadata from TSDB", "err", err)
		return
	}
	defer mr.Close()

	metadataSet := map[scrape.MetricMetadata]struct{}{}
	var md []scrape.MetricMetadata
	err = mr.IterByMetricName(func(name string, metas []metadata.Metadata) error {
		for _, m := range metas {
			entry := scrape.MetricMetadata{
				MetricFamily: name,
				Type:         m.Type,
				Help:         m.Help,
				Unit:         m.Unit,
			}
			if _, ok := metadataSet[entry]; !ok {
				md = append(md, entry)
				metadataSet[entry] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		mw.logger.Error("error iterating series metadata from TSDB", "err", err)
		return
	}

	mw.writer.AppendWatcherMetadata(mw.hardShutdownCtx, md)
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
