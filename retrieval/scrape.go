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
	"math"
	"net/http"
	"sync"
	"time"
	"unsafe"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"

	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/pool"
	"github.com/prometheus/prometheus/pkg/relabel"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/pkg/value"
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
	targetScrapeSampleDuplicate = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_duplicate_timestamp_total",
			Help: "Total number of samples rejected due to duplicate timestamps but different values",
		},
	)
	targetScrapeSampleOutOfOrder = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_order_total",
			Help: "Total number of samples rejected due to not being out of the expected order",
		},
	)
	targetScrapeSampleOutOfBounds = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_target_scrapes_sample_out_of_bounds_total",
			Help: "Total number of samples rejected due to timestamp falling outside of the time bounds",
		},
	)
)

func init() {
	prometheus.MustRegister(targetIntervalLength)
	prometheus.MustRegister(targetReloadIntervalLength)
	prometheus.MustRegister(targetSyncIntervalLength)
	prometheus.MustRegister(targetScrapePoolSyncsCounter)
	prometheus.MustRegister(targetScrapeSampleLimit)
	prometheus.MustRegister(targetScrapeSampleDuplicate)
	prometheus.MustRegister(targetScrapeSampleOutOfOrder)
	prometheus.MustRegister(targetScrapeSampleOutOfBounds)
}

// scrapePool manages scrapes for sets of targets.
type scrapePool struct {
	appendable Appendable

	ctx context.Context

	mtx    sync.RWMutex
	config *config.ScrapeConfig
	client *http.Client

	targets map[uint64]*targetEntry

	// Constructor for new scrape loops. This is settable for testing convenience.
	newLoop func(*Target, scraper) loop

	logger log.Logger
}

type labelsMutator func(labels.Labels) labels.Labels

func newScrapePool(ctx context.Context, cfg *config.ScrapeConfig, app Appendable, logger log.Logger) *scrapePool {
	sp := &scrapePool{
		appendable: app,
		targets:    map[uint64]*targetEntry{},
		logger:     logger,
	}
	sp.updateConfig(cfg)

	return sp
}

type loop interface {
	// run starts the loop and runs until the context gets canceled.
	run(context.Context)
	// setInterval adjusts the iteration interval on the fly.
	setInterval(time.Duration)
	// wait blocks until the loop has terminated.
	wait()
}

func (sp *scrapePool) wait() {
	<-sp.ctx.Done()

	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	for _, t := range sp.targets {
		t.loop.wait()
	}
}

type targetEntry struct {
	target *Target
	loop   loop
	cache  *scrapeCache
	cancel func()
}

func (sp *scrapePool) getTargets() []*Target {
	sp.mtx.RLock()
	defer sp.mtx.RUnlock()

	res := make([]*Target, 0, len(sp.targets))

	for _, t := range sp.targets {
		res = append(res, t.target)
	}
	return res
}

func (sp *scrapePool) updateConfig(cfg *config.ScrapeConfig) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	client, err := httputil.NewClientFromConfig(cfg.HTTPClientConfig)
	if err != nil {
		// Any errors that could occur here should be caught during config validation.
		sp.logger.Errorf("Error creating HTTP client for job %q: %s", cfg.JobName, err)
	}

	sp.config = cfg
	sp.client = client

	sp.newLoop = func(interval time.Duration) loop {
		return &
	}
}

// Sync converts target groups into actual scrape targets and synchronizes
// the currently running scraper with the resulting set.
func (sp *scrapePool) Sync(tgs []*config.TargetGroup) {
	sp.mtx.RLock()
	defer sp.mtx.RUnlock()

	start := time.Now()

	newTargets := map[uint64]*Target{}

	for _, tg := range tgs {
		targets, err := targetsFromGroup(tg, sp.config)
		if err != nil {
			sp.logger.With("err", err).Error("creating targets failed")
			continue
		}
		for _, t := range targets {
			newTargets[t.hash()] = t
		}
	}

	var termwg sync.WaitGroup

	for hash, t := range sp.targets {
		// Kill loops for targets that were removed or if
		newTarget, ok := newTargets[hash]
		if !ok {
			go func(t *targetEntry, hash uint64) {
				t.cancel()
				t.loop.wait()
				termwg.Done()
			}(t, hash)

			delete(sp.targets, hash)
			continue
		}

		// Update targets that remained. Scrape config options other than
		// the interval are always applied on demand on every loop iteration.
		// Scrape cache entries are invalidated automatically if config changes
		// create different series.
		t.target = newTarget
		t.loop.setInterval(time.Duration(sp.config.ScrapeInterval))
	}

	// Create new targets.
	for hash, newTarget := range newTargets {
		if _, ok := sp.targets[hash]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(sp.ctx)

		loop := sp.newLoop(time.Duration(sp.config.ScrapeInterval))
		loop.run(ctx)

		sp.targets[hash] = &targetEntry{
			target: newTarget,
			loop:   loop,
			cache:  newScrapeCache(),
			cancel: cancel,
		}
	}

	// Wait for all terminated loops to exit so two successive calls to Sync
	// do not cause overlapping loops for the same targets.
	termwg.Wait()

	targetSyncIntervalLength.WithLabelValues(sp.config.JobName).Observe(
		time.Since(start).Seconds(),
	)
	targetScrapePoolSyncsCounter.WithLabelValues(sp.config.JobName).Inc()
}

func mutateSampleLabels(cfg *config.ScrapeConfig, lset labels.Labels, target *Target) labels.Labels {
	lb := labels.NewBuilder(lset)

	if cfg.HonorLabels {
		for _, l := range target.Labels() {
			if lv := lset.Get(l.Name); lv == "" {
				lb.Set(l.Name, l.Value)
			}
		}
	} else {
		for _, l := range target.Labels() {
			lv := lset.Get(l.Name)
			if lv != "" {
				lb.Set(model.ExportedLabelPrefix+l.Name, lv)
			}
			lb.Set(l.Name, l.Value)
		}
	}

	res := lb.Labels()

	if mrc := cfg.MetricRelabelConfigs; len(mrc) > 0 {
		res = relabel.Process(res, mrc...)
	}

	return res
}

func mutateReportSampleLabels(lset labels.Labels, target *Target) labels.Labels {
	lb := labels.NewBuilder(lset)

	for _, l := range target.Labels() {
		lv := lset.Get(l.Name)
		if lv != "" {
			lb.Set(model.ExportedLabelPrefix+l.Name, lv)
		}
		lb.Set(l.Name, l.Value)
	}

	return lb.Labels()
}

// sampleAppender returns an appender for ingested samples from the target.
func newSampleAppender(base Appendable, maxAheadTime int64, sampleLimit uint) storage.Appender {
	app, err := sp.appendable.Appender()
	if err != nil {
		panic(err)
	}

	app = &timeLimitAppender{
		Appender: app,
		maxTime:  timestamp.FromTime(time.Now().Add(10 * time.Minute)),
	}

	// The limit is applied after metrics are potentially dropped via relabeling.
	if sampleLimit > 0 {
		app = &limitAppender{
			Appender: app,
			limit:    sampleLimit,
		}
	}

	return app
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

	client  *http.Client
	req     *http.Request
	timeout time.Duration

	gzipr *gzip.Reader
	buf   *bufio.Reader
}

type ingestor func([]byte) error

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
		req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", fmt.Sprintf("%f", s.timeout.Seconds()))

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

type lsetCacheEntry struct {
	metric string
	lset   labels.Labels
	hash   uint64
}

type refEntry struct {
	ref      string
	lastIter uint64
}

type scrapeLoop struct {
	scraper scraper
	l       log.Logger
	cache   *scrapeCache

	lastScrapeSize int

	appender            func() storage.Appender
	client func() *http.Client

	sampleMutator      func() labelsMutator
	reportSampleMutator func() labelsMutator

	ctx       context.Context
	scrapeCtx context.Context
	cancel    func()
	stopped   chan struct{}
}

type scrapeLoop2 struct {
	scraper  scraper
	ingestor ingestor
}

func (sl *scrapeLoop2) run(ctx context.Context, interval time.Duration) {
	select {
	case <-time.After(sl.scraper.offset(interval)):
		// Continue after a scraping offset.
	case <-sl.scrapeCtx.Done():
		close(sl.stopped)
		return
	}

	var last time.Time

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

mainLoop:
	for {
		select {
		case <-sl.ctx.Done():
			close(sl.stopped)
			return
		case <-sl.scrapeCtx.Done():
			break mainLoop
		default:
		}

		var (
			start             = time.Now()
			scrapeCtx, cancel = context.WithTimeout(sl.ctx, timeout)
		)

		// Only record after the first scrape.
		if !last.IsZero() {
			targetIntervalLength.WithLabelValues(interval.String()).Observe(
				time.Since(last).Seconds(),
			)
		}

		b := scrapeBuffers.Get(sl.lastScrapeSize)
		buf := bytes.NewBuffer(b)

		scrapeErr := sl.scraper.scrape(scrapeCtx, buf)
		cancel()
		if scrapeErr == nil {
			b = buf.Bytes()
			// NOTE: There were issues with misbehaving clients in the past
			// that occasionally returned empty results. We don't want those
			// to falsely reset our buffer size.
			if len(b) > 0 {
				sl.lastScrapeSize = len(b)
			}
		} else if errc != nil {
			errc <- scrapeErr
		}

		// A failed scrape is the same as an empty scrape,
		// we still call sl.append to trigger stale markers.
		total, added, appErr := sl.append(b, start)
		if appErr != nil {
			sl.l.With("err", appErr).Warn("append failed")
			// The append failed, probably due to a parse error or sample limit.
			// Call sl.append again with an empty scrape to trigger stale markers.
			if _, _, err := sl.append([]byte{}, start); err != nil {
				sl.l.With("err", err).Error("append failed")
			}
		}
		scrapeBuffers.Put(b)

		if scrapeErr == nil {
			scrapeErr = appErr
		}

		sl.report(start, time.Since(start), total, added, scrapeErr)
		last = start

		select {
		case <-sl.ctx.Done():
			close(sl.stopped)
			return
		case <-sl.scrapeCtx.Done():
			break mainLoop
		case <-ticker.C:
		}
	}

	close(sl.stopped)

	sl.endOfRunStaleness(last, ticker, interval)
}


// scrapeCache tracks mappings of exposed metric strings to label sets and
// storage references. Additionally, it tracks staleness of series between
// scrapes.
type scrapeCache struct {
	iter uint64 // Current scrape iteration.

	refs  map[string]*refEntry       // Parsed string to ref.
	lsets map[string]*lsetCacheEntry // Ref to labelset and string.

	// Cache of dropped metric strings and their iteration. The iteration must
	// be a pointer so we can update it without setting a new entry with an unsafe
	// string in addDropped().
	dropped map[string]*uint64

	// seriesCur and seriesPrev store the labels of series that were seen
	// in the current and previous scrape.
	// We hold two maps and swap them out to save allocations.
	seriesCur  map[uint64]labels.Labels
	seriesPrev map[uint64]labels.Labels
}

func newScrapeCache() *scrapeCache {
	return &scrapeCache{
		refs:       map[string]*refEntry{},
		lsets:      map[string]*lsetCacheEntry{},
		seriesCur:  map[uint64]labels.Labels{},
		seriesPrev: map[uint64]labels.Labels{},
		dropped:    map[string]*uint64{},
	}
}

func (c *scrapeCache) iterDone() {
	// refCache and lsetCache may grow over time through series churn
	// or multiple string representations of the same metric. Clean up entries
	// that haven't appeared in the last scrape.
	for s, e := range c.refs {
		if e.lastIter < c.iter {
			delete(c.refs, s)
			delete(c.lsets, e.ref)
		}
	}
	for s, iter := range c.dropped {
		if *iter < c.iter {
			delete(c.dropped, s)
		}
	}

	// Swap current and previous series.
	c.seriesPrev, c.seriesCur = c.seriesCur, c.seriesPrev

	// We have to delete every single key in the map.
	for k := range c.seriesCur {
		delete(c.seriesCur, k)
	}

	c.iter++
}

func (c *scrapeCache) addDropped(met string) {
	iter := c.iter
	c.dropped[met] = &iter
}

func (c *scrapeCache) getDropped(met string) bool {
	iterp, ok := c.dropped[met]
	if ok {
		*iterp = c.iter
	}
	return ok
}

func (c *scrapeCache) getRef(met string) (string, bool) {
	e, ok := c.refs[met]
	if !ok {
		return "", false
	}
	e.lastIter = c.iter
	return e.ref, true
}

func (c *scrapeCache) addRef(met, ref string, lset labels.Labels, hash uint64) {
	if ref == "" {
		return
	}
	// Clean up the label set cache before overwriting the ref for a previously seen
	// metric representation. It won't be caught by the cleanup in iterDone otherwise.
	if e, ok := c.refs[met]; ok {
		delete(c.lsets, e.ref)
	}
	c.refs[met] = &refEntry{ref: ref, lastIter: c.iter}
	// met is the raw string the metric was ingested as. The label set is not ordered
	// and thus it's not suitable to uniquely identify cache entries.
	// We store a hash over the label set instead.
	c.lsets[ref] = &lsetCacheEntry{metric: met, lset: lset, hash: hash}
}

func (c *scrapeCache) trackStaleness(hash uint64, lset labels.Labels) {
	c.seriesCur[hash] = lset
}

func (c *scrapeCache) forEachStale(f func(labels.Labels) bool) {
	for h, lset := range c.seriesPrev {
		if _, ok := c.seriesCur[h]; !ok {
			if !f(lset) {
				break
			}
		}
	}
}

func newScrapeLoop(
	ctx context.Context,
	sc scraper,
	l log.Logger,
	sampleMutator labelsMutator,
	reportSampleMutator labelsMutator,
	appender func() storage.Appender,
) *scrapeLoop {
	if l == nil {
		l = log.Base()
	}
	sl := &scrapeLoop{
		scraper:             sc,
		appender:            appender,
		cache:               newScrapeCache(),
		sampleMutator:       sampleMutator,
		reportSampleMutator: reportSampleMutator,
		lastScrapeSize:      16000,
		stopped:             make(chan struct{}),
		ctx:                 ctx,
		l:                   l,
	}
	sl.scrapeCtx, sl.cancel = context.WithCancel(ctx)

	return sl
}

var scrapeBuffers = pool.NewBytesPool(16e3, 100e6, 3)

func (sl *scrapeLoop) run(interval, timeout time.Duration, errc chan<- error) {
	select {
	case <-time.After(sl.scraper.offset(interval)):
		// Continue after a scraping offset.
	case <-sl.scrapeCtx.Done():
		close(sl.stopped)
		return
	}

	var last time.Time

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

mainLoop:
	for {
		select {
		case <-sl.ctx.Done():
			close(sl.stopped)
			return
		case <-sl.scrapeCtx.Done():
			break mainLoop
		default:
		}

		var (
			start             = time.Now()
			scrapeCtx, cancel = context.WithTimeout(sl.ctx, timeout)
		)

		// Only record after the first scrape.
		if !last.IsZero() {
			targetIntervalLength.WithLabelValues(interval.String()).Observe(
				time.Since(last).Seconds(),
			)
		}

		b := scrapeBuffers.Get(sl.lastScrapeSize)
		buf := bytes.NewBuffer(b)

		scrapeErr := sl.scraper.scrape(scrapeCtx, buf)
		cancel()
		if scrapeErr == nil {
			b = buf.Bytes()
			// NOTE: There were issues with misbehaving clients in the past
			// that occasionally returned empty results. We don't want those
			// to falsely reset our buffer size.
			if len(b) > 0 {
				sl.lastScrapeSize = len(b)
			}
		} else if errc != nil {
			errc <- scrapeErr
		}

		// A failed scrape is the same as an empty scrape,
		// we still call sl.append to trigger stale markers.
		total, added, appErr := sl.append(b, start)
		if appErr != nil {
			sl.l.With("err", appErr).Warn("append failed")
			// The append failed, probably due to a parse error or sample limit.
			// Call sl.append again with an empty scrape to trigger stale markers.
			if _, _, err := sl.append([]byte{}, start); err != nil {
				sl.l.With("err", err).Error("append failed")
			}
		}
		scrapeBuffers.Put(b)

		if scrapeErr == nil {
			scrapeErr = appErr
		}

		sl.report(start, time.Since(start), total, added, scrapeErr)
		last = start

		select {
		case <-sl.ctx.Done():
			close(sl.stopped)
			return
		case <-sl.scrapeCtx.Done():
			break mainLoop
		case <-ticker.C:
		}
	}

	close(sl.stopped)

	sl.endOfRunStaleness(last, ticker, interval)
}

func (sl *scrapeLoop) endOfRunStaleness(last time.Time, ticker *time.Ticker, interval time.Duration) {
	// Scraping has stopped. We want to write stale markers but
	// the target may be recreated, so we wait just over 2 scrape intervals
	// before creating them.
	// If the context is cancelled, we presume the server is shutting down
	// and will restart where is was. We do not attempt to write stale markers
	// in this case.

	if last.IsZero() {
		// There never was a scrape, so there will be no stale markers.
		return
	}

	// Wait for when the next scrape would have been, record its timestamp.
	var staleTime time.Time
	select {
	case <-sl.ctx.Done():
		return
	case <-ticker.C:
		staleTime = time.Now()
	}

	// Wait for when the next scrape would have been, if the target was recreated
	// samples should have been ingested by now.
	select {
	case <-sl.ctx.Done():
		return
	case <-ticker.C:
	}

	// Wait for an extra 10% of the interval, just to be safe.
	select {
	case <-sl.ctx.Done():
		return
	case <-time.After(interval / 10):
	}

	// Call sl.append again with an empty scrape to trigger stale markers.
	// If the target has since been recreated and scraped, the
	// stale markers will be out of order and ignored.
	if _, _, err := sl.append([]byte{}, staleTime); err != nil {
		sl.l.With("err", err).Error("stale append failed")
	}
	if err := sl.reportStale(staleTime); err != nil {
		sl.l.With("err", err).Error("stale report failed")
	}
}

// Stop the scraping. May still write data and stale markers after it has
// returned. Cancel the context to stop all writes.
func (sl *scrapeLoop) stop() {
	sl.cancel()
	<-sl.stopped
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
		app            = sl.appender()
		p              = textparse.New(b)
		defTime        = timestamp.FromTime(ts)
		numOutOfOrder  = 0
		numDuplicates  = 0
		numOutOfBounds = 0
	)
	var sampleLimitErr error

loop:
	for p.Next() {
		total++

		t := defTime
		met, tp, v := p.At()
		if tp != nil {
			t = *tp
		}

		if sl.cache.getDropped(yoloString(met)) {
			continue
		}
		ref, ok := sl.cache.getRef(yoloString(met))
		if ok {
			lset := sl.cache.lsets[ref].lset
			switch err = app.AddFast(lset, ref, t, v); err {
			case nil:
				if tp == nil {
					e := sl.cache.lsets[ref]
					sl.cache.trackStaleness(e.hash, e.lset)
				}
			case storage.ErrNotFound:
				ok = false
			case storage.ErrOutOfOrderSample:
				numOutOfOrder++
				sl.l.With("timeseries", string(met)).Debug("Out of order sample")
				targetScrapeSampleOutOfOrder.Inc()
				continue
			case storage.ErrDuplicateSampleForTimestamp:
				numDuplicates++
				sl.l.With("timeseries", string(met)).Debug("Duplicate sample for timestamp")
				targetScrapeSampleDuplicate.Inc()
				continue
			case storage.ErrOutOfBounds:
				numOutOfBounds++
				sl.l.With("timeseries", string(met)).Debug("Out of bounds metric")
				targetScrapeSampleOutOfBounds.Inc()
				continue
			case errSampleLimit:
				// Keep on parsing output if we hit the limit, so we report the correct
				// total number of samples scraped.
				sampleLimitErr = err
				added++
				continue
			default:
				break loop
			}
		}
		if !ok {
			var (
				lset labels.Labels
				mets string
				hash uint64
			)
			if e, ok := sl.cache.lsets[ref]; ok {
				mets = e.metric
				lset = e.lset
				hash = e.hash
			} else {
				mets = p.Metric(&lset)
				hash = lset.Hash()

				// Hash label set as it is seen local to the target. Then add target labels
				// and relabeling and store the final label set.
				lset = sl.sampleMutator(lset)

				// The label set may be set to nil to indicate dropping.
				if lset == nil {
					sl.cache.addDropped(mets)
					continue
				}
			}

			var ref string
			ref, err = app.Add(lset, t, v)
			// TODO(fabxc): also add a dropped-cache?
			switch err {
			case nil:
			case storage.ErrOutOfOrderSample:
				err = nil
				numOutOfOrder++
				sl.l.With("timeseries", string(met)).Debug("Out of order sample")
				targetScrapeSampleOutOfOrder.Inc()
				continue
			case storage.ErrDuplicateSampleForTimestamp:
				err = nil
				numDuplicates++
				sl.l.With("timeseries", string(met)).Debug("Duplicate sample for timestamp")
				targetScrapeSampleDuplicate.Inc()
				continue
			case storage.ErrOutOfBounds:
				err = nil
				numOutOfBounds++
				sl.l.With("timeseries", string(met)).Debug("Out of bounds metric")
				targetScrapeSampleOutOfBounds.Inc()
				continue
			case errSampleLimit:
				sampleLimitErr = err
				added++
				continue
			default:
				break loop
			}
			if tp == nil {
				// Bypass staleness logic if there is an explicit timestamp.
				sl.cache.trackStaleness(hash, lset)
			}
			sl.cache.addRef(mets, ref, lset, hash)
		}
		added++
	}
	if err == nil {
		err = p.Err()
	}
	if err == nil && sampleLimitErr != nil {
		targetScrapeSampleLimit.Inc()
		err = sampleLimitErr
	}
	if numOutOfOrder > 0 {
		sl.l.With("numDropped", numOutOfOrder).Warn("Error on ingesting out-of-order samples")
	}
	if numDuplicates > 0 {
		sl.l.With("numDropped", numDuplicates).Warn("Error on ingesting samples with different value but same timestamp")
	}
	if numOutOfBounds > 0 {
		sl.l.With("numOutOfBounds", numOutOfBounds).Warn("Error on ingesting samples that are too old or are too far into the future")
	}
	if err == nil {
		sl.cache.forEachStale(func(lset labels.Labels) bool {
			// Series no longer exposed, mark it stale.
			_, err = app.Add(lset, defTime, math.Float64frombits(value.StaleNaN))
			switch err {
			case storage.ErrOutOfOrderSample, storage.ErrDuplicateSampleForTimestamp:
				// Do not count these in logging, as this is expected if a target
				// goes away and comes back again with a new scrape loop.
				err = nil
			}
			return err == nil
		})
	}
	if err != nil {
		app.Rollback()
		return total, added, err
	}
	if err := app.Commit(); err != nil {
		return total, added, err
	}

	sl.cache.iterDone()

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
	app := sl.appender()

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

func (sl *scrapeLoop) reportStale(start time.Time) error {
	ts := timestamp.FromTime(start)
	app := sl.appender()
	stale := math.Float64frombits(value.StaleNaN)

	if err := sl.addReportSample(app, scrapeHealthMetricName, ts, stale); err != nil {
		app.Rollback()
		return err
	}
	if err := sl.addReportSample(app, scrapeDurationMetricName, ts, stale); err != nil {
		app.Rollback()
		return err
	}
	if err := sl.addReportSample(app, scrapeSamplesMetricName, ts, stale); err != nil {
		app.Rollback()
		return err
	}
	if err := sl.addReportSample(app, samplesPostRelabelMetricName, ts, stale); err != nil {
		app.Rollback()
		return err
	}
	return app.Commit()
}

func (sl *scrapeLoop) addReportSample(app storage.Appender, s string, t int64, v float64) error {
	// Suffix s with the invalid \xff unicode rune to avoid collisions
	// with scraped metrics.
	s2 := s + "\xff"

	ref, ok := sl.cache.getRef(s2)
	if ok {
		lset := sl.cache.lsets[ref].lset
		err := app.AddFast(lset, ref, t, v)
		switch err {
		case nil:
			return nil
		case storage.ErrNotFound:
			// Try an Add.
		case storage.ErrOutOfOrderSample, storage.ErrDuplicateSampleForTimestamp:
			// Do not log here, as this is expected if a target goes away and comes back
			// again with a new scrape loop.
			return nil
		default:
			return err
		}
	}
	lset := labels.Labels{
		labels.Label{Name: labels.MetricName, Value: s},
	}
	hash := lset.Hash()
	lset = sl.reportSampleMutator(lset)

	ref, err := app.Add(lset, t, v)
	switch err {
	case nil:
		sl.cache.addRef(s2, ref, lset, hash)
		return nil
	case storage.ErrOutOfOrderSample, storage.ErrDuplicateSampleForTimestamp:
		return nil
	default:
		return err
	}
}
