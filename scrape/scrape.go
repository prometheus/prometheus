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

package scrape

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptrace"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/klauspost/compress/gzip"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/version"
	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/logging"
	"github.com/prometheus/prometheus/util/namevalidationutil"
	"github.com/prometheus/prometheus/util/pool"
)

var aOptionRejectEarlyOOO = storage.AppendOptions{DiscardOutOfOrder: true}

// ScrapeTimestampTolerance is the tolerance for scrape appends timestamps
// alignment, to enable better compression at the TSDB level.
// See https://github.com/prometheus/prometheus/issues/7846
var ScrapeTimestampTolerance = 2 * time.Millisecond

// AlignScrapeTimestamps enables the tolerance for scrape appends timestamps described above.
var AlignScrapeTimestamps = true

var errNameLabelMandatory = fmt.Errorf("missing metric name (%s label)", model.MetricNameLabel)

var _ FailureLogger = (*logging.JSONFileLogger)(nil)

// FailureLogger is an interface that can be used to log all failed
// scrapes.
type FailureLogger interface {
	slog.Handler
	io.Closer
}

// scrapePool manages scrapes for sets of targets.
type scrapePool struct {
	appendable storage.Appendable
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	options    *Options

	// mtx must not be taken after targetMtx.
	mtx    sync.Mutex
	config *config.ScrapeConfig
	client *http.Client
	loops  map[uint64]loop

	symbolTable           *labels.SymbolTable
	lastSymbolTableCheck  time.Time
	initialSymbolTableLen int

	targetMtx sync.Mutex
	// activeTargets and loops must always be synchronized to have the same
	// set of hashes.
	activeTargets       map[uint64]*Target
	droppedTargets      []*Target // Subject to KeepDroppedTargets limit.
	droppedTargetsCount int       // Count of all dropped targets.

	// newLoop injection for testing purposes.
	injectTestNewLoop func(scrapeLoopOptions) loop

	metrics    *scrapeMetrics
	buffers    *pool.Pool
	offsetSeed uint64

	scrapeFailureLogger    FailureLogger
	scrapeFailureLoggerMtx sync.RWMutex
}

type labelLimits struct {
	labelLimit            int
	labelNameLengthLimit  int
	labelValueLengthLimit int
}

const maxAheadTime = 10 * time.Minute

// returning an empty label set is interpreted as "drop".
type labelsMutator func(labels.Labels) labels.Labels

// scrapeLoopAppendAdapter allows support for multiple storage.Appender versions.
type scrapeLoopAppendAdapter interface {
	Commit() error
	Rollback() error

	addReportSample(s reportSample, t int64, v float64, b *labels.Builder, rejectOOO bool) error
	append(b []byte, contentType string, ts time.Time) (total, added, seriesAdded int, err error)
}

func newScrapePool(
	cfg *config.ScrapeConfig,
	appendable storage.Appendable,
	offsetSeed uint64,
	logger *slog.Logger,
	buffers *pool.Pool,
	options *Options,
	metrics *scrapeMetrics,
) (*scrapePool, error) {
	if logger == nil {
		logger = promslog.NewNopLogger()
	}
	if buffers == nil {
		buffers = pool.New(1e3, 1e6, 3, func(sz int) any { return make([]byte, 0, sz) })
	}

	client, err := newScrapeClient(cfg.HTTPClientConfig, cfg.JobName, options.HTTPClientOptions...)
	if err != nil {
		return nil, err
	}

	// Validate scheme so we don't need to do it later.
	// We also do it on scrapePool.reload(...)
	// TODO(bwplotka): Can we move it to scrape config validation?
	if err := namevalidationutil.CheckNameValidationScheme(cfg.MetricNameValidationScheme); err != nil {
		return nil, errors.New("newScrapePool: MetricNameValidationScheme must be set in scrape configuration")
	}
	if _, err = config.ToEscapingScheme(cfg.MetricNameEscapingScheme, cfg.MetricNameValidationScheme); err != nil {
		return nil, fmt.Errorf("invalid metric name escaping scheme, %w", err)
	}

	symbols := labels.NewSymbolTable()
	ctx, cancel := context.WithCancel(context.Background())
	sp := &scrapePool{
		appendable:           appendable,
		logger:               logger,
		ctx:                  ctx,
		cancel:               cancel,
		options:              options,
		config:               cfg,
		client:               client,
		loops:                map[uint64]loop{},
		symbolTable:          symbols,
		lastSymbolTableCheck: time.Now(),
		activeTargets:        map[uint64]*Target{},
		metrics:              metrics,
		buffers:              buffers,
		offsetSeed:           offsetSeed,
	}
	sp.metrics.targetScrapePoolTargetLimit.WithLabelValues(sp.config.JobName).Set(float64(sp.config.TargetLimit))
	return sp, nil
}

func (sp *scrapePool) newLoop(opts scrapeLoopOptions) loop {
	if sp.injectTestNewLoop != nil {
		return sp.injectTestNewLoop(opts)
	}
	return newScrapeLoop(opts)
}

func (sp *scrapePool) ActiveTargets() []*Target {
	sp.targetMtx.Lock()
	defer sp.targetMtx.Unlock()

	var tActive []*Target
	for _, t := range sp.activeTargets {
		tActive = append(tActive, t)
	}
	return tActive
}

// Return dropped targets, subject to KeepDroppedTargets limit.
func (sp *scrapePool) DroppedTargets() []*Target {
	sp.targetMtx.Lock()
	defer sp.targetMtx.Unlock()
	return sp.droppedTargets
}

func (sp *scrapePool) DroppedTargetsCount() int {
	sp.targetMtx.Lock()
	defer sp.targetMtx.Unlock()
	return sp.droppedTargetsCount
}

func (sp *scrapePool) SetScrapeFailureLogger(l FailureLogger) {
	sp.scrapeFailureLoggerMtx.Lock()
	defer sp.scrapeFailureLoggerMtx.Unlock()
	if l != nil {
		l = slog.New(l).With("job_name", sp.config.JobName).Handler().(FailureLogger)
	}
	sp.scrapeFailureLogger = l

	sp.targetMtx.Lock()
	defer sp.targetMtx.Unlock()
	for _, s := range sp.loops {
		s.setScrapeFailureLogger(sp.scrapeFailureLogger)
	}
}

func (sp *scrapePool) getScrapeFailureLogger() FailureLogger {
	sp.scrapeFailureLoggerMtx.RLock()
	defer sp.scrapeFailureLoggerMtx.RUnlock()
	return sp.scrapeFailureLogger
}

// stop terminates all scrape loops and returns after they all terminated.
func (sp *scrapePool) stop() {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	sp.cancel()
	var wg sync.WaitGroup

	sp.targetMtx.Lock()

	for fp, l := range sp.loops {
		wg.Add(1)

		go func(l loop) {
			l.stop()
			wg.Done()
		}(l)

		delete(sp.loops, fp)
		delete(sp.activeTargets, fp)
	}

	sp.targetMtx.Unlock()

	wg.Wait()
	sp.client.CloseIdleConnections()

	if sp.config != nil {
		sp.metrics.targetScrapePoolSyncsCounter.DeleteLabelValues(sp.config.JobName)
		sp.metrics.targetScrapePoolTargetLimit.DeleteLabelValues(sp.config.JobName)
		sp.metrics.targetScrapePoolTargetsAdded.DeleteLabelValues(sp.config.JobName)
		sp.metrics.targetScrapePoolSymbolTableItems.DeleteLabelValues(sp.config.JobName)
		sp.metrics.targetSyncIntervalLength.DeleteLabelValues(sp.config.JobName)
		sp.metrics.targetSyncIntervalLengthHistogram.DeleteLabelValues(sp.config.JobName)
		sp.metrics.targetSyncFailed.DeleteLabelValues(sp.config.JobName)
	}
}

// reload the scrape pool with the given scrape configuration. The target state is preserved
// but all scrape loops are restarted with the new scrape configuration.
// This method returns after all scrape loops that were stopped have stopped scraping.
func (sp *scrapePool) reload(cfg *config.ScrapeConfig) error {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	sp.metrics.targetScrapePoolReloads.Inc()
	start := time.Now()

	client, err := newScrapeClient(cfg.HTTPClientConfig, cfg.JobName, sp.options.HTTPClientOptions...)
	if err != nil {
		sp.metrics.targetScrapePoolReloadsFailed.Inc()
		return err
	}

	reuseCache := reusableCache(sp.config, cfg)
	sp.config = cfg
	oldClient := sp.client
	sp.client = client

	// Validate scheme so we don't need to do it later.
	if err := namevalidationutil.CheckNameValidationScheme(cfg.MetricNameValidationScheme); err != nil {
		return errors.New("scrapePool.reload: MetricNameValidationScheme must be set in scrape configuration")
	}
	if _, err = config.ToEscapingScheme(cfg.MetricNameEscapingScheme, cfg.MetricNameValidationScheme); err != nil {
		return fmt.Errorf("scrapePool.reload: invalid metric name escaping scheme, %w", err)
	}
	sp.metrics.targetScrapePoolTargetLimit.WithLabelValues(sp.config.JobName).Set(float64(sp.config.TargetLimit))

	sp.restartLoops(reuseCache)
	oldClient.CloseIdleConnections()
	sp.metrics.targetReloadIntervalLength.WithLabelValues(time.Duration(sp.config.ScrapeInterval).String()).Observe(
		time.Since(start).Seconds(),
	)
	return nil
}

func (sp *scrapePool) restartLoops(reuseCache bool) {
	var wg sync.WaitGroup
	sp.targetMtx.Lock()

	forcedErr := sp.refreshTargetLimitErr()
	for fp, oldLoop := range sp.loops {
		var cache *scrapeCache
		if oc := oldLoop.getCache(); reuseCache && oc != nil {
			oldLoop.disableEndOfRunStalenessMarkers()
			cache = oc
		} else {
			cache = newScrapeCache(sp.metrics)
		}

		t := sp.activeTargets[fp]
		targetInterval, targetTimeout, err := t.intervalAndTimeout(
			time.Duration(sp.config.ScrapeInterval),
			time.Duration(sp.config.ScrapeTimeout),
		)
		escapingScheme, _ := config.ToEscapingScheme(sp.config.MetricNameEscapingScheme, sp.config.MetricNameValidationScheme)
		newLoop := sp.newLoop(scrapeLoopOptions{
			target: t,
			scraper: &targetScraper{
				Target:               t,
				client:               sp.client,
				timeout:              targetTimeout,
				bodySizeLimit:        int64(sp.config.BodySizeLimit),
				acceptHeader:         acceptHeader(sp.config.ScrapeProtocols, escapingScheme),
				acceptEncodingHeader: acceptEncodingHeader(sp.config.EnableCompression),
				metrics:              sp.metrics,
			},
			cache:    cache,
			interval: targetInterval,
			timeout:  targetTimeout,
			sp:       sp,
		})
		if err != nil {
			newLoop.setForcedError(err)
		}
		wg.Add(1)

		go func(oldLoop, newLoop loop) {
			oldLoop.stop()
			wg.Done()

			newLoop.setForcedError(forcedErr)
			newLoop.setScrapeFailureLogger(sp.getScrapeFailureLogger())
			newLoop.run(nil)
		}(oldLoop, newLoop)

		sp.loops[fp] = newLoop
	}

	sp.targetMtx.Unlock()

	wg.Wait()
}

// Must be called with sp.mtx held.
func (sp *scrapePool) checkSymbolTable() {
	// Here we take steps to clear out the symbol table if it has grown a lot.
	// After waiting some time for things to settle, we take the size of the symbol-table.
	// If, after some more time, the table has grown to twice that size, we start a new one.
	const minTimeToCleanSymbolTable = 5 * time.Minute
	if time.Since(sp.lastSymbolTableCheck) > minTimeToCleanSymbolTable {
		if sp.initialSymbolTableLen == 0 {
			sp.initialSymbolTableLen = sp.symbolTable.Len()
		} else if sp.symbolTable.Len() > 2*sp.initialSymbolTableLen {
			sp.symbolTable = labels.NewSymbolTable()
			sp.initialSymbolTableLen = 0
			sp.restartLoops(false) // To drop all caches.
		}
		sp.lastSymbolTableCheck = time.Now()
	}
}

// Sync converts target groups into actual scrape targets and synchronizes
// the currently running scraper with the resulting set and returns all scraped and dropped targets.
func (sp *scrapePool) Sync(tgs []*targetgroup.Group) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	start := time.Now()

	sp.targetMtx.Lock()
	var all []*Target
	var targets []*Target
	lb := labels.NewBuilderWithSymbolTable(sp.symbolTable)
	sp.droppedTargets = []*Target{}
	sp.droppedTargetsCount = 0
	for _, tg := range tgs {
		targets, failures := TargetsFromGroup(tg, sp.config, targets, lb)
		for _, err := range failures {
			sp.logger.Error("Creating target failed", "err", err)
		}
		sp.metrics.targetSyncFailed.WithLabelValues(sp.config.JobName).Add(float64(len(failures)))
		for _, t := range targets {
			// Replicate .Labels().IsEmpty() with a loop here to avoid generating garbage.
			nonEmpty := false
			t.LabelsRange(func(labels.Label) { nonEmpty = true })
			switch {
			case nonEmpty:
				all = append(all, t)
			default:
				if sp.config.KeepDroppedTargets == 0 || uint(len(sp.droppedTargets)) < sp.config.KeepDroppedTargets {
					sp.droppedTargets = append(sp.droppedTargets, t)
				}
				sp.droppedTargetsCount++
			}
		}
	}
	sp.metrics.targetScrapePoolSymbolTableItems.WithLabelValues(sp.config.JobName).Set(float64(sp.symbolTable.Len()))
	sp.targetMtx.Unlock()
	sp.sync(all)
	sp.checkSymbolTable()

	sp.metrics.targetSyncIntervalLength.WithLabelValues(sp.config.JobName).Observe(
		time.Since(start).Seconds(),
	)
	sp.metrics.targetSyncIntervalLengthHistogram.WithLabelValues(sp.config.JobName).Observe(
		time.Since(start).Seconds(),
	)
	sp.metrics.targetScrapePoolSyncsCounter.WithLabelValues(sp.config.JobName).Inc()
}

// sync takes a list of potentially duplicated targets, deduplicates them, starts
// scrape loops for new targets, and stops scrape loops for disappeared targets.
// It returns after all stopped scrape loops terminated.
func (sp *scrapePool) sync(targets []*Target) {
	uniqueLoops := make(map[uint64]loop)

	sp.targetMtx.Lock()
	escapingScheme, _ := config.ToEscapingScheme(sp.config.MetricNameEscapingScheme, sp.config.MetricNameValidationScheme)
	for _, t := range targets {
		hash := t.hash()

		if _, ok := sp.activeTargets[hash]; !ok {
			// The scrape interval and timeout labels are set to the config's values initially,
			// so whether changed via relabeling or not, they'll exist and hold the correct values
			// for every target.
			var err error
			targetInterval, targetTimeout, err := t.intervalAndTimeout(
				time.Duration(sp.config.ScrapeInterval),
				time.Duration(sp.config.ScrapeTimeout),
			)
			l := sp.newLoop(scrapeLoopOptions{
				target: t,
				scraper: &targetScraper{
					Target:               t,
					client:               sp.client,
					timeout:              targetTimeout,
					bodySizeLimit:        int64(sp.config.BodySizeLimit),
					acceptHeader:         acceptHeader(sp.config.ScrapeProtocols, escapingScheme),
					acceptEncodingHeader: acceptEncodingHeader(sp.config.EnableCompression),
					metrics:              sp.metrics,
				},
				cache:    newScrapeCache(sp.metrics),
				interval: targetInterval,
				timeout:  targetTimeout,
				sp:       sp,
			})
			if err != nil {
				l.setForcedError(err)
			}
			l.setScrapeFailureLogger(sp.scrapeFailureLogger)

			sp.activeTargets[hash] = t
			sp.loops[hash] = l

			uniqueLoops[hash] = l
		} else {
			// This might be a duplicated target.
			if _, ok := uniqueLoops[hash]; !ok {
				uniqueLoops[hash] = nil
			}
			// Need to keep the most updated ScrapeConfig for
			// displaying labels in the Service Discovery web page.
			sp.activeTargets[hash].SetScrapeConfig(sp.config, t.tLabels, t.tgLabels)
		}
	}

	var wg sync.WaitGroup

	// Stop and remove old targets and scraper loops.
	for hash := range sp.activeTargets {
		if _, ok := uniqueLoops[hash]; !ok {
			wg.Add(1)
			go func(l loop) {
				l.stop()
				wg.Done()
			}(sp.loops[hash])

			delete(sp.loops, hash)
			delete(sp.activeTargets, hash)
		}
	}

	sp.targetMtx.Unlock()

	sp.metrics.targetScrapePoolTargetsAdded.WithLabelValues(sp.config.JobName).Set(float64(len(uniqueLoops)))
	forcedErr := sp.refreshTargetLimitErr()
	for _, l := range sp.loops {
		l.setForcedError(forcedErr)
	}
	for _, l := range uniqueLoops {
		if l != nil {
			go l.run(nil)
		}
	}
	// Wait for all potentially stopped scrapers to terminate.
	// This covers the case of flapping targets. If the server is under high load, a new scraper
	// may be active and tries to insert. The old scraper that didn't terminate yet could still
	// be inserting a previous sample set.
	wg.Wait()
}

// refreshTargetLimitErr returns an error that can be passed to the scrape loops
// if the number of targets exceeds the configured limit.
func (sp *scrapePool) refreshTargetLimitErr() error {
	if sp.config == nil || sp.config.TargetLimit == 0 {
		return nil
	}
	if l := len(sp.activeTargets); l > int(sp.config.TargetLimit) {
		sp.metrics.targetScrapePoolExceededTargetLimit.Inc()
		return fmt.Errorf("target_limit exceeded (number of targets: %d, limit: %d)", l, sp.config.TargetLimit)
	}
	return nil
}

func (sp *scrapePool) disableEndOfRunStalenessMarkers(targets []*Target) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()
	for i := range targets {
		if l, ok := sp.loops[targets[i].hash()]; ok {
			l.disableEndOfRunStalenessMarkers()
		}
	}
}

func verifyLabelLimits(lset labels.Labels, limits *labelLimits) error {
	if limits == nil {
		return nil
	}

	met := lset.Get(model.MetricNameLabel)
	if limits.labelLimit > 0 {
		nbLabels := lset.Len()
		if nbLabels > limits.labelLimit {
			return fmt.Errorf("label_limit exceeded (metric: %.50s, number of labels: %d, limit: %d)", met, nbLabels, limits.labelLimit)
		}
	}

	if limits.labelNameLengthLimit == 0 && limits.labelValueLengthLimit == 0 {
		return nil
	}

	return lset.Validate(func(l labels.Label) error {
		if limits.labelNameLengthLimit > 0 {
			nameLength := len(l.Name)
			if nameLength > limits.labelNameLengthLimit {
				return fmt.Errorf("label_name_length_limit exceeded (metric: %.50s, label name: %.50s, length: %d, limit: %d)", met, l.Name, nameLength, limits.labelNameLengthLimit)
			}
		}

		if limits.labelValueLengthLimit > 0 {
			valueLength := len(l.Value)
			if valueLength > limits.labelValueLengthLimit {
				return fmt.Errorf("label_value_length_limit exceeded (metric: %.50s, label name: %.50s, value: %.50q, length: %d, limit: %d)", met, l.Name, l.Value, valueLength, limits.labelValueLengthLimit)
			}
		}
		return nil
	})
}

func mutateSampleLabels(lset labels.Labels, target *Target, honor bool, rc []*relabel.Config) labels.Labels {
	lb := labels.NewBuilder(lset)

	if honor {
		target.LabelsRange(func(l labels.Label) {
			if !lset.Has(l.Name) {
				lb.Set(l.Name, l.Value)
			}
		})
	} else {
		var conflictingExposedLabels []labels.Label
		target.LabelsRange(func(l labels.Label) {
			existingValue := lset.Get(l.Name)
			if existingValue != "" {
				conflictingExposedLabels = append(conflictingExposedLabels, labels.Label{Name: l.Name, Value: existingValue})
			}
			// It is now safe to set the target label.
			lb.Set(l.Name, l.Value)
		})

		if len(conflictingExposedLabels) > 0 {
			resolveConflictingExposedLabels(lb, conflictingExposedLabels)
		}
	}

	if keep := relabel.ProcessBuilder(lb, rc...); !keep {
		return labels.EmptyLabels()
	}

	return lb.Labels()
}

func resolveConflictingExposedLabels(lb *labels.Builder, conflictingExposedLabels []labels.Label) {
	slices.SortStableFunc(conflictingExposedLabels, func(a, b labels.Label) int {
		return len(a.Name) - len(b.Name)
	})

	for _, l := range conflictingExposedLabels {
		newName := l.Name
		for {
			newName = model.ExportedLabelPrefix + newName
			if lb.Get(newName) == "" {
				lb.Set(newName, l.Value)
				break
			}
		}
	}
}

func mutateReportSampleLabels(lset labels.Labels, target *Target) labels.Labels {
	lb := labels.NewBuilder(lset)

	target.LabelsRange(func(l labels.Label) {
		lb.Set(model.ExportedLabelPrefix+l.Name, lset.Get(l.Name))
		lb.Set(l.Name, l.Value)
	})

	return lb.Labels()
}

// appenderWithLimits returns an appender with additional validation.
func appenderWithLimits(app storage.Appender, sampleLimit, bucketLimit int, maxSchema int32) storage.Appender {
	app = &timeLimitAppender{
		Appender: app,
		maxTime:  timestamp.FromTime(time.Now().Add(maxAheadTime)),
	}

	// The sampleLimit is applied after metrics are potentially dropped via relabeling.
	if sampleLimit > 0 {
		app = &limitAppender{
			Appender: app,
			limit:    sampleLimit,
		}
	}

	if bucketLimit > 0 {
		app = &bucketLimitAppender{
			Appender: app,
			limit:    bucketLimit,
		}
	}

	if maxSchema < histogram.ExponentialSchemaMax {
		app = &maxSchemaAppender{
			Appender:  app,
			maxSchema: maxSchema,
		}
	}

	return app
}

// A scraper retrieves samples and accepts a status report at the end.
type scraper interface {
	scrape(ctx context.Context) (*http.Response, error)
	readResponse(ctx context.Context, resp *http.Response, w io.Writer) (string, error)
	Report(start time.Time, dur time.Duration, err error)
	offset(interval time.Duration, offsetSeed uint64) time.Duration
}

// targetScraper implements the scraper interface for a target.
type targetScraper struct {
	*Target

	client  *http.Client
	req     *http.Request
	timeout time.Duration

	gzipr *gzip.Reader
	buf   *bufio.Reader

	bodySizeLimit        int64
	acceptHeader         string
	acceptEncodingHeader string

	metrics *scrapeMetrics
}

var errBodySizeLimit = errors.New("body size limit exceeded")

// acceptHeader transforms preference from the options into specific header values as
// https://www.rfc-editor.org/rfc/rfc9110.html#name-accept defines.
// No validation is here, we expect scrape protocols to be validated already.
func acceptHeader(sps []config.ScrapeProtocol, scheme model.EscapingScheme) string {
	var vals []string
	weight := len(config.ScrapeProtocolsHeaders) + 1
	for _, sp := range sps {
		val := config.ScrapeProtocolsHeaders[sp]
		// Escaping header is only valid for newer versions of the text formats.
		if sp == config.PrometheusText1_0_0 || sp == config.OpenMetricsText1_0_0 {
			val += ";" + model.EscapingKey + "=" + scheme.String()
		}
		val += fmt.Sprintf(";q=0.%d", weight)
		vals = append(vals, val)
		weight--
	}
	// Default match anything.
	vals = append(vals, fmt.Sprintf("*/*;q=0.%d", weight))
	return strings.Join(vals, ",")
}

func acceptEncodingHeader(enableCompression bool) string {
	if enableCompression {
		return "gzip"
	}
	return "identity"
}

var UserAgent = version.PrometheusUserAgent()

func (s *targetScraper) scrape(ctx context.Context) (*http.Response, error) {
	if s.req == nil {
		req, err := http.NewRequest(http.MethodGet, s.URL().String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Add("Accept", s.acceptHeader)
		req.Header.Add("Accept-Encoding", s.acceptEncodingHeader)
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", strconv.FormatFloat(s.timeout.Seconds(), 'f', -1, 64))

		s.req = req
	}
	ctx, span := otel.Tracer("").Start(ctx, "Scrape", trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	return s.client.Do(s.req.WithContext(ctx))
}

func (s *targetScraper) readResponse(_ context.Context, resp *http.Response, w io.Writer) (string, error) {
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	if s.bodySizeLimit <= 0 {
		s.bodySizeLimit = math.MaxInt64
	}
	if resp.Header.Get("Content-Encoding") != "gzip" {
		n, err := io.Copy(w, io.LimitReader(resp.Body, s.bodySizeLimit))
		if err != nil {
			return "", err
		}
		if n >= s.bodySizeLimit {
			s.metrics.targetScrapeExceededBodySizeLimit.Inc()
			return "", errBodySizeLimit
		}
		return resp.Header.Get("Content-Type"), nil
	}

	if s.gzipr == nil {
		s.buf = bufio.NewReader(resp.Body)
		var err error
		s.gzipr, err = gzip.NewReader(s.buf)
		if err != nil {
			return "", err
		}
	} else {
		s.buf.Reset(resp.Body)
		if err := s.gzipr.Reset(s.buf); err != nil {
			return "", err
		}
	}

	n, err := io.Copy(w, io.LimitReader(s.gzipr, s.bodySizeLimit))
	s.gzipr.Close()
	if err != nil {
		return "", err
	}
	if n >= s.bodySizeLimit {
		s.metrics.targetScrapeExceededBodySizeLimit.Inc()
		return "", errBodySizeLimit
	}
	return resp.Header.Get("Content-Type"), nil
}

// A loop can run and be stopped again. It must not be reused after it was stopped.
type loop interface {
	run(errc chan<- error)
	setForcedError(err error)
	setScrapeFailureLogger(FailureLogger)
	stop()
	getCache() *scrapeCache
	disableEndOfRunStalenessMarkers()
}

type cacheEntry struct {
	ref      storage.SeriesRef
	lastIter uint64
	hash     uint64
	lset     labels.Labels
}

type scrapeLoop struct {
	// Parameters.
	ctx         context.Context
	cancel      func()
	stopped     chan struct{}
	parentCtx   context.Context
	appenderCtx context.Context
	l           *slog.Logger
	cache       *scrapeCache

	interval            time.Duration
	timeout             time.Duration
	sampleMutator       labelsMutator
	reportSampleMutator labelsMutator
	scraper             scraper

	// Static params per scrapePool.
	appendable  storage.Appendable
	buffers     *pool.Pool
	offsetSeed  uint64
	symbolTable *labels.SymbolTable
	metrics     *scrapeMetrics

	// Options from config.ScrapeConfig.
	sampleLimit                   int
	bucketLimit                   int
	maxSchema                     int32
	labelLimits                   *labelLimits
	honorLabels                   bool
	honorTimestamps               bool
	trackTimestampsStaleness      bool
	enableNativeHistogramScraping bool
	alwaysScrapeClassicHist       bool
	convertClassicHistToNHCB      bool
	fallbackScrapeProtocol        string
	enableCompression             bool
	mrc                           []*relabel.Config
	validationScheme              model.ValidationScheme

	// Options from scrape.Options.
	enableSTZeroIngestion   bool
	enableTypeAndUnitLabels bool
	reportExtraMetrics      bool
	appendMetadataToWAL     bool
	passMetadataInContext   bool
	skipOffsetting          bool // For testability.

	// error injection through setForcedError.
	forcedErr    error
	forcedErrMtx sync.Mutex

	// Special logger set on setScrapeFailureLogger
	scrapeFailureLoggerMtx sync.RWMutex
	scrapeFailureLogger    FailureLogger

	// Locally cached data.
	lastScrapeSize                   int
	disabledEndOfRunStalenessMarkers atomic.Bool
}

// scrapeCache tracks mappings of exposed metric strings to label sets and
// storage references. Additionally, it tracks staleness of series between
// scrapes.
// Cache is meant to be used per a single target.
type scrapeCache struct {
	iter uint64 // Current scrape iteration.

	// How many series and metadata entries there were at the last success.
	successfulCount int

	// Parsed string to an entry with information about the actual label set
	// and its storage reference.
	series map[string]*cacheEntry

	// Cache of dropped metric strings and their iteration. The iteration must
	// be a pointer so we can update it.
	droppedSeries map[string]*uint64

	// Series that were seen in the current and previous scrape, for staleness detection.
	seriesCur  map[storage.SeriesRef]*cacheEntry
	seriesPrev map[storage.SeriesRef]*cacheEntry

	// TODO(bwplotka): Consider moving metadata caching to head. See
	// https://github.com/prometheus/prometheus/issues/17619.
	metaMtx  sync.Mutex            // Mutex is needed due to api touching it when metadata is queried.
	metadata map[string]*metaEntry // metadata by metric family name.

	metrics *scrapeMetrics
}

// metaEntry holds meta information about a metric.
type metaEntry struct {
	metadata.Metadata

	lastIter       uint64 // Last scrape iteration the entry was observed at.
	lastIterChange uint64 // Last scrape iteration the entry was changed at.
}

func (m *metaEntry) size() int {
	// The attribute lastIter although part of the struct it is not metadata.
	return len(m.Help) + len(m.Unit) + len(m.Type)
}

func newScrapeCache(metrics *scrapeMetrics) *scrapeCache {
	return &scrapeCache{
		series:        map[string]*cacheEntry{},
		droppedSeries: map[string]*uint64{},
		seriesCur:     map[storage.SeriesRef]*cacheEntry{},
		seriesPrev:    map[storage.SeriesRef]*cacheEntry{},
		metadata:      map[string]*metaEntry{},
		metrics:       metrics,
	}
}

func (c *scrapeCache) iterDone(flushCache bool) {
	c.metaMtx.Lock()
	count := len(c.series) + len(c.droppedSeries) + len(c.metadata)
	c.metaMtx.Unlock()

	switch {
	case flushCache:
		c.successfulCount = count
	case count > c.successfulCount*2+1000:
		// If a target had varying labels in scrapes that ultimately failed,
		// the caches would grow indefinitely. Force a flush when this happens.
		// We use the heuristic that this is a doubling of the cache size
		// since the last scrape, and allow an additional 1000 in case
		// initial scrapes all fail.
		flushCache = true
		c.metrics.targetScrapeCacheFlushForced.Inc()
	}

	if flushCache {
		// All caches may grow over time through series churn
		// or multiple string representations of the same metric. Clean up entries
		// that haven't appeared in the last scrape.
		for s, e := range c.series {
			if c.iter != e.lastIter {
				delete(c.series, s)
			}
		}
		for s, iter := range c.droppedSeries {
			if c.iter != *iter {
				delete(c.droppedSeries, s)
			}
		}
		c.metaMtx.Lock()
		for m, e := range c.metadata {
			// Keep metadata around for 10 scrapes after its metric disappeared.
			if c.iter-e.lastIter > 10 {
				delete(c.metadata, m)
			}
		}
		c.metaMtx.Unlock()
	}

	// Swap current and previous series then clear the new current, to save allocations.
	c.seriesPrev, c.seriesCur = c.seriesCur, c.seriesPrev
	clear(c.seriesCur)

	c.iter++
}

func (c *scrapeCache) get(met []byte) (*cacheEntry, bool, bool) {
	e, ok := c.series[string(met)]
	if !ok {
		return nil, false, false
	}
	alreadyScraped := e.lastIter == c.iter
	e.lastIter = c.iter
	return e, true, alreadyScraped
}

func (c *scrapeCache) addRef(met []byte, ref storage.SeriesRef, lset labels.Labels, hash uint64) (ce *cacheEntry) {
	if ref == 0 {
		return nil
	}
	ce = &cacheEntry{ref: ref, lastIter: c.iter, lset: lset, hash: hash}
	c.series[string(met)] = ce
	return ce
}

func (c *scrapeCache) addDropped(met []byte) {
	iter := c.iter
	c.droppedSeries[string(met)] = &iter
}

func (c *scrapeCache) getDropped(met []byte) bool {
	iterp, ok := c.droppedSeries[string(met)]
	if ok {
		*iterp = c.iter
	}
	return ok
}

func (c *scrapeCache) trackStaleness(ref storage.SeriesRef, ce *cacheEntry) {
	c.seriesCur[ref] = ce
}

func (c *scrapeCache) forEachStale(f func(storage.SeriesRef, labels.Labels) bool) {
	for ref, ce := range c.seriesPrev {
		if _, ok := c.seriesCur[ref]; !ok {
			if !f(ce.ref, ce.lset) {
				break
			}
		}
	}
}

func yoloString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func (c *scrapeCache) setType(mfName []byte, t model.MetricType) ([]byte, *metaEntry) {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	e, ok := c.metadata[string(mfName)]
	if !ok {
		e = &metaEntry{Metadata: metadata.Metadata{Type: model.MetricTypeUnknown}}
		c.metadata[string(mfName)] = e
	}
	if e.Type != t {
		e.Type = t
		e.lastIterChange = c.iter
	}
	e.lastIter = c.iter
	return mfName, e
}

func (c *scrapeCache) setHelp(mfName, help []byte) ([]byte, *metaEntry) {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	e, ok := c.metadata[string(mfName)]
	if !ok {
		e = &metaEntry{Metadata: metadata.Metadata{Type: model.MetricTypeUnknown}}
		c.metadata[string(mfName)] = e
	}
	if e.Help != string(help) {
		e.Help = string(help)
		e.lastIterChange = c.iter
	}
	e.lastIter = c.iter
	return mfName, e
}

func (c *scrapeCache) setUnit(mfName, unit []byte) ([]byte, *metaEntry) {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	e, ok := c.metadata[string(mfName)]
	if !ok {
		e = &metaEntry{Metadata: metadata.Metadata{Type: model.MetricTypeUnknown}}
		c.metadata[string(mfName)] = e
	}
	if e.Unit != string(unit) {
		e.Unit = string(unit)
		e.lastIterChange = c.iter
	}
	e.lastIter = c.iter
	return mfName, e
}

// GetMetadata returns metadata given the metric family name.
func (c *scrapeCache) GetMetadata(mfName string) (MetricMetadata, bool) {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	m, ok := c.metadata[mfName]
	if !ok {
		return MetricMetadata{}, false
	}
	return MetricMetadata{
		MetricFamily: mfName,
		Type:         m.Type,
		Help:         m.Help,
		Unit:         m.Unit,
	}, true
}

// ListMetadata lists metadata.
func (c *scrapeCache) ListMetadata() []MetricMetadata {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	res := make([]MetricMetadata, 0, len(c.metadata))

	for m, e := range c.metadata {
		res = append(res, MetricMetadata{
			MetricFamily: m,
			Type:         e.Type,
			Help:         e.Help,
			Unit:         e.Unit,
		})
	}
	return res
}

// SizeMetadata returns the size of the metadata cache.
func (c *scrapeCache) SizeMetadata() (s int) {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()
	for _, e := range c.metadata {
		s += e.size()
	}

	return s
}

// LengthMetadata returns the number of metadata entries in the cache.
func (c *scrapeCache) LengthMetadata() int {
	c.metaMtx.Lock()
	defer c.metaMtx.Unlock()

	return len(c.metadata)
}

// scrapeLoopOptions contains static options that do not change per scrapePool lifecycle.
type scrapeLoopOptions struct {
	target            *Target
	scraper           scraper
	cache             *scrapeCache
	interval, timeout time.Duration

	sp *scrapePool
}

// newScrapeLoop constructs new scrapeLoop.
// NOTE: Technically this could be a scrapePool method, but it's a standalone function to make it clear scrapeLoop
// can be used outside scrapePool lifecycle (e.g. in tests).
func newScrapeLoop(opts scrapeLoopOptions) *scrapeLoop {
	// Update the targets retrieval function for metadata to a new target.
	opts.target.SetMetadataStore(opts.cache)

	appenderCtx := opts.sp.ctx
	if opts.sp.options.PassMetadataInContext {
		// Store the cache and target in the context. This is then used by downstream OTel Collector
		// to lookup the metadata required to process the samples. Not used by Prometheus itself.
		// TODO(gouthamve) We're using a dedicated context because using the parentCtx caused a memory
		// leak. We should ideally fix the main leak. See: https://github.com/prometheus/prometheus/pull/10590
		// TODO(bwplotka): Remove once OpenTelemetry collector uses AppenderV2 (add issue)
		appenderCtx = ContextWithMetricMetadataStore(appenderCtx, opts.cache)
		appenderCtx = ContextWithTarget(appenderCtx, opts.target)
	}

	ctx, cancel := context.WithCancel(opts.sp.ctx)
	return &scrapeLoop{
		ctx:         ctx,
		cancel:      cancel,
		stopped:     make(chan struct{}),
		parentCtx:   opts.sp.ctx,
		appenderCtx: appenderCtx,
		l:           opts.sp.logger.With("target", opts.target),
		cache:       opts.cache,

		interval: opts.interval,
		timeout:  opts.timeout,
		sampleMutator: func(l labels.Labels) labels.Labels {
			return mutateSampleLabels(l, opts.target, opts.sp.config.HonorLabels, opts.sp.config.MetricRelabelConfigs)
		},
		reportSampleMutator: func(l labels.Labels) labels.Labels { return mutateReportSampleLabels(l, opts.target) },
		scraper:             opts.scraper,

		// Static params per scrapePool.
		appendable:  opts.sp.appendable,
		buffers:     opts.sp.buffers,
		offsetSeed:  opts.sp.offsetSeed,
		symbolTable: opts.sp.symbolTable,
		metrics:     opts.sp.metrics,

		// config.ScrapeConfig.
		sampleLimit: int(opts.sp.config.SampleLimit),
		bucketLimit: int(opts.sp.config.NativeHistogramBucketLimit),
		maxSchema:   pickSchema(opts.sp.config.NativeHistogramMinBucketFactor),
		labelLimits: &labelLimits{
			labelLimit:            int(opts.sp.config.LabelLimit),
			labelNameLengthLimit:  int(opts.sp.config.LabelNameLengthLimit),
			labelValueLengthLimit: int(opts.sp.config.LabelValueLengthLimit),
		},
		honorLabels:                   opts.sp.config.HonorLabels,
		honorTimestamps:               opts.sp.config.HonorTimestamps,
		trackTimestampsStaleness:      opts.sp.config.TrackTimestampsStaleness,
		enableNativeHistogramScraping: opts.sp.config.ScrapeNativeHistogramsEnabled(),
		alwaysScrapeClassicHist:       opts.sp.config.AlwaysScrapeClassicHistogramsEnabled(),
		convertClassicHistToNHCB:      opts.sp.config.ConvertClassicHistogramsToNHCBEnabled(),
		fallbackScrapeProtocol:        opts.sp.config.ScrapeFallbackProtocol.HeaderMediaType(),
		enableCompression:             opts.sp.config.EnableCompression,
		mrc:                           opts.sp.config.MetricRelabelConfigs,
		reportExtraMetrics:            opts.sp.config.ExtraScrapeMetricsEnabled(),
		validationScheme:              opts.sp.config.MetricNameValidationScheme,

		// scrape.Options.
		enableSTZeroIngestion:   opts.sp.options.EnableStartTimestampZeroIngestion,
		enableTypeAndUnitLabels: opts.sp.options.EnableTypeAndUnitLabels,
		appendMetadataToWAL:     opts.sp.options.AppendMetadata,
		passMetadataInContext:   opts.sp.options.PassMetadataInContext,
		skipOffsetting:          opts.sp.options.skipOffsetting,
	}
}

func (sl *scrapeLoop) setScrapeFailureLogger(l FailureLogger) {
	sl.scrapeFailureLoggerMtx.Lock()
	defer sl.scrapeFailureLoggerMtx.Unlock()
	if ts, ok := sl.scraper.(fmt.Stringer); ok && l != nil {
		l = slog.New(l).With("target", ts.String()).Handler().(FailureLogger)
	}
	sl.scrapeFailureLogger = l
}

func (sl *scrapeLoop) run(errc chan<- error) {
	if !sl.skipOffsetting {
		select {
		case <-time.After(sl.scraper.offset(sl.interval, sl.offsetSeed)):
			// Continue after a scraping offset.
		case <-sl.ctx.Done():
			close(sl.stopped)
			return
		}
	}

	var last time.Time

	alignedScrapeTime := time.Now().Round(0)
	ticker := time.NewTicker(sl.interval)
	defer ticker.Stop()

mainLoop:
	for {
		select {
		case <-sl.parentCtx.Done():
			close(sl.stopped)
			return
		case <-sl.ctx.Done():
			break mainLoop
		default:
		}

		// Temporary workaround for a jitter in go timers that causes disk space
		// increase in TSDB.
		// See https://github.com/prometheus/prometheus/issues/7846
		// Calling Round ensures the time used is the wall clock, as otherwise .Sub
		// and .Add on time.Time behave differently (see time package docs).
		scrapeTime := time.Now().Round(0)
		if AlignScrapeTimestamps {
			// Tolerance is clamped to maximum 1% of the scrape interval.
			tolerance := min(sl.interval/100, ScrapeTimestampTolerance)
			// For some reason, a tick might have been skipped, in which case we
			// would call alignedScrapeTime.Add(interval) multiple times.
			for scrapeTime.Sub(alignedScrapeTime) >= sl.interval {
				alignedScrapeTime = alignedScrapeTime.Add(sl.interval)
			}
			// Align the scrape time if we are in the tolerance boundaries.
			if scrapeTime.Sub(alignedScrapeTime) <= tolerance {
				scrapeTime = alignedScrapeTime
			}
		}

		last = sl.scrapeAndReport(last, scrapeTime, errc)

		select {
		case <-sl.parentCtx.Done():
			close(sl.stopped)
			return
		case <-sl.ctx.Done():
			break mainLoop
		case <-ticker.C:
		}
	}

	close(sl.stopped)

	if !sl.disabledEndOfRunStalenessMarkers.Load() {
		sl.endOfRunStaleness(last, ticker, sl.interval)
	}
}

func (sl *scrapeLoop) appender() scrapeLoopAppendAdapter {
	// NOTE(bwplotka): Add AppenderV2 implementation, see https://github.com/prometheus/prometheus/issues/17632.
	return &scrapeLoopAppender{scrapeLoop: sl, Appender: sl.appendable.Appender(sl.appenderCtx)}
}

// scrapeAndReport performs a scrape and then appends the result to the storage
// together with reporting metrics, by using as few appenders as possible.
// In the happy scenario, a single appender is used.
// This function uses sl.appenderCtx instead of sl.ctx on purpose. A scrape should
// only be cancelled on shutdown, not on reloads.
func (sl *scrapeLoop) scrapeAndReport(last, appendTime time.Time, errc chan<- error) time.Time {
	start := time.Now()

	// Only record after the first scrape.
	if !last.IsZero() {
		sl.metrics.targetIntervalLength.WithLabelValues(sl.interval.String()).Observe(
			time.Since(last).Seconds(),
		)
		sl.metrics.targetIntervalLengthHistogram.WithLabelValues(sl.interval.String()).Observe(
			time.Since(last).Seconds(),
		)
	}

	var total, added, seriesAdded, bytesRead int
	var err, appErr, scrapeErr error

	app := sl.appender()
	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
		if err != nil {
			sl.l.Error("Scrape commit failed", "err", err)
		}
	}()

	defer func() {
		if err = sl.report(app, appendTime, time.Since(start), total, added, seriesAdded, bytesRead, scrapeErr); err != nil {
			sl.l.Warn("Appending scrape report failed", "err", err)
		}
	}()

	if forcedErr := sl.getForcedError(); forcedErr != nil {
		scrapeErr = forcedErr
		// Add stale markers.
		if _, _, _, err := app.append([]byte{}, "", appendTime); err != nil {
			_ = app.Rollback()
			app = sl.appender()
			sl.l.Warn("Append failed", "err", err)
		}
		if errc != nil {
			select {
			case errc <- forcedErr:
			case <-sl.ctx.Done():
			}
		}

		return start
	}

	var contentType string
	var resp *http.Response
	var b []byte
	var buf *bytes.Buffer
	scrapeCtx, cancel := context.WithTimeout(sl.parentCtx, sl.timeout)
	resp, scrapeErr = sl.scraper.scrape(scrapeCtx)
	if scrapeErr == nil {
		b = sl.buffers.Get(sl.lastScrapeSize).([]byte)
		defer sl.buffers.Put(b)
		buf = bytes.NewBuffer(b)
		contentType, scrapeErr = sl.scraper.readResponse(scrapeCtx, resp, buf)
	}
	cancel()

	if scrapeErr == nil {
		b = buf.Bytes()
		// NOTE: There were issues with misbehaving clients in the past
		// that occasionally returned empty results. We don't want those
		// to falsely reset our buffer size.
		if len(b) > 0 {
			sl.lastScrapeSize = len(b)
		}
		bytesRead = len(b)
	} else {
		sl.l.Debug("Scrape failed", "err", scrapeErr)
		sl.scrapeFailureLoggerMtx.RLock()
		if sl.scrapeFailureLogger != nil {
			slog.New(sl.scrapeFailureLogger).Error(scrapeErr.Error())
		}
		sl.scrapeFailureLoggerMtx.RUnlock()
		if errc != nil {
			select {
			case errc <- scrapeErr:
			case <-sl.ctx.Done():
			}
		}
		if errors.Is(scrapeErr, errBodySizeLimit) {
			bytesRead = -1
		}
	}

	// A failed scrape is the same as an empty scrape,
	// we still call sl.append to trigger stale markers.
	total, added, seriesAdded, appErr = app.append(b, contentType, appendTime)
	if appErr != nil {
		_ = app.Rollback()
		app = sl.appender()
		sl.l.Debug("Append failed", "err", appErr)
		// The append failed, probably due to a parse error or sample limit.
		// Call sl.append again with an empty scrape to trigger stale markers.
		if _, _, _, err := app.append([]byte{}, "", appendTime); err != nil {
			_ = app.Rollback()
			app = sl.appender()
			sl.l.Warn("Append failed", "err", err)
		}
	}

	if scrapeErr == nil {
		scrapeErr = appErr
	}

	return start
}

func (sl *scrapeLoop) setForcedError(err error) {
	sl.forcedErrMtx.Lock()
	defer sl.forcedErrMtx.Unlock()
	sl.forcedErr = err
}

func (sl *scrapeLoop) getForcedError() error {
	sl.forcedErrMtx.Lock()
	defer sl.forcedErrMtx.Unlock()
	return sl.forcedErr
}

func (sl *scrapeLoop) endOfRunStaleness(last time.Time, ticker *time.Ticker, interval time.Duration) {
	// Scraping has stopped. We want to write stale markers but
	// the target may be recreated, so we wait just over 2 scrape intervals
	// before creating them.
	// If the context is canceled, we presume the server is shutting down
	// and will restart where is was. We do not attempt to write stale markers
	// in this case.

	if last.IsZero() {
		// There never was a scrape, so there will be no stale markers.
		return
	}

	// Wait for when the next scrape would have been, record its timestamp.
	var staleTime time.Time
	select {
	case <-sl.parentCtx.Done():
		return
	case <-ticker.C:
		staleTime = time.Now()
	}

	// Wait for when the next scrape would have been, if the target was recreated
	// samples should have been ingested by now.
	select {
	case <-sl.parentCtx.Done():
		return
	case <-ticker.C:
	}

	// Wait for an extra 10% of the interval, just to be safe.
	select {
	case <-sl.parentCtx.Done():
		return
	case <-time.After(interval / 10):
	}

	// Check if end-of-run staleness markers have been disabled while we were waiting.
	if sl.disabledEndOfRunStalenessMarkers.Load() {
		return
	}

	// Call sl.append again with an empty scrape to trigger stale markers.
	// If the target has since been recreated and scraped, the
	// stale markers will be out of order and ignored.
	// sl.context would have been cancelled, hence using sl.appenderCtx.
	app := sl.appender()
	var err error
	defer func() {
		if err != nil {
			_ = app.Rollback()
			return
		}
		err = app.Commit()
		if err != nil {
			sl.l.Warn("Stale commit failed", "err", err)
		}
	}()
	if _, _, _, err = app.append([]byte{}, "", staleTime); err != nil {
		_ = app.Rollback()
		app = sl.appender()
		sl.l.Warn("Stale append failed", "err", err)
	}
	if err = sl.reportStale(app, staleTime); err != nil {
		sl.l.Warn("Stale report failed", "err", err)
	}
}

// Stop the scraping. May still write data and stale markers after it has
// returned. Cancel the context to stop all writes.
func (sl *scrapeLoop) stop() {
	sl.cancel()
	<-sl.stopped
}

func (sl *scrapeLoop) disableEndOfRunStalenessMarkers() {
	sl.disabledEndOfRunStalenessMarkers.Store(true)
}

func (sl *scrapeLoop) getCache() *scrapeCache {
	return sl.cache
}

type appendErrors struct {
	numOutOfOrder         int
	numDuplicates         int
	numOutOfBounds        int
	numExemplarOutOfOrder int
}

// Update the stale markers.
func (sl *scrapeLoop) updateStaleMarkers(app storage.Appender, defTime int64) (err error) {
	sl.cache.forEachStale(func(ref storage.SeriesRef, lset labels.Labels) bool {
		// Series no longer exposed, mark it stale.
		app.SetOptions(&aOptionRejectEarlyOOO)
		_, err = app.Append(ref, lset, defTime, math.Float64frombits(value.StaleNaN))
		app.SetOptions(nil)
		switch {
		case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
			// Do not count these in logging, as this is expected if a target
			// goes away and comes back again with a new scrape loop.
			err = nil
		}
		return err == nil
	})
	return err
}

type scrapeLoopAppender struct {
	*scrapeLoop

	storage.Appender
}

var _ scrapeLoopAppendAdapter = &scrapeLoopAppender{}

func (sl *scrapeLoopAppender) append(b []byte, contentType string, ts time.Time) (total, added, seriesAdded int, err error) {
	defTime := timestamp.FromTime(ts)

	if len(b) == 0 {
		// Empty scrape. Just update the stale makers and swap the cache (but don't flush it).
		err = sl.updateStaleMarkers(sl.Appender, defTime)
		sl.cache.iterDone(false)
		return total, added, seriesAdded, err
	}

	p, err := textparse.New(b, contentType, sl.symbolTable, textparse.ParserOptions{
		EnableTypeAndUnitLabels:                 sl.enableTypeAndUnitLabels,
		IgnoreNativeHistograms:                  !sl.enableNativeHistogramScraping,
		ConvertClassicHistogramsToNHCB:          sl.convertClassicHistToNHCB,
		KeepClassicOnClassicAndNativeHistograms: sl.alwaysScrapeClassicHist,
		OpenMetricsSkipSTSeries:                 sl.enableSTZeroIngestion,
		FallbackContentType:                     sl.fallbackScrapeProtocol,
	})
	if p == nil {
		sl.l.Error(
			"Failed to determine correct type of scrape target.",
			"content_type", contentType,
			"fallback_media_type", sl.fallbackScrapeProtocol,
			"err", err,
		)
		return total, added, seriesAdded, err
	}
	if err != nil {
		sl.l.Debug(
			"Invalid content type on scrape, using fallback setting.",
			"content_type", contentType,
			"fallback_media_type", sl.fallbackScrapeProtocol,
			"err", err,
		)
	}
	var (
		appErrs        = appendErrors{}
		sampleLimitErr error
		bucketLimitErr error
		lset           labels.Labels     // Escapes to heap so hoisted out of loop.
		e              exemplar.Exemplar // Escapes to heap so hoisted out of loop.
		lastMeta       *metaEntry
		lastMFName     []byte
	)

	exemplars := make([]exemplar.Exemplar, 0, 1)

	// Take an appender with limits.
	app := appenderWithLimits(sl.Appender, sl.sampleLimit, sl.bucketLimit, sl.maxSchema)

	defer func() {
		if err != nil {
			return
		}
		// Flush and swap the cache as the scrape was non-empty.
		sl.cache.iterDone(true)
	}()

loop:
	for {
		var (
			et                       textparse.Entry
			sampleAdded, isHistogram bool
			met                      []byte
			parsedTimestamp          *int64
			val                      float64
			h                        *histogram.Histogram
			fh                       *histogram.FloatHistogram
		)
		if et, err = p.Next(); err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			break
		}
		switch et {
		// TODO(bwplotka): Consider changing parser to give metadata at once instead of type, help and unit in separation, ideally on `Series()/Histogram()
		// otherwise we can expose metadata without series on metadata API.
		case textparse.EntryType:
			// TODO(bwplotka): Build meta entry directly instead of locking and updating the map. This will
			// allow to properly update metadata when e.g unit was added, then removed;
			lastMFName, lastMeta = sl.cache.setType(p.Type())
			continue
		case textparse.EntryHelp:
			lastMFName, lastMeta = sl.cache.setHelp(p.Help())
			continue
		case textparse.EntryUnit:
			lastMFName, lastMeta = sl.cache.setUnit(p.Unit())
			continue
		case textparse.EntryComment:
			continue
		case textparse.EntryHistogram:
			isHistogram = true
		default:
		}
		total++

		t := defTime
		if isHistogram {
			met, parsedTimestamp, h, fh = p.Histogram()
		} else {
			met, parsedTimestamp, val = p.Series()
		}
		if !sl.honorTimestamps {
			parsedTimestamp = nil
		}
		if parsedTimestamp != nil {
			t = *parsedTimestamp
		}

		if sl.cache.getDropped(met) {
			continue
		}
		ce, seriesCached, seriesAlreadyScraped := sl.cache.get(met)
		var (
			ref  storage.SeriesRef
			hash uint64
		)

		if seriesCached {
			ref = ce.ref
			lset = ce.lset
			hash = ce.hash
		} else {
			p.Labels(&lset)
			hash = lset.Hash()

			// Hash label set as it is seen local to the target. Then add target labels
			// and relabeling and store the final label set.
			lset = sl.sampleMutator(lset)

			// The label set may be set to empty to indicate dropping.
			if lset.IsEmpty() {
				sl.cache.addDropped(met)
				continue
			}

			if !lset.Has(model.MetricNameLabel) {
				err = errNameLabelMandatory
				break loop
			}
			if !lset.IsValid(sl.validationScheme) {
				err = fmt.Errorf("invalid metric name or label names: %s", lset.String())
				break loop
			}

			// If any label limits is exceeded the scrape should fail.
			if err = verifyLabelLimits(lset, sl.labelLimits); err != nil {
				sl.metrics.targetScrapePoolExceededLabelLimits.Inc()
				break loop
			}
		}

		if seriesAlreadyScraped && parsedTimestamp == nil {
			err = storage.ErrDuplicateSampleForTimestamp
		} else {
			if sl.enableSTZeroIngestion {
				if stMs := p.StartTimestamp(); stMs != 0 {
					if isHistogram {
						if h != nil {
							ref, err = app.AppendHistogramSTZeroSample(ref, lset, t, stMs, h, nil)
						} else {
							ref, err = app.AppendHistogramSTZeroSample(ref, lset, t, stMs, nil, fh)
						}
					} else {
						ref, err = app.AppendSTZeroSample(ref, lset, t, stMs)
					}
					if err != nil && !errors.Is(err, storage.ErrOutOfOrderST) { // OOO is a common case, ignoring completely for now.
						// ST is an experimental feature. For now, we don't need to fail the
						// scrape on errors updating the created timestamp, log debug.
						sl.l.Debug("Error when appending ST in scrape loop", "series", string(met), "ct", stMs, "t", t, "err", err)
					}
				}
			}

			if isHistogram {
				if h != nil {
					ref, err = app.AppendHistogram(ref, lset, t, h, nil)
				} else {
					ref, err = app.AppendHistogram(ref, lset, t, nil, fh)
				}
			} else {
				ref, err = app.Append(ref, lset, t, val)
			}
		}

		if err == nil {
			if (parsedTimestamp == nil || sl.trackTimestampsStaleness) && ce != nil {
				sl.cache.trackStaleness(ce.ref, ce)
			}
		}

		sampleAdded, err = sl.checkAddError(met, err, &sampleLimitErr, &bucketLimitErr, &appErrs)
		if err != nil {
			if !errors.Is(err, storage.ErrNotFound) {
				sl.l.Debug("Unexpected error", "series", string(met), "err", err)
			}
			break loop
		}

		// If series wasn't cached (is new, not seen on previous scrape) we need need to add it to the scrape cache.
		// But we only do this for series that were appended to TSDB without errors.
		// If a series was new but we didn't append it due to sample_limit or other errors then we don't need
		// it in the scrape cache because we don't need to emit StaleNaNs for it when it disappears.
		if !seriesCached && sampleAdded {
			ce = sl.cache.addRef(met, ref, lset, hash)
			if ce != nil && (parsedTimestamp == nil || sl.trackTimestampsStaleness) {
				// Bypass staleness logic if there is an explicit timestamp.
				// But make sure we only do this if we have a cache entry (ce) for our series.
				sl.cache.trackStaleness(ref, ce)
			}
			if sampleLimitErr == nil && bucketLimitErr == nil {
				seriesAdded++
			}
		}

		// Increment added even if there's an error so we correctly report the
		// number of samples remaining after relabeling.
		// We still report duplicated samples here since this number should be the exact number
		// of time series exposed on a scrape after relabelling.
		added++
		exemplars = exemplars[:0] // Reset and reuse the exemplar slice.
		for hasExemplar := p.Exemplar(&e); hasExemplar; hasExemplar = p.Exemplar(&e) {
			if !e.HasTs {
				if isHistogram {
					// We drop exemplars for native histograms if they don't have a timestamp.
					// Missing timestamps are deliberately not supported as we want to start
					// enforcing timestamps for exemplars as otherwise proper deduplication
					// is inefficient and purely based on heuristics: we cannot distinguish
					// between repeated exemplars and new instances with the same values.
					// This is done silently without logs as it is not an error but out of spec.
					// This does not affect classic histograms so that behaviour is unchanged.
					e = exemplar.Exemplar{} // Reset for next time round loop.
					continue
				}
				e.Ts = t
			}
			exemplars = append(exemplars, e)
			e = exemplar.Exemplar{} // Reset for next time round loop.
		}
		// Sort so that checking for duplicates / out of order is more efficient during validation.
		slices.SortFunc(exemplars, exemplar.Compare)
		outOfOrderExemplars := 0
		for _, e := range exemplars {
			_, exemplarErr := app.AppendExemplar(ref, lset, e)
			switch {
			case exemplarErr == nil:
				// Do nothing.
			case errors.Is(exemplarErr, storage.ErrOutOfOrderExemplar):
				outOfOrderExemplars++
			default:
				// Since exemplar storage is still experimental, we don't fail the scrape on ingestion errors.
				sl.l.Debug("Error while adding exemplar in AddExemplar", "exemplar", fmt.Sprintf("%+v", e), "err", exemplarErr)
			}
		}
		if outOfOrderExemplars > 0 && outOfOrderExemplars == len(exemplars) {
			// Only report out of order exemplars if all are out of order, otherwise this was a partial update
			// to some existing set of exemplars.
			appErrs.numExemplarOutOfOrder += outOfOrderExemplars
			sl.l.Debug("Out of order exemplars", "count", outOfOrderExemplars, "latest", fmt.Sprintf("%+v", exemplars[len(exemplars)-1]))
			sl.metrics.targetScrapeExemplarOutOfOrder.Add(float64(outOfOrderExemplars))
		}

		if sl.appendMetadataToWAL && lastMeta != nil {
			// Is it new series OR did metadata change for this family?
			if !seriesCached || lastMeta.lastIterChange == sl.cache.iter {
				// In majority cases we can trust that the current series/histogram is matching the lastMeta and lastMFName.
				// However, optional TYPE etc metadata and broken OM text can break this, detect those cases here.
				// TODO(bwplotka): Consider moving this to parser as many parser users end up doing this (e.g. ST and NHCB parsing).
				if isSeriesPartOfFamily(lset.Get(model.MetricNameLabel), lastMFName, lastMeta.Type) {
					if _, merr := app.UpdateMetadata(ref, lset, lastMeta.Metadata); merr != nil {
						// No need to fail the scrape on errors appending metadata.
						sl.l.Debug("Error when appending metadata in scrape loop", "ref", fmt.Sprintf("%d", ref), "metadata", fmt.Sprintf("%+v", lastMeta.Metadata), "err", merr)
					}
				}
			}
		}
	}
	if sampleLimitErr != nil {
		if err == nil {
			err = sampleLimitErr
		}
		// We only want to increment this once per scrape, so this is Inc'd outside the loop.
		sl.metrics.targetScrapeSampleLimit.Inc()
	}
	if bucketLimitErr != nil {
		if err == nil {
			err = bucketLimitErr // If sample limit is hit, that error takes precedence.
		}
		// We only want to increment this once per scrape, so this is Inc'd outside the loop.
		sl.metrics.targetScrapeNativeHistogramBucketLimit.Inc()
	}
	if appErrs.numOutOfOrder > 0 {
		sl.l.Warn("Error on ingesting out-of-order samples", "num_dropped", appErrs.numOutOfOrder)
	}
	if appErrs.numDuplicates > 0 {
		sl.l.Warn("Error on ingesting samples with different value but same timestamp", "num_dropped", appErrs.numDuplicates)
	}
	if appErrs.numOutOfBounds > 0 {
		sl.l.Warn("Error on ingesting samples that are too old or are too far into the future", "num_dropped", appErrs.numOutOfBounds)
	}
	if appErrs.numExemplarOutOfOrder > 0 {
		sl.l.Warn("Error on ingesting out-of-order exemplars", "num_dropped", appErrs.numExemplarOutOfOrder)
	}
	if err == nil {
		err = sl.updateStaleMarkers(app, defTime)
	}
	return total, added, seriesAdded, err
}

func isSeriesPartOfFamily(mName string, mfName []byte, typ model.MetricType) bool {
	mfNameStr := yoloString(mfName)
	if !strings.HasPrefix(mName, mfNameStr) { // Fast path.
		return false
	}

	var (
		gotMFName string
		ok        bool
	)
	switch typ {
	case model.MetricTypeCounter:
		// Prometheus allows _total, cut it from mf name to support this case.
		mfNameStr, _ = strings.CutSuffix(mfNameStr, "_total")

		gotMFName, ok = strings.CutSuffix(mName, "_total")
		if !ok {
			gotMFName = mName
		}
	case model.MetricTypeHistogram:
		gotMFName, ok = strings.CutSuffix(mName, "_bucket")
		if !ok {
			gotMFName, ok = strings.CutSuffix(mName, "_sum")
			if !ok {
				gotMFName, ok = strings.CutSuffix(mName, "_count")
				if !ok {
					gotMFName = mName
				}
			}
		}
	case model.MetricTypeGaugeHistogram:
		gotMFName, ok = strings.CutSuffix(mName, "_bucket")
		if !ok {
			gotMFName, ok = strings.CutSuffix(mName, "_gsum")
			if !ok {
				gotMFName, ok = strings.CutSuffix(mName, "_gcount")
				if !ok {
					gotMFName = mName
				}
			}
		}
	case model.MetricTypeSummary:
		gotMFName, ok = strings.CutSuffix(mName, "_sum")
		if !ok {
			gotMFName, ok = strings.CutSuffix(mName, "_count")
			if !ok {
				gotMFName = mName
			}
		}
	case model.MetricTypeInfo:
		// Technically prometheus text does not support info type, but we might
		// accidentally allow info type in prom parse, so support metric family names
		// with the _info explicitly too.
		mfNameStr, _ = strings.CutSuffix(mfNameStr, "_info")

		gotMFName, ok = strings.CutSuffix(mName, "_info")
		if !ok {
			gotMFName = mName
		}
	default:
		gotMFName = mName
	}
	return mfNameStr == gotMFName
}

// Adds samples to the appender, checking the error, and then returns the # of samples added,
// whether the caller should continue to process more samples, and any sample or bucket limit errors.
// Switch error cases for Sample and Bucket limits are checked first since they're more common
// during normal operation (e.g., accidental cardinality explosion, sudden traffic spikes).
// Current case ordering prevents exercising other cases when limits are exceeded.
// Remaining error cases typically occur only a few times, often during initial setup.
func (sl *scrapeLoop) checkAddError(met []byte, err error, sampleLimitErr, bucketLimitErr *error, appErrs *appendErrors) (sampleAdded bool, _ error) {
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errSampleLimit):
		// Keep on parsing output if we hit the limit, so we report the correct
		// total number of samples scraped.
		*sampleLimitErr = err
		return false, nil
	case errors.Is(err, errBucketLimit):
		// Keep on parsing output if we hit the limit, so we report the bucket
		// total number of samples scraped.
		*bucketLimitErr = err
		return false, nil
	case errors.Is(err, storage.ErrOutOfOrderSample):
		appErrs.numOutOfOrder++
		sl.l.Debug("Out of order sample", "series", string(met))
		sl.metrics.targetScrapeSampleOutOfOrder.Inc()
		return false, nil
	case errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
		appErrs.numDuplicates++
		sl.l.Debug("Duplicate sample for timestamp", "series", string(met))
		sl.metrics.targetScrapeSampleDuplicate.Inc()
		return false, nil
	case errors.Is(err, storage.ErrOutOfBounds):
		appErrs.numOutOfBounds++
		sl.l.Debug("Out of bounds metric", "series", string(met))
		sl.metrics.targetScrapeSampleOutOfBounds.Inc()
		return false, nil
	case errors.Is(err, storage.ErrNotFound):
		return false, storage.ErrNotFound
	default:
		return false, err
	}
}

// reportSample represents automatically generated timeseries documented in
// https://prometheus.io/docs/concepts/jobs_instances/#automatically-generated-labels-and-time-series
type reportSample struct {
	metadata.Metadata
	name []byte
}

// The constants are suffixed with the invalid \xff unicode rune to avoid collisions
// with scraped metrics in the cache.
var (
	scrapeHealthMetric = reportSample{
		name: []byte("up" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "Health of the scrape target. 1 means the target is healthy, 0 if the scrape failed.",
			Unit: "targets",
		},
	}
	scrapeDurationMetric = reportSample{
		name: []byte("scrape_duration_seconds" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "Duration of the last scrape in seconds.",
			Unit: "seconds",
		},
	}
	scrapeSamplesMetric = reportSample{
		name: []byte("scrape_samples_scraped" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "Number of samples last scraped.",
			Unit: "samples",
		},
	}
	samplesPostRelabelMetric = reportSample{
		name: []byte("scrape_samples_post_metric_relabeling" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "Number of samples remaining after metric relabeling was applied.",
			Unit: "samples",
		},
	}
	scrapeSeriesAddedMetric = reportSample{
		name: []byte("scrape_series_added" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "Number of series in the last scrape.",
			Unit: "series",
		},
	}
	scrapeTimeoutMetric = reportSample{
		name: []byte("scrape_timeout_seconds" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "The configured scrape timeout for a target.",
			Unit: "seconds",
		},
	}
	scrapeSampleLimitMetric = reportSample{
		name: []byte("scrape_sample_limit" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "The configured sample limit for a target. Returns zero if there is no limit configured.",
			Unit: "samples",
		},
	}
	scrapeBodySizeBytesMetric = reportSample{
		name: []byte("scrape_body_size_bytes" + "\xff"),
		Metadata: metadata.Metadata{
			Type: model.MetricTypeGauge,
			Help: "The uncompressed size of the last scrape response, if successful. Scrapes failing because body_size_limit is exceeded report -1, other scrape failures report 0.",
			Unit: "bytes",
		},
	}
)

func (sl *scrapeLoop) report(app scrapeLoopAppendAdapter, start time.Time, duration time.Duration, scraped, added, seriesAdded, bytes int, scrapeErr error) (err error) {
	sl.scraper.Report(start, duration, scrapeErr)

	ts := timestamp.FromTime(start)

	var health float64
	if scrapeErr == nil {
		health = 1
	}
	b := labels.NewBuilderWithSymbolTable(sl.symbolTable)

	if err = app.addReportSample(scrapeHealthMetric, ts, health, b, false); err != nil {
		return err
	}
	if err = app.addReportSample(scrapeDurationMetric, ts, duration.Seconds(), b, false); err != nil {
		return err
	}
	if err = app.addReportSample(scrapeSamplesMetric, ts, float64(scraped), b, false); err != nil {
		return err
	}
	if err = app.addReportSample(samplesPostRelabelMetric, ts, float64(added), b, false); err != nil {
		return err
	}
	if err = app.addReportSample(scrapeSeriesAddedMetric, ts, float64(seriesAdded), b, false); err != nil {
		return err
	}
	if sl.reportExtraMetrics {
		if err = app.addReportSample(scrapeTimeoutMetric, ts, sl.timeout.Seconds(), b, false); err != nil {
			return err
		}
		if err = app.addReportSample(scrapeSampleLimitMetric, ts, float64(sl.sampleLimit), b, false); err != nil {
			return err
		}
		if err = app.addReportSample(scrapeBodySizeBytesMetric, ts, float64(bytes), b, false); err != nil {
			return err
		}
	}
	return err
}

func (sl *scrapeLoop) reportStale(app scrapeLoopAppendAdapter, start time.Time) (err error) {
	ts := timestamp.FromTime(start)
	stale := math.Float64frombits(value.StaleNaN)
	b := labels.NewBuilder(labels.EmptyLabels())

	if err = app.addReportSample(scrapeHealthMetric, ts, stale, b, true); err != nil {
		return err
	}
	if err = app.addReportSample(scrapeDurationMetric, ts, stale, b, true); err != nil {
		return err
	}
	if err = app.addReportSample(scrapeSamplesMetric, ts, stale, b, true); err != nil {
		return err
	}
	if err = app.addReportSample(samplesPostRelabelMetric, ts, stale, b, true); err != nil {
		return err
	}
	if err = app.addReportSample(scrapeSeriesAddedMetric, ts, stale, b, true); err != nil {
		return err
	}
	if sl.reportExtraMetrics {
		if err = app.addReportSample(scrapeTimeoutMetric, ts, stale, b, true); err != nil {
			return err
		}
		if err = app.addReportSample(scrapeSampleLimitMetric, ts, stale, b, true); err != nil {
			return err
		}
		if err = app.addReportSample(scrapeBodySizeBytesMetric, ts, stale, b, true); err != nil {
			return err
		}
	}
	return err
}

func (sl *scrapeLoopAppender) addReportSample(s reportSample, t int64, v float64, b *labels.Builder, rejectOOO bool) (err error) {
	ce, ok, _ := sl.cache.get(s.name)
	var ref storage.SeriesRef
	var lset labels.Labels
	if ok {
		ref = ce.ref
		lset = ce.lset
	} else {
		// The constants are suffixed with the invalid \xff unicode rune to avoid collisions
		// with scraped metrics in the cache.
		// We have to drop it when building the actual metric.
		b.Reset(labels.EmptyLabels())
		b.Set(model.MetricNameLabel, string(s.name[:len(s.name)-1]))
		lset = sl.reportSampleMutator(b.Labels())
	}

	// This will be improved in AppenderV2.
	if rejectOOO {
		sl.SetOptions(&aOptionRejectEarlyOOO)
		ref, err = sl.Append(ref, lset, t, v)
		sl.SetOptions(nil)
	} else {
		ref, err = sl.Append(ref, lset, t, v)
	}

	switch {
	case err == nil:
		if !ok {
			sl.cache.addRef(s.name, ref, lset, lset.Hash())
			// We only need to add metadata once a scrape target appears.
			if sl.appendMetadataToWAL {
				if _, merr := sl.UpdateMetadata(ref, lset, s.Metadata); merr != nil {
					sl.l.Debug("Error when appending metadata in addReportSample", "ref", fmt.Sprintf("%d", ref), "metadata", fmt.Sprintf("%+v", s.Metadata), "err", merr)
				}
			}
		}
		return nil
	case errors.Is(err, storage.ErrOutOfOrderSample), errors.Is(err, storage.ErrDuplicateSampleForTimestamp):
		// Do not log here, as this is expected if a target goes away and comes back
		// again with a new scrape loop.
		return nil
	default:
		return err
	}
}

// zeroConfig returns a new scrape config that only contains configuration items
// that alter metrics.
func zeroConfig(c *config.ScrapeConfig) *config.ScrapeConfig {
	z := *c
	// We zero out the fields that for sure don't affect scrape.
	z.ScrapeInterval = 0
	z.ScrapeTimeout = 0
	z.SampleLimit = 0
	z.HTTPClientConfig = config_util.HTTPClientConfig{}
	return &z
}

// reusableCache compares two scrape config and tells whether the cache is still
// valid.
func reusableCache(r, l *config.ScrapeConfig) bool {
	if r == nil || l == nil {
		return false
	}
	return reflect.DeepEqual(zeroConfig(r), zeroConfig(l))
}

// CtxKey is a dedicated type for keys of context-embedded values propagated
// with the scrape context.
type ctxKey int

// Valid CtxKey values.
const (
	ctxKeyMetadata ctxKey = iota + 1
	ctxKeyTarget
)

func ContextWithMetricMetadataStore(ctx context.Context, s MetricMetadataStore) context.Context {
	return context.WithValue(ctx, ctxKeyMetadata, s)
}

func MetricMetadataStoreFromContext(ctx context.Context) (MetricMetadataStore, bool) {
	s, ok := ctx.Value(ctxKeyMetadata).(MetricMetadataStore)
	return s, ok
}

func ContextWithTarget(ctx context.Context, t *Target) context.Context {
	return context.WithValue(ctx, ctxKeyTarget, t)
}

func TargetFromContext(ctx context.Context) (*Target, bool) {
	t, ok := ctx.Value(ctxKeyTarget).(*Target)
	return t, ok
}

func pickSchema(bucketFactor float64) int32 {
	if bucketFactor <= 1 {
		bucketFactor = 1.00271
	}
	floor := math.Floor(-math.Log2(math.Log2(bucketFactor)))
	switch {
	case floor >= float64(histogram.ExponentialSchemaMax):
		return histogram.ExponentialSchemaMax
	case floor <= float64(histogram.ExponentialSchemaMin):
		return histogram.ExponentialSchemaMin
	default:
		return int32(floor)
	}
}

func newScrapeClient(cfg config_util.HTTPClientConfig, name string, optFuncs ...config_util.HTTPClientOption) (*http.Client, error) {
	client, err := config_util.NewClientFromConfig(cfg, name, optFuncs...)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}
	client.Transport = otelhttp.NewTransport(
		client.Transport,
		otelhttp.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
			return otelhttptrace.NewClientTrace(ctx, otelhttptrace.WithoutSubSpans())
		}))
	return client, nil
}
