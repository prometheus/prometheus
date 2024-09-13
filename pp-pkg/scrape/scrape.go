package scrape

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	op_model "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"golang.org/x/exp/slices"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/pool"
)

// ScrapeTimestampTolerance is the tolerance for scrape appends timestamps
// alignment, to enable better compression at the TSDB level.
// See https://github.com/prometheus/prometheus/issues/7846
var ScrapeTimestampTolerance = 2 * time.Millisecond

// AlignScrapeTimestamps enables the tolerance for scrape appends timestamps described above.
var AlignScrapeTimestamps = true

// scrapePool manages scrapes for sets of targets.
type scrapePool struct {
	receiver Receiver
	logger   log.Logger
	cancel   context.CancelFunc
	httpOpts []config_util.HTTPClientOption

	// mtx must not be taken after targetMtx.
	mtx    sync.Mutex
	config *config.ScrapeConfig
	client *http.Client
	loops  map[uint64]loop

	targetMtx sync.Mutex
	// activeTargets and loops must always be synchronized to have the same
	// set of hashes.
	activeTargets       map[uint64]*Target
	droppedTargets      []*Target // Subject to KeepDroppedTargets limit.
	droppedTargetsCount int       // Count of all dropped targets.

	// Constructor for new scrape loops. This is settable for testing convenience.
	newLoop func(scrapeLoopOptions) loop

	noDefaultPort bool

	metrics *scrapeMetrics

	bufferBuilders *buildersPool
	bufferBatches  *batchesPool
}

type scrapeLoopOptions struct {
	target                   *Target
	scraper                  scraper
	metricLimits             *cppbridge.MetricLimits
	honorLabels              bool
	honorTimestamps          bool
	trackTimestampsStaleness bool
	interval                 time.Duration
	timeout                  time.Duration
	scrapeClassicHistograms  bool

	mrc []*relabel.Config
	// cache             *scrapeCache
	enableCompression bool
}

// inject scrape labels.
type labelsInjector func(builder *op_model.LabelSetSimpleBuilder)

func newScrapePool(
	cfg *config.ScrapeConfig,
	receiver Receiver,
	offsetSeed uint64,
	logger log.Logger,
	buffers *pool.Pool,
	options *Options,
	metrics *scrapeMetrics,
) (*scrapePool, error) {
	scrapeName := config.ScrapePrefix + cfg.JobName
	if !receiver.RelabelerIDIsExist(scrapeName) {
		return nil, fmt.Errorf("relabeler id not found for scrape name: %s", scrapeName)
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}

	client, err := config_util.NewClientFromConfig(cfg.HTTPClientConfig, cfg.JobName, options.HTTPClientOptions...)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sp := &scrapePool{
		cancel:        cancel,
		receiver:      receiver,
		config:        cfg,
		client:        client,
		activeTargets: map[uint64]*Target{},
		loops:         map[uint64]loop{},
		logger:        logger,
		metrics:       metrics,
		httpOpts:      options.HTTPClientOptions,
		noDefaultPort: options.NoDefaultPort,

		bufferBuilders: newBuildersPool(),
		bufferBatches:  newbatchesPool(),
	}
	sp.newLoop = func(opts scrapeLoopOptions) loop {
		// PP_CHANGES.md: override sample limit with annotation
		limit := opts.target.SampleLimit()
		if limit != 0 {
			opts.metricLimits.SampleLimit = int64(limit)
		}

		return newScrapeLoop(
			ctx,
			opts.scraper,
			log.With(logger, "target", opts.target),
			buffers,
			sp.bufferBuilders,
			sp.bufferBatches,
			func(builder *op_model.LabelSetSimpleBuilder) {
				injectSampleLabels(builder, opts.target, opts.honorLabels)
			},
			func(builder *op_model.LabelSetSimpleBuilder) {
				injectReportSampleLabels(builder, opts.target)
			},
			receiver,
			config.ScrapePrefix+cfg.JobName,
			offsetSeed,
			opts.honorTimestamps,
			opts.trackTimestampsStaleness,
			opts.enableCompression,
			opts.metricLimits,
			opts.interval,
			opts.timeout,
			opts.scrapeClassicHistograms,
			options.EnableCreatedTimestampZeroIngestion,
			options.ExtraMetrics,
			options.EnableMetadataStorage,
			opts.target,
			options.PassMetadataInContext,
			metrics,
			options.skipOffsetting,
		)
	}
	sp.metrics.targetScrapePoolTargetLimit.WithLabelValues(sp.config.JobName).Set(float64(sp.config.TargetLimit))
	return sp, nil
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
		sp.metrics.targetSyncIntervalLength.DeleteLabelValues(sp.config.JobName)
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

	client, err := config_util.NewClientFromConfig(cfg.HTTPClientConfig, cfg.JobName, sp.httpOpts...)
	if err != nil {
		sp.metrics.targetScrapePoolReloadsFailed.Inc()
		return fmt.Errorf("error creating HTTP client: %w", err)
	}

	// reuseCache := reusableCache(sp.config, cfg)
	sp.config = cfg
	oldClient := sp.client
	sp.client = client

	sp.metrics.targetScrapePoolTargetLimit.WithLabelValues(sp.config.JobName).Set(float64(sp.config.TargetLimit))

	var (
		wg            sync.WaitGroup
		interval      = time.Duration(sp.config.ScrapeInterval)
		timeout       = time.Duration(sp.config.ScrapeTimeout)
		bodySizeLimit = int64(sp.config.BodySizeLimit)
		metricLimits  = &cppbridge.MetricLimits{
			LabelLimit:            int64(sp.config.LabelLimit),
			LabelNameLengthLimit:  int64(sp.config.LabelNameLengthLimit),
			LabelValueLengthLimit: int64(sp.config.LabelValueLengthLimit),
			SampleLimit:           int64(sp.config.SampleLimit),
		}
		honorLabels              = sp.config.HonorLabels
		honorTimestamps          = sp.config.HonorTimestamps
		enableCompression        = sp.config.EnableCompression
		trackTimestampsStaleness = sp.config.TrackTimestampsStaleness
		mrc                      = sp.config.MetricRelabelConfigs
	)

	sp.targetMtx.Lock()

	forcedErr := sp.refreshTargetLimitErr()
	for fp, oldLoop := range sp.loops {
		// var cache *scrapeCache
		// if oc := oldLoop.getCache(); reuseCache && oc != nil {
		// 	oldLoop.disableEndOfRunStalenessMarkers()
		// 	cache = oc
		// } else {
		// 	cache = newScrapeCache(sp.metrics)
		// }

		t := sp.activeTargets[fp]
		interval, timeout, err := t.intervalAndTimeout(interval, timeout)
		var (
			s = &targetScraper{
				Target:               t,
				sourceStates:         relabeler.NewSourceStates(),
				client:               sp.client,
				timeout:              timeout,
				bodySizeLimit:        bodySizeLimit,
				acceptHeader:         acceptHeader(cfg.ScrapeProtocols),
				acceptEncodingHeader: acceptEncodingHeader(enableCompression),
			}
			newLoop = sp.newLoop(scrapeLoopOptions{
				target:                   t,
				scraper:                  s,
				metricLimits:             metricLimits,
				honorLabels:              honorLabels,
				honorTimestamps:          honorTimestamps,
				enableCompression:        enableCompression,
				trackTimestampsStaleness: trackTimestampsStaleness,
				mrc:                      mrc,
				// cache:                    cache,
				interval: interval,
				timeout:  timeout,
			})
		)
		if err != nil {
			newLoop.setForcedError(err)
		}
		wg.Add(1)

		go func(oldLoop, newLoop loop) {
			oldLoop.stop()
			wg.Done()

			newLoop.setForcedError(forcedErr)
			newLoop.run(nil)
		}(oldLoop, newLoop)

		sp.loops[fp] = newLoop
	}

	sp.targetMtx.Unlock()

	wg.Wait()
	oldClient.CloseIdleConnections()
	sp.metrics.targetReloadIntervalLength.WithLabelValues(interval.String()).Observe(
		time.Since(start).Seconds(),
	)
	return nil
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
	lb := labels.NewBuilder(labels.EmptyLabels())
	sp.droppedTargets = []*Target{}
	sp.droppedTargetsCount = 0
	for _, tg := range tgs {
		targets, failures := TargetsFromGroup(tg, sp.config, sp.noDefaultPort, targets, lb)
		for _, err := range failures {
			level.Error(sp.logger).Log("msg", "Creating target failed", "err", err)
		}
		sp.metrics.targetSyncFailed.WithLabelValues(sp.config.JobName).Add(float64(len(failures)))
		for _, t := range targets {
			// Replicate .Labels().IsEmpty() with a loop here to avoid generating garbage.
			nonEmpty := false
			t.LabelsRange(func(l labels.Label) { nonEmpty = true })
			switch {
			case nonEmpty:
				all = append(all, t)
			case !t.discoveredLabels.IsEmpty():
				if sp.config.KeepDroppedTargets == 0 || uint(len(sp.droppedTargets)) < sp.config.KeepDroppedTargets {
					sp.droppedTargets = append(sp.droppedTargets, t)
				}
				sp.droppedTargetsCount++
			}
		}
	}
	sp.targetMtx.Unlock()
	sp.sync(all)

	sp.metrics.targetSyncIntervalLength.WithLabelValues(sp.config.JobName).Observe(
		time.Since(start).Seconds(),
	)
	sp.metrics.targetScrapePoolSyncsCounter.WithLabelValues(sp.config.JobName).Inc()
}

// sync takes a list of potentially duplicated targets, deduplicates them, starts
// scrape loops for new targets, and stops scrape loops for disappeared targets.
// It returns after all stopped scrape loops terminated.
func (sp *scrapePool) sync(targets []*Target) {
	var (
		uniqueLoops   = make(map[uint64]loop)
		interval      = time.Duration(sp.config.ScrapeInterval)
		timeout       = time.Duration(sp.config.ScrapeTimeout)
		bodySizeLimit = int64(sp.config.BodySizeLimit)
		metricLimits  = &cppbridge.MetricLimits{
			LabelLimit:            int64(sp.config.LabelLimit),
			LabelNameLengthLimit:  int64(sp.config.LabelNameLengthLimit),
			LabelValueLengthLimit: int64(sp.config.LabelValueLengthLimit),
			SampleLimit:           int64(sp.config.SampleLimit),
		}
		honorLabels              = sp.config.HonorLabels
		honorTimestamps          = sp.config.HonorTimestamps
		enableCompression        = sp.config.EnableCompression
		trackTimestampsStaleness = sp.config.TrackTimestampsStaleness
		mrc                      = sp.config.MetricRelabelConfigs
		scrapeClassicHistograms  = sp.config.ScrapeClassicHistograms
	)

	sp.targetMtx.Lock()
	for _, t := range targets {
		hash := t.hash()

		if _, ok := sp.activeTargets[hash]; !ok {
			// The scrape interval and timeout labels are set to the config's values initially,
			// so whether changed via relabeling or not, they'll exist and hold the correct values
			// for every target.
			var err error
			interval, timeout, err = t.intervalAndTimeout(interval, timeout)
			s := &targetScraper{
				Target:               t,
				client:               sp.client,
				timeout:              timeout,
				bodySizeLimit:        bodySizeLimit,
				acceptHeader:         acceptHeader(sp.config.ScrapeProtocols),
				acceptEncodingHeader: acceptEncodingHeader(enableCompression),
				metrics:              sp.metrics,
			}
			l := sp.newLoop(scrapeLoopOptions{
				target:                   t,
				scraper:                  s,
				metricLimits:             metricLimits,
				honorLabels:              honorLabels,
				honorTimestamps:          honorTimestamps,
				enableCompression:        enableCompression,
				trackTimestampsStaleness: trackTimestampsStaleness,
				mrc:                      mrc,
				interval:                 interval,
				timeout:                  timeout,
				scrapeClassicHistograms:  scrapeClassicHistograms,
			})
			if err != nil {
				l.setForcedError(err)
			}

			sp.activeTargets[hash] = t
			sp.loops[hash] = l

			uniqueLoops[hash] = l
		} else {
			// This might be a duplicated target.
			if _, ok := uniqueLoops[hash]; !ok {
				uniqueLoops[hash] = nil
			}
			// Need to keep the most updated labels information
			// for displaying it in the Service Discovery web page.
			sp.activeTargets[hash].SetDiscoveredLabels(t.DiscoveredLabels())
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

func injectSampleLabels(builder *op_model.LabelSetSimpleBuilder, target *Target, honor bool) {
	if honor {
		target.LabelsRange(func(l labels.Label) {
			if !builder.Has(l.Name) {
				builder.Add(l.Name, l.Value)
			}
		})
	} else {
		var conflictingExposedLabels []labels.Label
		target.LabelsRange(func(l labels.Label) {
			existingValue := builder.Get(l.Name)
			if existingValue != "" {
				conflictingExposedLabels = append(conflictingExposedLabels, labels.Label{Name: l.Name, Value: existingValue})
			}
			// It is now safe to set the target label.
			builder.Set(l.Name, l.Value)
		})

		if len(conflictingExposedLabels) > 0 {
			resolveConflictingExposedLabels(builder, conflictingExposedLabels)
		}
	}
}

func resolveConflictingExposedLabels(builder *op_model.LabelSetSimpleBuilder, conflictingExposedLabels []labels.Label) {
	slices.SortStableFunc(conflictingExposedLabels, func(a, b labels.Label) int {
		return len(a.Name) - len(b.Name)
	})

	for _, l := range conflictingExposedLabels {
		newName := l.Name
		for {
			newName = model.ExportedLabelPrefix + newName
			if builder.Get(newName) == "" {
				builder.Add(newName, l.Value)
				break
			}
		}
	}
}

func injectReportSampleLabels(builder *op_model.LabelSetSimpleBuilder, target *Target) {
	target.LabelsRange(func(l labels.Label) {
		builder.Add(model.ExportedLabelPrefix+l.Name, builder.Get(l.Name))
		builder.Add(l.Name, l.Value)
	})
}

// A scraper retrieves samples and accepts a status report at the end.
type scraper interface {
	scrape(ctx context.Context) (*http.Response, error)
	readResponse(ctx context.Context, resp *http.Response, w io.Writer) (string, error)
	Report(start time.Time, dur time.Duration, err error)
	offset(interval time.Duration, offsetSeed uint64) time.Duration
	stalenansState() *relabeler.SourceStates
}

// targetScraper implements the scraper interface for a target.
type targetScraper struct {
	*Target

	sourceStates *relabeler.SourceStates

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
func acceptHeader(sps []config.ScrapeProtocol) string {
	var vals []string
	weight := len(config.ScrapeProtocolsHeaders) + 1
	for _, sp := range sps {
		vals = append(vals, fmt.Sprintf("%s;q=0.%d", config.ScrapeProtocolsHeaders[sp], weight))
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

var UserAgent = fmt.Sprintf("Prometheus/%s", version.Version)

func (s *targetScraper) scrape(ctx context.Context) (*http.Response, error) {
	if s.req == nil {
		req, err := http.NewRequest("GET", s.URL().String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Add("Accept", s.acceptHeader)
		req.Header.Add("Accept-Encoding", s.acceptEncodingHeader)
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", strconv.FormatFloat(s.timeout.Seconds(), 'f', -1, 64))

		s.req = req
	}

	return s.client.Do(s.req.WithContext(ctx))
}

func (s *targetScraper) readResponse(ctx context.Context, resp *http.Response, w io.Writer) (string, error) {
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

func (s *targetScraper) stalenansState() *relabeler.SourceStates {
	return s.sourceStates
}

// A loop can run and be stopped again. It must not be reused after it was stopped.
type loop interface {
	run(errc chan<- error)
	setForcedError(err error)
	stop()
	// getCache() *scrapeCache
	disableEndOfRunStalenessMarkers()
}

type scrapeLoop struct {
	scraper        scraper
	logger         log.Logger
	lastScrapeSize int
	buffers        *pool.Pool
	bufferBuilders *buildersPool
	bufferBatches  *batchesPool

	offsetSeed               uint64
	honorTimestamps          bool
	trackTimestampsStaleness bool
	enableCompression        bool
	forcedErr                error
	forcedErrMtx             sync.Mutex
	metricLimits             *cppbridge.MetricLimits
	interval                 time.Duration
	timeout                  time.Duration
	scrapeClassicHistograms  bool
	enableCTZeroIngestion    bool

	receiver             Receiver
	sampleInjector       labelsInjector
	reportSampleInjector labelsInjector
	scrapeName           string

	parentCtx   context.Context
	appenderCtx context.Context
	ctx         context.Context
	cancel      func()
	stopped     chan struct{}

	disabledEndOfRunStalenessMarkers bool

	reportExtraMetrics  bool
	appendMetadataToWAL bool

	metrics *scrapeMetrics

	skipOffsetting bool // For testability.
}

func newScrapeLoop(
	ctx context.Context,
	sc scraper,
	logger log.Logger,
	buffers *pool.Pool,
	bufferBuilders *buildersPool,
	bufferBatches *batchesPool,
	sampleInjector labelsInjector,
	reportSampleInjector labelsInjector,
	receiver Receiver,
	scrapeName string,
	offsetSeed uint64,
	honorTimestamps bool,
	trackTimestampsStaleness bool,
	enableCompression bool,
	metricLimits *cppbridge.MetricLimits,
	interval time.Duration,
	timeout time.Duration,
	scrapeClassicHistograms bool,
	enableCTZeroIngestion bool,
	reportExtraMetrics bool,
	appendMetadataToWAL bool,
	target *Target,
	passMetadataInContext bool,
	metrics *scrapeMetrics,
	skipOffsetting bool,
) *scrapeLoop {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	if buffers == nil {
		buffers = pool.New(1e3, 1e6, 3, func(sz int) interface{} { return make([]byte, 0, sz) })
	}

	appenderCtx := ctx

	if passMetadataInContext {
		// Store the cache and target in the context. This is then used by downstream OTel Collector
		// to lookup the metadata required to process the samples. Not used by Prometheus itself.
		// TODO(gouthamve) We're using a dedicated context because using the parentCtx caused a memory
		// leak. We should ideally fix the main leak. See: https://github.com/prometheus/prometheus/pull/10590
		// appenderCtx = ContextWithMetricMetadataStore(appenderCtx, cache)
		appenderCtx = ContextWithTarget(appenderCtx, target)
	}

	sl := &scrapeLoop{
		scraper:                  sc,
		buffers:                  buffers,
		bufferBuilders:           bufferBuilders,
		bufferBatches:            bufferBatches,
		receiver:                 receiver,
		sampleInjector:           sampleInjector,
		reportSampleInjector:     reportSampleInjector,
		scrapeName:               scrapeName,
		stopped:                  make(chan struct{}),
		offsetSeed:               offsetSeed,
		logger:                   logger,
		parentCtx:                ctx,
		appenderCtx:              appenderCtx,
		honorTimestamps:          honorTimestamps,
		trackTimestampsStaleness: trackTimestampsStaleness,
		enableCompression:        enableCompression,
		metricLimits:             metricLimits,
		interval:                 interval,
		timeout:                  timeout,
		scrapeClassicHistograms:  scrapeClassicHistograms,
		enableCTZeroIngestion:    enableCTZeroIngestion,
		reportExtraMetrics:       reportExtraMetrics,
		appendMetadataToWAL:      appendMetadataToWAL,
		metrics:                  metrics,
		skipOffsetting:           skipOffsetting,
	}
	sl.ctx, sl.cancel = context.WithCancel(ctx)

	return sl
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
		if AlignScrapeTimestamps && sl.interval > 100*ScrapeTimestampTolerance {
			// For some reason, a tick might have been skipped, in which case we
			// would call alignedScrapeTime.Add(interval) multiple times.
			for scrapeTime.Sub(alignedScrapeTime) >= sl.interval {
				alignedScrapeTime = alignedScrapeTime.Add(sl.interval)
			}
			// Align the scrape time if we are in the tolerance boundaries.
			if scrapeTime.Sub(alignedScrapeTime) <= ScrapeTimestampTolerance {
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

	if !sl.disabledEndOfRunStalenessMarkers {
		sl.endOfRunStaleness(last, ticker, sl.interval)
	}
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
	}

	var total, added, bytesRead int
	var appErr, scrapeErr error

	// var total, added, seriesAdded, bytesRead int
	// var err, appErr, scrapeErr error

	defer func() {
		if err := sl.report(appendTime, time.Since(start), total, added, bytesRead, scrapeErr); err != nil {
			level.Warn(sl.logger).Log("msg", "Appending scrape report failed", "err", err)
		}
	}()

	if forcedErr := sl.getForcedError(); forcedErr != nil {
		scrapeErr = forcedErr
		// Add stale markers.
		if _, _, err := sl.append([]byte{}, "", appendTime); err != nil {
			level.Warn(sl.logger).Log("msg", "Append failed", "err", err)
		}
		if errc != nil {
			errc <- forcedErr
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
		level.Debug(sl.logger).Log("msg", "Scrape failed", "err", scrapeErr)
		if errc != nil {
			errc <- scrapeErr
		}
		if errors.Is(scrapeErr, errBodySizeLimit) {
			bytesRead = -1
		}
	}

	// A failed scrape is the same as an empty scrape,
	// we still call sl.append to trigger stale markers.
	total, added, appErr = sl.append(b, contentType, appendTime)
	level.Debug(sl.logger).Log("msg", "scrape append", "total", total, "added", added)
	if appErr != nil {
		level.Debug(sl.logger).Log("msg", "Append failed", "err", appErr)
		// The append failed, probably due to a parse error or sample limit.
		// Call sl.append again with an empty scrape to trigger stale markers.
		if _, _, err := sl.append([]byte{}, "", appendTime); err != nil {
			level.Warn(sl.logger).Log("msg", "Append failed", "err", err)
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

	// Call sl.append again with an empty scrape to trigger stale markers.
	// If the target has since been recreated and scraped, the
	// stale markers will be out of order and ignored.
	// sl.context would have been cancelled, hence using sl.appenderCtx.
	emptyBatch := sl.bufferBatches.get()
	if err := sl.receiver.AppendTimeSeries(
		sl.appenderCtx,
		emptyBatch,
		sl.metricLimits,
		sl.scraper.stalenansState(),
		timestamp.FromTime(staleTime),
		sl.scrapeName,
	); err != nil {
		level.Warn(sl.logger).Log("msg", "Stale append failed", "err", err)
	}

	if err := sl.reportStale(staleTime); err != nil {
		level.Warn(sl.logger).Log("msg", "Stale report failed", "err", err)
	}
}

// Stop the scraping. May still write data and stale markers after it has
// returned. Cancel the context to stop all writes.
func (sl *scrapeLoop) stop() {
	sl.cancel()
	<-sl.stopped
}

func (sl *scrapeLoop) disableEndOfRunStalenessMarkers() {
	sl.disabledEndOfRunStalenessMarkers = true
}

func (sl *scrapeLoop) append(b []byte, contentType string, ts time.Time) (total, added int, err error) {
	p, err := textparse.New(b, contentType, sl.scrapeClassicHistograms)
	if err != nil {
		level.Debug(sl.logger).Log(
			"msg", "Invalid content type on scrape, using prometheus parser as fallback.",
			"content_type", contentType,
			"err", err,
		)
	}

	defer func() {
		if err != nil {
			return
		}
		// // Only perform cache cleaning if the scrape was not empty.
		// // An empty scrape (usually) is used to indicate a failed scrape.
		// sl.cache.iterDone(len(b) > 0)
	}()

	var (
		defTime = timestamp.FromTime(ts)
		met     []byte
	)

	batch := BatchWithLimit(sl.bufferBatches.get())
	builder := sl.bufferBuilders.get()
	defer sl.bufferBuilders.put(builder)

loop:
	for {
		var (
			et              textparse.Entry
			parsedTimestamp *int64
			val             float64
		)
		if et, err = p.Next(); err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			break
		}

		switch et {
		case textparse.EntryType,
			textparse.EntryHelp,
			textparse.EntryUnit,
			textparse.EntryComment,
			textparse.EntryHistogram:
			continue
		default:
		}
		total++

		t := defTime
		met, parsedTimestamp, val = p.Series()
		if !sl.honorTimestamps {
			parsedTimestamp = nil
		}
		if parsedTimestamp != nil {
			t = *parsedTimestamp
		}

		p.MetricToBuilder(builder)
		sl.sampleInjector(builder)
		if ln, dup := builder.HasDuplicateLabelNames(); dup {
			level.Warn(sl.logger).Log(
				"msg", "label name is not unique, skip series",
				"series", string(met),
				"series byte", met,
				"builder", builder.Build().String(),
				"name", ln,
			)
			continue
		}

		if addErr := batch.Add(builder, uint64(t), val); addErr != nil {
			level.Debug(sl.logger).Log("msg", "failed add metrics", "err", addErr)
			switch {
			case errors.Is(err, errSampleLimit):
				sl.metrics.targetScrapeSampleLimit.Inc()
				break loop
			case errors.Is(err, storage.ErrOutOfBounds):
				sl.metrics.targetScrapeSampleOutOfBounds.Inc()
				continue
			}
		}
	}

	if batch.IsEmpty() {
		batch.Destroy()
		level.Warn(sl.logger).Log("msg", "scrape data is empty")
		return
	}

	batched := batch.Len()
	if err = sl.receiver.AppendTimeSeries(
		sl.appenderCtx,
		batch,
		sl.metricLimits,
		sl.scraper.stalenansState(),
		defTime,
		sl.scrapeName,
	); err != nil {
		return
	}
	added = batched
	return
}

// The constants are suffixed with the invalid \xff unicode rune to avoid collisions
// with scraped metrics in the cache.
var (
	scrapeHealthMetricName        = "up"
	scrapeDurationMetricName      = "scrape_duration_seconds"
	scrapeSamplesMetricName       = "scrape_samples_scraped"
	samplesPostRelabelMetricName  = "scrape_samples_post_metric_relabeling"
	scrapeTimeoutMetricName       = "scrape_timeout_seconds"
	scrapeSampleLimitMetricName   = "scrape_sample_limit"
	scrapeBodySizeBytesMetricName = "scrape_body_size_bytes"
)

func (sl *scrapeLoop) report(
	start time.Time,
	duration time.Duration,
	scraped, added, bytes int,
	scrapeErr error,
) (err error) {
	sl.scraper.Report(start, duration, scrapeErr)
	ts := timestamp.FromTime(start)

	var health float64
	if scrapeErr == nil {
		health = 1
	}

	builder := sl.bufferBuilders.get()
	defer sl.bufferBuilders.put(builder)
	batch := sl.bufferBatches.get()

	if err = sl.addReportSample(builder, batch, scrapeHealthMetricName, ts, health); err != nil {
		return
	}
	if err = sl.addReportSample(builder, batch, scrapeDurationMetricName, ts, duration.Seconds()); err != nil {
		return
	}
	if err = sl.addReportSample(builder, batch, scrapeSamplesMetricName, ts, float64(scraped)); err != nil {
		return
	}
	if err = sl.addReportSample(builder, batch, samplesPostRelabelMetricName, ts, float64(added)); err != nil {
		return
	}

	if sl.reportExtraMetrics {
		if err = sl.addReportSample(builder, batch, scrapeTimeoutMetricName, ts, sl.timeout.Seconds()); err != nil {
			return
		}
		if err = sl.addReportSample(
			builder,
			batch,
			scrapeSampleLimitMetricName,
			ts,
			float64(sl.metricLimits.SampleLimit),
		); err != nil {
			return
		}
		if err = sl.addReportSample(builder, batch, scrapeBodySizeBytesMetricName, ts, float64(bytes)); err != nil {
			return
		}
	}

	if err = sl.receiver.AppendTimeSeries(
		sl.appenderCtx,
		batch,
		nil,
		nil,
		0,
		config.TransparentRelabeler,
	); err != nil {
		return
	}

	return
}

func (sl *scrapeLoop) reportStale(start time.Time) (err error) {
	ts := timestamp.FromTime(start)
	stale := math.Float64frombits(value.StaleNaN)

	builder := sl.bufferBuilders.get()
	defer sl.bufferBuilders.put(builder)
	batch := sl.bufferBatches.get()

	if err = sl.addReportSample(builder, batch, scrapeHealthMetricName, ts, stale); err != nil {
		return
	}
	if err = sl.addReportSample(builder, batch, scrapeDurationMetricName, ts, stale); err != nil {
		return
	}
	if err = sl.addReportSample(builder, batch, scrapeSamplesMetricName, ts, stale); err != nil {
		return
	}
	if err = sl.addReportSample(builder, batch, samplesPostRelabelMetricName, ts, stale); err != nil {
		return
	}

	if sl.reportExtraMetrics {
		if err = sl.addReportSample(builder, batch, scrapeTimeoutMetricName, ts, stale); err != nil {
			return
		}
		if err = sl.addReportSample(builder, batch, scrapeSampleLimitMetricName, ts, stale); err != nil {
			return
		}
		if err = sl.addReportSample(builder, batch, scrapeBodySizeBytesMetricName, ts, stale); err != nil {
			return
		}
	}

	if err = sl.receiver.AppendTimeSeries(
		sl.appenderCtx,
		batch,
		nil,
		nil,
		0,
		config.TransparentRelabeler,
	); err != nil {
		return
	}

	return
}

func (sl *scrapeLoop) addReportSample(
	builder *op_model.LabelSetSimpleBuilder,
	batch *BatchTimeSeries,
	nameValue string,
	t int64,
	v float64,
) error {
	builder.Reset()
	builder.Add(labels.MetricName, nameValue)
	sl.reportSampleInjector(builder)
	return batch.Add(builder, uint64(t), v)
}

// CtxKey is a dedicated type for keys of context-embedded values propagated
// with the scrape context.
type ctxKey int

// Valid CtxKey values.
const (
	ctxKeyMetadata ctxKey = iota + 1
	ctxKeyTarget
)

// func ContextWithMetricMetadataStore(ctx context.Context, s MetricMetadataStore) context.Context {
// 	return context.WithValue(ctx, ctxKeyMetadata, s)
// }

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
