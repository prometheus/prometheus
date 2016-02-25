// Copyright 2016 The Prometheus Authors
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

package retrieval

import (
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/local"
)

const (
	scrapeHealthMetricName   = "up"
	scrapeDurationMetricName = "scrape_duration_seconds"

	// Capacity of the channel to buffer samples during ingestion.
	ingestedSamplesCap = 256

	// Constants for instrumentation.
	namespace = "prometheus"
	interval  = "interval"
)

var (
	errSkippedScrape = errors.New("scrape skipped due to throttled ingestion")

	targetIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "target_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{interval},
	)
	targetSkippedScrapes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "target_skipped_scrapes_total",
			Help:      "Total number of scrapes that were skipped because the metric storage was throttled.",
		},
		[]string{interval},
	)
)

func init() {
	prometheus.MustRegister(targetIntervalLength)
	prometheus.MustRegister(targetSkippedScrapes)
}

// scrapePool manages scrapes for sets of targets.
type scrapePool struct {
	appender storage.SampleAppender

	ctx     context.Context
	mtx     sync.RWMutex
	tgroups map[string]map[model.Fingerprint]*Target

	targets map[model.Fingerprint]loop
}

func newScrapePool(app storage.SampleAppender) *scrapePool {
	return &scrapePool{
		appender: app,
		tgroups:  map[string]map[model.Fingerprint]*Target{},
	}
}

func (sp *scrapePool) stop() {
	var wg sync.WaitGroup

	sp.mtx.RLock()

	for _, tgroup := range sp.tgroups {
		for _, t := range tgroup {
			wg.Add(1)

			go func(t *Target) {
				t.scrapeLoop.stop()
				wg.Done()
			}(t)
		}
	}
	sp.mtx.RUnlock()

	wg.Wait()
}

func (sp *scrapePool) sync(tgroups map[string]map[model.Fingerprint]*Target) {
	sp.mtx.Lock()

	var (
		wg         sync.WaitGroup
		newTgroups = map[string]map[model.Fingerprint]*Target{}
	)

	for source, targets := range tgroups {
		var (
			prevTargets = sp.tgroups[source]
			newTargets  = map[model.Fingerprint]*Target{}
		)
		newTgroups[source] = newTargets

		for fp, tnew := range targets {
			// If the same target existed before, we let it run and replace
			// the new one with it.
			if told, ok := prevTargets[fp]; ok {
				newTargets[fp] = told
			} else {
				newTargets[fp] = tnew

				tnew.scrapeLoop = newScrapeLoop(sp.ctx, tnew, tnew.wrapAppender(sp.appender), tnew.wrapReportingAppender(sp.appender))
				go tnew.scrapeLoop.run(tnew.interval(), tnew.timeout(), nil)
			}
		}
		for fp, told := range prevTargets {
			// A previous target is no longer in the group.
			if _, ok := targets[fp]; !ok {
				wg.Add(1)

				go func(told *Target) {
					told.scrapeLoop.stop()
					wg.Done()
				}(told)
			}
		}
	}

	// Stop scrapers for target groups that disappeared completely.
	for source, targets := range sp.tgroups {
		if _, ok := tgroups[source]; ok {
			continue
		}
		for _, told := range targets {
			wg.Add(1)

			go func(told *Target) {
				told.scrapeLoop.stop()
				wg.Done()
			}(told)
		}
	}

	sp.tgroups = newTgroups

	// Wait for all potentially stopped scrapers to terminate.
	// This covers the case of flapping targets. If the server is under high load, a new scraper
	// may be active and tries to insert. The old scraper that didn't terminate yet could still
	// be inserting a previous sample set.
	wg.Wait()

	// TODO(fabxc): maybe this can be released earlier with subsequent refactoring.
	sp.mtx.Unlock()
}

// A scraper retrieves samples and accepts a status report at the end.
type scraper interface {
	scrape(ctx context.Context, ts time.Time) (model.Samples, error)
	report(start time.Time, dur time.Duration, err error)
	offset(interval time.Duration) time.Duration
}

type loop interface {
	run(interval, timeout time.Duration, errc chan<- error)
	stop()
}

type scrapeLoop struct {
	scraper scraper

	appender       storage.SampleAppender
	reportAppender storage.SampleAppender

	done   chan struct{}
	mtx    sync.RWMutex
	ctx    context.Context
	cancel func()
}

func newScrapeLoop(ctx context.Context, sc scraper, app, reportApp storage.SampleAppender) *scrapeLoop {
	sl := &scrapeLoop{
		scraper:        sc,
		appender:       app,
		reportAppender: reportApp,
		done:           make(chan struct{}),
	}
	sl.ctx, sl.cancel = context.WithCancel(ctx)

	return sl
}

func (sl *scrapeLoop) run(interval, timeout time.Duration, errc chan<- error) {
	defer close(sl.done)

	select {
	case <-time.After(sl.scraper.offset(interval)):
		// Continue after a scraping offset.
	case <-sl.ctx.Done():
		return
	}

	var last time.Time

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-sl.ctx.Done():
			return
		default:
		}

		if !sl.appender.NeedsThrottling() {
			var (
				start        = time.Now()
				scrapeCtx, _ = context.WithTimeout(sl.ctx, timeout)
			)

			// Only record after the first scrape.
			if !last.IsZero() {
				targetIntervalLength.WithLabelValues(interval.String()).Observe(
					float64(time.Since(last)) / float64(time.Second), // Sub-second precision.
				)
			}

			samples, err := sl.scraper.scrape(scrapeCtx, start)
			if err == nil {
				sl.append(samples)
			} else if errc != nil {
				errc <- err
			}

			sl.report(start, time.Since(start), err)
			last = start
		} else {
			targetSkippedScrapes.WithLabelValues(interval.String()).Inc()
		}

		select {
		case <-sl.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (sl *scrapeLoop) stop() {
	sl.mtx.RLock()
	sl.cancel()
	sl.mtx.RUnlock()

	<-sl.done
}

func (sl *scrapeLoop) append(samples model.Samples) {
	numOutOfOrder := 0

	for _, s := range samples {
		if err := sl.appender.Append(s); err != nil {
			if err == local.ErrOutOfOrderSample {
				numOutOfOrder++
			} else {
				log.Warnf("Error inserting sample: %s", err)
			}
		}
	}
	if numOutOfOrder > 0 {
		log.With("numDropped", numOutOfOrder).Warn("Error on ingesting out-of-order samples")
	}
}

func (sl *scrapeLoop) report(start time.Time, duration time.Duration, err error) {
	sl.scraper.report(start, duration, err)

	ts := model.TimeFromUnixNano(start.UnixNano())

	var health model.SampleValue
	if err == nil {
		health = 1
	}

	healthSample := &model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: scrapeHealthMetricName,
		},
		Timestamp: ts,
		Value:     health,
	}
	durationSample := &model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: scrapeDurationMetricName,
		},
		Timestamp: ts,
		Value:     model.SampleValue(float64(duration) / float64(time.Second)),
	}

	sl.reportAppender.Append(healthSample)
	sl.reportAppender.Append(durationSample)
}
