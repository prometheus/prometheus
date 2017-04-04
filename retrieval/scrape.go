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
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"

	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/httputil"
)

const (
	scrapeHealthMetricName       = "up"
	scrapeDurationMetricName     = "scrape_duration_seconds"
	scrapeSamplesMetricName      = "scrape_samples_scraped"
	samplesPostRelabelMetricName = "scrape_samples_post_metric_relabeling"
)

var (
	targetIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"interval"},
	)
	targetSkippedScrapes = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_skipped_scrapes_total",
			Help: "Total number of scrapes that were skipped because the metric storage was throttled.",
		},
	)
	targetReloadIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_reload_length_seconds",
			Help:       "Actual interval to reload the scrape pool with a given configuration.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"interval"},
	)
	targetSyncIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_target_sync_length_seconds",
			Help:       "Actual interval to sync the scrape pool.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{"scrape_job"},
	)
	targetScrapePoolSyncsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrape_pool_sync_total",
			Help: "Total number of syncs that were executed on a scrape pool.",
		},
		[]string{"scrape_job"},
	)
	targetScrapeSampleLimit = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_exceeded_sample_limit_total",
			Help: "Total number of scrapes that hit the sample limit and were rejected.",
		},
	)
)

func init() {
	prometheus.MustRegister(targetIntervalLength)
	prometheus.MustRegister(targetSkippedScrapes)
	prometheus.MustRegister(targetReloadIntervalLength)
	prometheus.MustRegister(targetSyncIntervalLength)
	prometheus.MustRegister(targetScrapePoolSyncsCounter)
	prometheus.MustRegister(targetScrapeSampleLimit)
}

// scrapePool manages scrapes for sets of targets.
type scrapePool struct {
	appendable Appendable

	ctx context.Context

	mtx    sync.RWMutex
	config *config.ScrapeConfig
	client *http.Client
	// Targets and loops must always be synchronized to have the same
	// set of hashes.
	targets map[uint64]*Target
	loops   map[uint64]loop

	// Constructor for new scrape loops. This is settable for testing convenience.
	newLoop func(context.Context, scraper, func() storage.Appender, func() storage.Appender) loop
}

func newScrapePool(ctx context.Context, cfg *config.ScrapeConfig, app Appendable) *scrapePool {
	client, err := NewHTTPClient(cfg.HTTPClientConfig)
	if err != nil {
		// Any errors that could occur here should be caught during config validation.
		log.Errorf("Error creating HTTP client for job %q: %s", cfg.JobName, err)
	}
	return &scrapePool{
		appendable: app,
		config:     cfg,
		ctx:        ctx,
		client:     client,
		targets:    map[uint64]*Target{},
		loops:      map[uint64]loop{},
		newLoop:    newScrapeLoop,
	}
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

	client, err := httputil.NewClientFromConfig(cfg.HTTPClientConfig)
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
			newLoop = sp.newLoop(sp.ctx, s,
				func() storage.Appender {
					return sp.sampleAppender(t)
				},
				func() storage.Appender {
					return sp.reportAppender(t)
				},
			)
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

// Sync converts target groups into actual scrape targets and synchronizes
// the currently running scraper with the resulting set.
func (sp *scrapePool) Sync(tgs []*config.TargetGroup) {
	start := time.Now()

	var all []*Target
	for _, tg := range tgs {
		targets, err := targetsFromGroup(tg, sp.config)
		if err != nil {
			log.With("err", err).Error("creating targets failed")
			continue
		}
		all = append(all, targets...)
	}
	sp.sync(all)

	targetSyncIntervalLength.WithLabelValues(sp.config.JobName).Observe(
		time.Since(start).Seconds(),
	)
	targetScrapePoolSyncsCounter.WithLabelValues(sp.config.JobName).Inc()
}

// sync takes a list of potentially duplicated targets, deduplicates them, starts
// scrape loops for new targets, and stops scrape loops for disappeared targets.
// It returns after all stopped scrape loops terminated.
func (sp *scrapePool) sync(targets []*Target) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	var (
		uniqueTargets = map[uint64]struct{}{}
		interval      = time.Duration(sp.config.ScrapeInterval)
		timeout       = time.Duration(sp.config.ScrapeTimeout)
	)

	for _, t := range targets {
		t := t
		hash := t.hash()
		uniqueTargets[hash] = struct{}{}

		if _, ok := sp.targets[hash]; !ok {
			s := &targetScraper{Target: t, client: sp.client}
			l := sp.newLoop(sp.ctx, s,
				func() storage.Appender {
					return sp.sampleAppender(t)
				},
				func() storage.Appender {
					return sp.reportAppender(t)
				},
			)

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
}

// sampleAppender returns an appender for ingested samples from the target.
func (sp *scrapePool) sampleAppender(target *Target) storage.Appender {
	app, err := sp.appendable.Appender()
	if err != nil {
		panic(err)
	}

	// The limit is applied after metrics are potentially dropped via relabeling.
	if sp.config.SampleLimit > 0 {
		app = &limitAppender{
			Appender: app,
			limit:    int(sp.config.SampleLimit),
		}
	}

	// The relabelAppender has to be inside the label-modifying appenders
	// so the relabeling rules are applied to the correct label set.
	if mrc := sp.config.MetricRelabelConfigs; len(mrc) > 0 {
		app = relabelAppender{
			Appender:    app,
			relabelings: mrc,
		}
	}

	if sp.config.HonorLabels {
		app = honorLabelsAppender{
			Appender: app,
			labels:   target.Labels(),
		}
	} else {
		app = ruleLabelsAppender{
			Appender: app,
			labels:   target.Labels(),
		}
	}
	return app
}

// reportAppender returns an appender for reporting samples for the target.
func (sp *scrapePool) reportAppender(target *Target) storage.Appender {
	app, err := sp.appendable.Appender()
	if err != nil {
		panic(err)
	}
	return ruleLabelsAppender{
		Appender: app,
		labels:   target.Labels(),
	}
}

// A scraper retrieves samples and accepts a status report at the end.
type scraper interface {
	scrape(ctx context.Context, w io.Writer) error
	report(start time.Time, dur time.Duration, err error)
	offset(interval time.Duration) time.Duration
}

// targetScraper implements the scraper interface for a target.
type targetScraper struct {
	*Target

	client *http.Client
	req    *http.Request

	gzipr *gzip.Reader
	buf   *bufio.Reader
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,*/*;q=0.1`

var userAgentHeader = fmt.Sprintf("Prometheus/%s", version.Version)

func (s *targetScraper) scrape(ctx context.Context, w io.Writer) error {
	if s.req == nil {
		req, err := http.NewRequest("GET", s.URL().String(), nil)
		if err != nil {
			return err
		}
		// Disable accept header to always negotiate for text format.
		// req.Header.Add("Accept", acceptHeader)
		req.Header.Add("Accept-Encoding", "gzip")
		req.Header.Set("User-Agent", userAgentHeader)

		s.req = req
	}

	resp, err := ctxhttp.Do(ctx, s.client, s.req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	if resp.Header.Get("Content-Encoding") != "gzip" {
		_, err = io.Copy(w, resp.Body)
		return err
	}

	if s.gzipr == nil {
		s.buf = bufio.NewReader(resp.Body)
		s.gzipr, err = gzip.NewReader(s.buf)
		if err != nil {
			return err
		}
	} else {
		s.buf.Reset(resp.Body)
		s.gzipr.Reset(s.buf)
	}

	_, err = io.Copy(w, s.gzipr)
	s.gzipr.Close()
	return err
}

// A loop can run and be stopped again. It must not be reused after it was stopped.
type loop interface {
	run(interval, timeout time.Duration, errc chan<- error)
	stop()
}

type scrapeLoop struct {
	scraper scraper

	appender       func() storage.Appender
	reportAppender func() storage.Appender

	cache map[string]uint64

	done   chan struct{}
	ctx    context.Context
	cancel func()
}

func newScrapeLoop(ctx context.Context, sc scraper, app, reportApp func() storage.Appender) loop {
	sl := &scrapeLoop{
		scraper:        sc,
		appender:       app,
		reportAppender: reportApp,
		cache:          map[string]uint64{},
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

	buf := bytes.NewBuffer(make([]byte, 0, 16000))

	for {
		buf.Reset()
		select {
		case <-sl.ctx.Done():
			return
		default:
		}

		var (
			total, added int
			start        = time.Now()
			scrapeCtx, _ = context.WithTimeout(sl.ctx, timeout)
		)

		// Only record after the first scrape.
		if !last.IsZero() {
			targetIntervalLength.WithLabelValues(interval.String()).Observe(
				time.Since(last).Seconds(),
			)
		}

		err := sl.scraper.scrape(scrapeCtx, buf)
		if err == nil {
			b := buf.Bytes()

			if total, added, err = sl.append(b, start); err != nil {
				log.With("err", err).Error("append failed")
			}
		} else if errc != nil {
			errc <- err
		}

		sl.report(start, time.Since(start), total, added, err)
		last = start

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

type sample struct {
	metric labels.Labels
	t      int64
	v      float64
}

type samples []sample

func (s samples) Len() int      { return len(s) }
func (s samples) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func (s samples) Less(i, j int) bool {
	d := labels.Compare(s[i].metric, s[j].metric)
	if d < 0 {
		return true
	} else if d > 0 {
		return false
	}
	return s[i].t < s[j].t
}

func (sl *scrapeLoop) append(b []byte, ts time.Time) (total, added int, err error) {
	var (
		app     = sl.appender()
		p       = textparse.New(b)
		defTime = timestamp.FromTime(ts)
	)

loop:
	for p.Next() {
		total++

		t := defTime
		met, tp, v := p.At()
		if tp != nil {
			t = *tp
		}

		mets := yoloString(met)
		ref, ok := sl.cache[mets]
		if ok {
			switch err = app.AddFast(ref, t, v); err {
			case nil:
			case storage.ErrNotFound:
				ok = false
			case errSeriesDropped:
				continue
			default:
				break loop
			}
		}
		if !ok {
			var lset labels.Labels
			p.Metric(&lset)

			ref, err = app.Add(lset, t, v)
			// TODO(fabxc): also add a dropped-cache?
			switch err {
			case nil:
			case errSeriesDropped:
				continue
			default:
				break loop
			}
			// Allocate a real string.
			mets = string(met)
			sl.cache[mets] = ref
		}
		added++
	}
	if err == nil {
		err = p.Err()
	}
	if err != nil {
		app.Rollback()
		return total, 0, err
	}
	if err := app.Commit(); err != nil {
		return total, 0, err
	}
	return total, added, nil
}

func yoloString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}

func (sl *scrapeLoop) report(start time.Time, duration time.Duration, scraped, appended int, err error) error {
	sl.scraper.report(start, duration, err)

	ts := timestamp.FromTime(start)

	var health float64
	if err == nil {
		health = 1
	}

	app := sl.reportAppender()

	if err := sl.addReportSample(app, scrapeHealthMetricName, ts, health); err != nil {
		app.Rollback()
		return err
	}
	if err := sl.addReportSample(app, scrapeDurationMetricName, ts, duration.Seconds()); err != nil {
		app.Rollback()
		return err
	}
	if err := sl.addReportSample(app, scrapeSamplesMetricName, ts, float64(scraped)); err != nil {
		app.Rollback()
		return err
	}
	if err := sl.addReportSample(app, samplesPostRelabelMetricName, ts, float64(appended)); err != nil {
		app.Rollback()
		return err
	}
	return app.Commit()
}

func (sl *scrapeLoop) addReportSample(app storage.Appender, s string, t int64, v float64) error {
	ref, ok := sl.cache[s]

	if ok {
		if err := app.AddFast(ref, t, v); err == nil {
			return nil
		} else if err != storage.ErrNotFound {
			return err
		}
	}
	met := labels.Labels{
		labels.Label{Name: labels.MetricName, Value: s},
	}
	ref, err := app.Add(met, t, v)
	if err != nil {
		return err
	}
	sl.cache[s] = ref

	return nil
}
