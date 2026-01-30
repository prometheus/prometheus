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
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/rules"
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

	rulesManagerMtx sync.RWMutex
	rulesManager    *rules.Manager

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

// SetRulesManager sets the rules manager. The rules manager is created later during initialization
// (after the MetadataWatcher is created), so this method must be called to provide it once it's available.
// It can be called at any time to update the rules manager.
func (mw *MetadataWatcher) SetRulesManager(rm *rules.Manager) {
	mw.rulesManagerMtx.Lock()
	defer mw.rulesManagerMtx.Unlock()
	mw.rulesManager = rm
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

	// Collect metadata from scrape targets.
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

	// Collect metadata from recording rules.
	mw.rulesManagerMtx.RLock()
	rm := mw.rulesManager
	mw.rulesManagerMtx.RUnlock()
	if rm != nil {
		for metricName, ruleMeta := range rm.RecordingRulesMetadata() {
			scrapeMeta := scrape.MetricMetadata{
				MetricFamily: metricName,
				Type:         ruleMeta.Type,
				Help:         ruleMeta.Help,
				Unit:         ruleMeta.Unit,
			}
			if _, ok := metadataSet[scrapeMeta]; !ok {
				metadata = append(metadata, scrapeMeta)
				metadataSet[scrapeMeta] = struct{}{}
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
