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
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/local"
)

const (
	scrapeHealthMetricName   = "up"
	scrapeDurationMetricName = "scrape_duration_seconds"

	// Constants for instrumentation.
	namespace = "prometheus"
	interval  = "interval"
	scrapeJob = "scrape_job"
)

var (
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
	targetReloadIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "target_reload_length_seconds",
			Help:       "Actual interval to reload the scrape pool with a given configuration.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{interval},
	)
	targetSyncIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "target_sync_length_seconds",
			Help:       "Actual interval to sync the scrape pool.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{scrapeJob},
	)
	targetScrapePoolSyncsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "target_scrape_pool_sync_total",
			Help:      "Total number of syncs that were executed on a scrape pool.",
		},
		[]string{scrapeJob},
	)
)

func init() {
	prometheus.MustRegister(targetIntervalLength)
	prometheus.MustRegister(targetSkippedScrapes)
	prometheus.MustRegister(targetReloadIntervalLength)
	prometheus.MustRegister(targetSyncIntervalLength)
	prometheus.MustRegister(targetScrapePoolSyncsCounter)
}

// scrapePool manages scrapes for sets of targets.
type scrapePool struct {
	appender storage.SampleAppender

	ctx context.Context

	mtx    sync.RWMutex
	config *config.ScrapeConfig
	client *http.Client
	// Targets and loops must always be synchronized to have the same
	// set of hashes.
	targets map[uint64]*Target
	loops   map[uint64]loop

	// Constructor for new scrape loops. This is settable for testing convenience.
	newLoop func(context.Context, scraper, storage.SampleAppender, storage.SampleAppender) loop
}

func newScrapePool(cfg *config.ScrapeConfig, app storage.SampleAppender) *scrapePool {
	client, err := newHTTPClient(cfg)
	if err != nil {
		// Any errors that could occur here should be caught during config validation.
		log.Errorf("Error creating HTTP client for job %q: %s", cfg.JobName, err)
	}
	return &scrapePool{
		appender: app,
		config:   cfg,
		client:   client,
		targets:  map[uint64]*Target{},
		loops:    map[uint64]loop{},
		newLoop:  newScrapeLoop,
	}
}

func (sp *scrapePool) init(ctx context.Context) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	sp.ctx = ctx
}

// stop terminates all scrape loops and returns after they all terminated.
func (sp *scrapePool) stop() {
	var wg sync.WaitGroup

	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	for fp, l := range sp.loops {
		wg.Add(1)

		go func(l loop) {
			l.stop()
			wg.Done()
		}(l)

		delete(sp.loops, fp)
		delete(sp.targets, fp)
	}

	wg.Wait()
}

// reload the scrape pool with the given scrape configuration. The target state is preserved
// but all scrape loops are restarted with the new scrape configuration.
// This method returns after all scrape loops that were stopped have fully terminated.
func (sp *scrapePool) reload(cfg *config.ScrapeConfig) {
	start := time.Now()
	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	client, err := newHTTPClient(cfg)
	if err != nil {
		// Any errors that could occur here should be caught during config validation.
		log.Errorf("Error creating HTTP client for job %q: %s", cfg.JobName, err)
	}
	sp.config = cfg
	sp.client = client

	var (
		wg       sync.WaitGroup
		interval = time.Duration(sp.config.ScrapeInterval)
		timeout  = time.Duration(sp.config.ScrapeTimeout)
	)

	for fp, oldLoop := range sp.loops {
		var (
			t       = sp.targets[fp]
			s       = &targetScraper{Target: t, client: sp.client}
			newLoop = sp.newLoop(sp.ctx, s, sp.sampleAppender(t), sp.reportAppender(t))
		)
		wg.Add(1)

		go func(oldLoop, newLoop loop) {
			oldLoop.stop()
			wg.Done()

			go newLoop.run(interval, timeout, nil)
		}(oldLoop, newLoop)

		sp.loops[fp] = newLoop
	}

	wg.Wait()
	targetReloadIntervalLength.WithLabelValues(interval.String()).Observe(
		time.Since(start).Seconds(),
	)
}

// sync takes a list of potentially duplicated targets, deduplicates them, starts
// scrape loops for new targets, and stops scrape loops for disappeared targets.
// It returns after all stopped scrape loops terminated.
func (sp *scrapePool) sync(targets []*Target) {
	start := time.Now()
	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	var (
		uniqueTargets = map[uint64]struct{}{}
		interval      = time.Duration(sp.config.ScrapeInterval)
		timeout       = time.Duration(sp.config.ScrapeTimeout)
	)

	for _, t := range targets {
		hash := t.hash()
		uniqueTargets[hash] = struct{}{}

		if _, ok := sp.targets[hash]; !ok {
			s := &targetScraper{Target: t, client: sp.client}
			l := sp.newLoop(sp.ctx, s, sp.sampleAppender(t), sp.reportAppender(t))

			sp.targets[hash] = t
			sp.loops[hash] = l

			go l.run(interval, timeout, nil)
		}
	}

	var wg sync.WaitGroup

	// Stop and remove old targets and scraper loops.
	for hash := range sp.targets {
		if _, ok := uniqueTargets[hash]; !ok {
			wg.Add(1)
			go func(l loop) {
				l.stop()
				wg.Done()
			}(sp.loops[hash])

			delete(sp.loops, hash)
			delete(sp.targets, hash)
		}
	}

	// Wait for all potentially stopped scrapers to terminate.
	// This covers the case of flapping targets. If the server is under high load, a new scraper
	// may be active and tries to insert. The old scraper that didn't terminate yet could still
	// be inserting a previous sample set.
	wg.Wait()
	targetSyncIntervalLength.WithLabelValues(sp.config.JobName).Observe(
		time.Since(start).Seconds(),
	)
	targetScrapePoolSyncsCounter.WithLabelValues(sp.config.JobName).Inc()
}

// sampleAppender returns an appender for ingested samples from the target.
func (sp *scrapePool) sampleAppender(target *Target) storage.SampleAppender {
	app := sp.appender
	// The relabelAppender has to be inside the label-modifying appenders
	// so the relabeling rules are applied to the correct label set.
	if mrc := sp.config.MetricRelabelConfigs; len(mrc) > 0 {
		app = relabelAppender{
			SampleAppender: app,
			relabelings:    mrc,
		}
	}

	if sp.config.HonorLabels {
		app = honorLabelsAppender{
			SampleAppender: app,
			labels:         target.Labels(),
		}
	} else {
		app = ruleLabelsAppender{
			SampleAppender: app,
			labels:         target.Labels(),
		}
	}
	return app
}

// reportAppender returns an appender for reporting samples for the target.
func (sp *scrapePool) reportAppender(target *Target) storage.SampleAppender {
	return ruleLabelsAppender{
		SampleAppender: sp.appender,
		labels:         target.Labels(),
	}
}

// A scraper retrieves samples and accepts a status report at the end.
type scraper interface {
	scrape(ctx context.Context, ts time.Time) (model.Samples, error)
	report(start time.Time, dur time.Duration, err error)
	offset(interval time.Duration) time.Duration
}

// targetScraper implements the scraper interface for a target.
type targetScraper struct {
	*Target
	client *http.Client
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema="prometheus/telemetry";version=0.0.2;q=0.2,*/*;q=0.1`

func (s *targetScraper) scrape(ctx context.Context, ts time.Time) (model.Samples, error) {
	req, err := http.NewRequest("GET", s.URL().String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", acceptHeader)

	resp, err := ctxhttp.Do(ctx, s.client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	var (
		allSamples = make(model.Samples, 0, 200)
		decSamples = make(model.Vector, 0, 50)
	)
	sdec := expfmt.SampleDecoder{
		Dec: expfmt.NewDecoder(resp.Body, expfmt.ResponseFormat(resp.Header)),
		Opts: &expfmt.DecodeOptions{
			Timestamp: model.TimeFromUnixNano(ts.UnixNano()),
		},
	}

	for {
		if err = sdec.Decode(&decSamples); err != nil {
			break
		}
		allSamples = append(allSamples, decSamples...)
		decSamples = decSamples[:0]
	}

	if err == io.EOF {
		// Set err to nil since it is used in the scrape health recording.
		err = nil
	}
	return allSamples, err
}

// A loop can run and be stopped again. It must not be reused after it was stopped.
type loop interface {
	run(interval, timeout time.Duration, errc chan<- error)
	stop()
}

type scrapeLoop struct {
	scraper scraper

	appender       storage.SampleAppender
	reportAppender storage.SampleAppender

	done   chan struct{}
	ctx    context.Context
	cancel func()
}

func newScrapeLoop(ctx context.Context, sc scraper, app, reportApp storage.SampleAppender) loop {
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
					time.Since(last).Seconds(),
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
	sl.cancel()
	<-sl.done
}

func (sl *scrapeLoop) append(samples model.Samples) {
	var (
		numOutOfOrder = 0
		numDuplicates = 0
	)

	for _, s := range samples {
		if err := sl.appender.Append(s); err != nil {
			switch err {
			case local.ErrOutOfOrderSample:
				numOutOfOrder++
				log.With("sample", s).With("error", err).Debug("Sample discarded")
			case local.ErrDuplicateSampleForTimestamp:
				numDuplicates++
				log.With("sample", s).With("error", err).Debug("Sample discarded")
			default:
				log.With("sample", s).With("error", err).Warn("Sample discarded")
			}
		}
	}
	if numOutOfOrder > 0 {
		log.With("numDropped", numOutOfOrder).Warn("Error on ingesting out-of-order samples")
	}
	if numDuplicates > 0 {
		log.With("numDropped", numDuplicates).Warn("Error on ingesting samples with different value but same timestamp")
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
		Value:     model.SampleValue(duration.Seconds()),
	}

	if err := sl.reportAppender.Append(healthSample); err != nil {
		log.With("sample", healthSample).With("error", err).Warn("Scrape health sample discarded")
	}
	if err := sl.reportAppender.Append(durationSample); err != nil {
		log.With("sample", durationSample).With("error", err).Warn("Scrape duration sample discarded")
	}
}
