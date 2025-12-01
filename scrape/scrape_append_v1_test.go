package scrape

import (
	"context"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/storage"
)

// Collection of tests for Appender v1 flow.
// TODO(bwplotka): Remove once Otel uses v2 flow.

type nopAppendableV1 struct{}

func (nopAppendableV1) Appender(context.Context) storage.Appender {
	return nopAppenderV1{}
}

type nopAppenderV1 struct{}

func (nopAppenderV1) SetOptions(*storage.AppendOptions) {}

func (nopAppenderV1) Append(storage.SeriesRef, labels.Labels, int64, float64) (storage.SeriesRef, error) {
	return 1, nil
}

func (nopAppenderV1) AppendExemplar(storage.SeriesRef, labels.Labels, exemplar.Exemplar) (storage.SeriesRef, error) {
	return 2, nil
}

func (nopAppenderV1) AppendHistogram(storage.SeriesRef, labels.Labels, int64, *histogram.Histogram, *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 3, nil
}

func (nopAppenderV1) AppendHistogramSTZeroSample(storage.SeriesRef, labels.Labels, int64, int64, *histogram.Histogram, *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, nil
}

func (nopAppenderV1) UpdateMetadata(storage.SeriesRef, labels.Labels, metadata.Metadata) (storage.SeriesRef, error) {
	return 4, nil
}

func (nopAppenderV1) AppendSTZeroSample(storage.SeriesRef, labels.Labels, int64, int64) (storage.SeriesRef, error) {
	return 5, nil
}

func (nopAppenderV1) Commit() error   { return nil }
func (nopAppenderV1) Rollback() error { return nil }

/*
type collectResultAppendableV1 struct {
	*collectResultAppenderV1
}

func (a *collectResultAppendableV1) Appender(context.Context) storage.Appender {
	return a
}

// collectResultAppenderV1 records all samples that were added through the appender.
// It can be used as its zero value or be backed by another appender it writes samples through.
type collectResultAppenderV1 struct {
	mtx sync.Mutex

	next                 storage.Appender
	resultFloats         []floatSample
	pendingFloats        []floatSample
	rolledbackFloats     []floatSample
	resultHistograms     []histogramSample
	pendingHistograms    []histogramSample
	rolledbackHistograms []histogramSample
	resultExemplars      []exemplar.Exemplar
	pendingExemplars     []exemplar.Exemplar
	resultMetadata       []metadataEntry
	pendingMetadata      []metadataEntry
}

func (*collectResultAppenderV1) SetOptions(*storage.AppendOptions) {}

func (a *collectResultAppenderV1) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.pendingFloats = append(a.pendingFloats, floatSample{
		metric: lset,
		t:      t,
		f:      v,
	})

	if a.next == nil {
		if ref == 0 {
			// Use labels hash as a stand-in for unique series reference, to avoid having to track all series.
			ref = storage.SeriesRef(lset.Hash())
		}
		return ref, nil
	}

	ref, err := a.next.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

func (a *collectResultAppenderV1) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.pendingExemplars = append(a.pendingExemplars, e)
	if a.next == nil {
		return 0, nil
	}

	return a.next.AppendExemplar(ref, l, e)
}

func (a *collectResultAppenderV1) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.pendingHistograms = append(a.pendingHistograms, histogramSample{h: h, fh: fh, t: t, metric: l})
	if a.next == nil {
		return 0, nil
	}

	return a.next.AppendHistogram(ref, l, t, h, fh)
}

func (a *collectResultAppenderV1) AppendHistogramSTZeroSample(ref storage.SeriesRef, l labels.Labels, _, st int64, h *histogram.Histogram, _ *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if h != nil {
		return a.AppendHistogram(ref, l, st, &histogram.Histogram{}, nil)
	}
	return a.AppendHistogram(ref, l, st, nil, &histogram.FloatHistogram{})
}

func (a *collectResultAppenderV1) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.pendingMetadata = append(a.pendingMetadata, metadataEntry{metric: l, m: m})
	if a.next == nil {
		if ref == 0 {
			ref = storage.SeriesRef(l.Hash())
		}
		return ref, nil
	}

	return a.next.UpdateMetadata(ref, l, m)
}

func (a *collectResultAppenderV1) AppendSTZeroSample(ref storage.SeriesRef, l labels.Labels, _, st int64) (storage.SeriesRef, error) {
	return a.Append(ref, l, st, 0.0)
}

func (a *collectResultAppenderV1) Commit() error {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.resultFloats = append(a.resultFloats, a.pendingFloats...)
	a.resultExemplars = append(a.resultExemplars, a.pendingExemplars...)
	a.resultHistograms = append(a.resultHistograms, a.pendingHistograms...)
	a.resultMetadata = append(a.resultMetadata, a.pendingMetadata...)
	a.pendingFloats = nil
	a.pendingExemplars = nil
	a.pendingHistograms = nil
	a.pendingMetadata = nil
	if a.next == nil {
		return nil
	}
	return a.next.Commit()
}

func (a *collectResultAppenderV1) Rollback() error {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.rolledbackFloats = a.pendingFloats
	a.rolledbackHistograms = a.pendingHistograms
	a.pendingFloats = nil
	a.pendingHistograms = nil
	if a.next == nil {
		return nil
	}
	return a.next.Rollback()
}

func (a *collectResultAppenderV1) String() string {
	var sb strings.Builder
	for _, s := range a.resultFloats {
		sb.WriteString(fmt.Sprintf("committed: %s %f %d\n", s.metric, s.f, s.t))
	}
	for _, s := range a.pendingFloats {
		sb.WriteString(fmt.Sprintf("pending: %s %f %d\n", s.metric, s.f, s.t))
	}
	for _, s := range a.rolledbackFloats {
		sb.WriteString(fmt.Sprintf("rolledback: %s %f %d\n", s.metric, s.f, s.t))
	}
	return sb.String()
}

func runManagersV1(t *testing.T, ctx context.Context, opts *Options, app storage.Appendable) (*discovery.Manager, *Manager) {
	t.Helper()

	if opts == nil {
		opts = &Options{}
	}
	opts.DiscoveryReloadInterval = model.Duration(100 * time.Millisecond)
	if app == nil {
		app = nopAppendableV1{}
	}

	reg := prometheus.NewRegistry()
	sdMetrics, err := discovery.RegisterSDMetrics(reg, discovery.NewRefreshMetrics(reg))
	require.NoError(t, err)
	discoveryManager := discovery.NewManager(
		ctx,
		promslog.NewNopLogger(),
		reg,
		sdMetrics,
		discovery.Updatert(100*time.Millisecond),
	)
	scrapeManager, err := NewManager(
		opts,
		nil,
		nil,
		app,
		prometheus.NewRegistry(),
	)
	require.NoError(t, err)
	go discoveryManager.Run()
	go scrapeManager.Run(discoveryManager.SyncCh())
	return discoveryManager, scrapeManager
}

// TestManagerSTZeroIngestion_AppenderV1 tests scrape manager for various ST cases.
func TestManagerSTZeroIngestion_AppenderV1(t *testing.T) {
	t.Parallel()
	const (
		// _total suffix is required, otherwise expfmt with OMText will mark metric as "unknown"
		expectedMetricName        = "expected_metric_total"
		expectedCreatedMetricName = "expected_metric_created"
		expectedSampleValue       = 17.0
	)

	for _, testFormat := range []config.ScrapeProtocol{config.PrometheusProto, config.OpenMetricsText1_0_0} {
		t.Run(fmt.Sprintf("format=%s", testFormat), func(t *testing.T) {
			for _, testWithST := range []bool{false, true} {
				t.Run(fmt.Sprintf("withST=%v", testWithST), func(t *testing.T) {
					for _, testSTZeroIngest := range []bool{false, true} {
						t.Run(fmt.Sprintf("ctZeroIngest=%v", testSTZeroIngest), func(t *testing.T) {
							ctx, cancel := context.WithCancel(context.Background())
							defer cancel()

							sampleTs := time.Now()
							stTs := time.Time{}
							if testWithST {
								stTs = sampleTs.Add(-2 * time.Minute)
							}

							// TODO(bwplotka): Add more types than just counter?
							encoded := prepareTestEncodedCounter(t, testFormat, expectedMetricName, expectedSampleValue, sampleTs, stTs)

							app := &collectResultAppenderV1{}
							discoveryManager, scrapeManager := runManagersV1(t, ctx, &Options{
								EnableStartTimestampZeroIngestion: testSTZeroIngest,
								skipOffsetting:                    true,
							}, &collectResultAppendableV1{app})
							defer scrapeManager.Stop()

							server := setupTestServer(t, config.ScrapeProtocolsHeaders[testFormat], encoded)
							serverURL, err := url.Parse(server.URL)
							require.NoError(t, err)

							testConfig := fmt.Sprintf(`
global:
  # Disable regular scrapes.
  scrape_interval: 9999m
  scrape_timeout: 5s

scrape_configs:
- job_name: test
  honor_timestamps: true
  static_configs:
  - targets: ['%s']
`, serverURL.Host)
							applyConfig(t, testConfig, scrapeManager, discoveryManager)

							// Wait for one scrape.
							ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
							defer cancel()
							require.NoError(t, runutil.Retry(100*time.Millisecond, ctx.Done(), func() error {
								app.mtx.Lock()
								defer app.mtx.Unlock()

								// Check if scrape happened and grab the relevant samples.
								if len(app.resultFloats) > 0 {
									return nil
								}
								return errors.New("expected some float samples, got none")
							}), "after 1 minute")

							// Verify results.
							// Verify what we got vs expectations around ST injection.
							samples := findSamplesForMetric(app.resultFloats, expectedMetricName)
							if testWithST && testSTZeroIngest {
								require.Len(t, samples, 2)
								require.Equal(t, 0.0, samples[0].f)
								require.Equal(t, timestamp.FromTime(stTs), samples[0].t)
								require.Equal(t, expectedSampleValue, samples[1].f)
								require.Equal(t, timestamp.FromTime(sampleTs), samples[1].t)
							} else {
								require.Len(t, samples, 1)
								require.Equal(t, expectedSampleValue, samples[0].f)
								require.Equal(t, timestamp.FromTime(sampleTs), samples[0].t)
							}

							// Verify what we got vs expectations around additional _created series for OM text.
							// enableSTZeroInjection also kills that _created line.
							createdSeriesSamples := findSamplesForMetric(app.resultFloats, expectedCreatedMetricName)
							if testFormat == config.OpenMetricsText1_0_0 && testWithST && !testSTZeroIngest {
								// For OM Text, when counter has ST, and feature flag disabled we should see _created lines.
								require.Len(t, createdSeriesSamples, 1)
								// Conversion taken from common/expfmt.writeOpenMetricsFloat.
								// We don't check the st timestamp as explicit ts was not implemented in expfmt.Encoder,
								// but exists in OM https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#:~:text=An%20example%20with%20a%20Metric%20with%20no%20labels%2C%20and%20a%20MetricPoint%20with%20a%20timestamp%20and%20a%20created
								// We can implement this, but we want to potentially get rid of OM 1.0 ST lines
								require.Equal(t, float64(timestamppb.New(stTs).AsTime().UnixNano())/1e9, createdSeriesSamples[0].f)
							} else {
								require.Empty(t, createdSeriesSamples)
							}
						})
					}
				})
			}
		})
	}
}

func TestManagerSTZeroIngestionHistogram_AppenderV1(t *testing.T) {
	t.Parallel()
	const mName = "expected_histogram"

	for _, tc := range []struct {
		name                  string
		inputHistSample       *dto.Histogram
		enableSTZeroIngestion bool
	}{
		{
			name: "disabled with ST on histogram",
			inputHistSample: func() *dto.Histogram {
				h := generateTestHistogram(0)
				h.CreatedTimestamp = timestamppb.Now()
				return h
			}(),
			enableSTZeroIngestion: false,
		},
		{
			name: "enabled with ST on histogram",
			inputHistSample: func() *dto.Histogram {
				h := generateTestHistogram(0)
				h.CreatedTimestamp = timestamppb.Now()
				return h
			}(),
			enableSTZeroIngestion: true,
		},
		{
			name: "enabled without ST on histogram",
			inputHistSample: func() *dto.Histogram {
				h := generateTestHistogram(0)
				return h
			}(),
			enableSTZeroIngestion: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			app := &collectResultAppenderV1{}
			discoveryManager, scrapeManager := runManagersV1(t, ctx, &Options{
				EnableStartTimestampZeroIngestion: tc.enableSTZeroIngestion,
				skipOffsetting:                    true,
			}, &collectResultAppendableV1{app})
			defer scrapeManager.Stop()

			once := sync.Once{}
			// Start fake HTTP target to that allow one scrape only.
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					fail := true
					once.Do(func() {
						fail = false
						w.Header().Set("Content-Type", `application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited`)

						ctrType := dto.MetricType_HISTOGRAM
						w.Write(protoMarshalDelimited(t, &dto.MetricFamily{
							Name:   proto.String(mName),
							Type:   &ctrType,
							Metric: []*dto.Metric{{Histogram: tc.inputHistSample}},
						}))
					})

					if fail {
						w.WriteHeader(http.StatusInternalServerError)
					}
				}),
			)
			defer server.Close()

			serverURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			testConfig := fmt.Sprintf(`
global:
  # Disable regular scrapes.
  scrape_interval: 9999m
  scrape_timeout: 5s

scrape_configs:
- job_name: test
  scrape_native_histograms: true
  static_configs:
  - targets: ['%s']
`, serverURL.Host)
			applyConfig(t, testConfig, scrapeManager, discoveryManager)

			var got []histogramSample

			// Wait for one scrape.
			ctx, cancel = context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()
			require.NoError(t, runutil.Retry(100*time.Millisecond, ctx.Done(), func() error {
				app.mtx.Lock()
				defer app.mtx.Unlock()

				// Check if scrape happened and grab the relevant histograms, they have to be there - or it's a bug
				// and it's not worth waiting.
				for _, h := range app.resultHistograms {
					if h.metric.Get(model.MetricNameLabel) == mName {
						got = append(got, h)
					}
				}
				if len(app.resultHistograms) > 0 {
					return nil
				}
				return errors.New("expected some histogram samples, got none")
			}), "after 1 minute")

			// Check for zero samples, assuming we only injected always one histogram sample.
			// Did it contain ST to inject? If yes, was ST zero enabled?
			if tc.inputHistSample.CreatedTimestamp.IsValid() && tc.enableSTZeroIngestion {
				require.Len(t, got, 2)
				// Zero sample.
				require.Equal(t, histogram.Histogram{}, *got[0].h)
				// Quick soft check to make sure it's the same sample or at least not zero.
				require.Equal(t, tc.inputHistSample.GetSampleSum(), got[1].h.Sum)
				return
			}

			// Expect only one, valid sample.
			require.Len(t, got, 1)
			// Quick soft check to make sure it's the same sample or at least not zero.
			require.Equal(t, tc.inputHistSample.GetSampleSum(), got[0].h.Sum)
		})
	}
}

// TestNHCBAndSTZeroIngestion_AppenderV1 verifies that both ConvertClassicHistogramsToNHCBEnabled
// and EnableStartTimestampZeroIngestion can be used simultaneously without errors.
// This test addresses issue #17216 by ensuring the previously blocking check has been removed.
// The test verifies that the presence of exemplars in the input does not cause errors,
// although exemplars are not preserved during NHCB conversion (as documented below).
func TestNHCBAndSTZeroIngestion_AppenderV1(t *testing.T) {
	t.Parallel()

	const (
		mName = "test_histogram"
		// The expected sum of the histogram, as defined by the test's OpenMetrics exposition data.
		// This value (45.5) is the sum reported in the test_histogram_sum metric below.
		expectedHistogramSum = 45.5
	)

	ctx := t.Context()

	app := &collectResultAppenderV1{}
	discoveryManager, scrapeManager := runManagersV1(t, ctx, &Options{
		EnableStartTimestampZeroIngestion: true,
		skipOffsetting:                    true,
	}, &collectResultAppendableV1{app})
	defer scrapeManager.Stop()

	once := sync.Once{}
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fail := true
			once.Do(func() {
				fail = false
				w.Header().Set("Content-Type", `application/openmetrics-text`)

				// Expose a histogram with created timestamp and exemplars to verify no parsing errors occur.
				fmt.Fprint(w, `# HELP test_histogram A histogram with created timestamp and exemplars
# TYPE test_histogram histogram
test_histogram_bucket{le="0.0"} 1
test_histogram_bucket{le="1.0"} 10 # {trace_id="trace-1"} 0.5 123456789
test_histogram_bucket{le="2.0"} 20 # {trace_id="trace-2"} 1.5 123456780
test_histogram_bucket{le="+Inf"} 30 # {trace_id="trace-3"} 2.5
test_histogram_count 30
test_histogram_sum 45.5
test_histogram_created 1520430001
# EOF
`)
			})

			if fail {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	// Configuration with both convert_classic_histograms_to_nhcb enabled and ST zero ingestion enabled.
	testConfig := fmt.Sprintf(`
global:
  # Use a very long scrape_interval to prevent automatic scraping during the test.
  scrape_interval: 9999m
  scrape_timeout: 5s

scrape_configs:
- job_name: test
  convert_classic_histograms_to_nhcb: true
  static_configs:
  - targets: ['%s']
`, serverURL.Host)

	applyConfig(t, testConfig, scrapeManager, discoveryManager)

	// Verify that the scrape pool was created (proves the blocking check was removed).
	require.Eventually(t, func() bool {
		scrapeManager.mtxScrape.Lock()
		defer scrapeManager.mtxScrape.Unlock()
		_, exists := scrapeManager.scrapePools["test"]
		return exists
	}, 5*time.Second, 100*time.Millisecond, "scrape pool should be created for job 'test'")

	// Helper function to get matching histograms to avoid race conditions.
	getMatchingHistograms := func() []histogramSample {
		app.mtx.Lock()
		defer app.mtx.Unlock()

		var got []histogramSample
		for _, h := range app.resultHistograms {
			if h.metric.Get(model.MetricNameLabel) == mName {
				got = append(got, h)
			}
		}
		return got
	}

	require.Eventually(t, func() bool {
		return len(getMatchingHistograms()) > 0
	}, 1*time.Minute, 100*time.Millisecond, "expected histogram samples, got none")

	// Verify that samples were ingested (proving both features work together).
	got := getMatchingHistograms()

	// With ST zero ingestion enabled and a created timestamp present, we expect 2 samples:
	// one zero sample and one actual sample.
	require.Len(t, got, 2, "expected 2 histogram samples (zero sample + actual sample)")
	require.Equal(t, histogram.Histogram{}, *got[0].h, "first sample should be zero sample")
	require.InDelta(t, expectedHistogramSum, got[1].h.Sum, 1e-9, "second sample should retain the expected sum")
	require.Len(t, app.resultExemplars, 2, "expected 2 exemplars from histogram buckets")
}

func TestManagerDisableEndOfRunStalenessMarkers_AppenderV1(t *testing.T) {
	cfgText := `
scrape_configs:
 - job_name: one
   scrape_interval: 1m
   scrape_timeout: 1m
 - job_name: two
   scrape_interval: 1m
   scrape_timeout: 1m
`

	cfg := loadConfiguration(t, cfgText)

	m, err := NewManager(&Options{}, nil, nil, &nopAppendableV1{}, prometheus.NewRegistry())
	require.NoError(t, err)
	defer m.Stop()
	require.NoError(t, m.ApplyConfig(cfg))

	// Pass targets to the manager.
	tgs := map[string][]*targetgroup.Group{
		"one": {{Targets: []model.LabelSet{{"__address__": "h1"}, {"__address__": "h2"}, {"__address__": "h3"}}}},
		"two": {{Targets: []model.LabelSet{{"__address__": "h4"}}}},
	}
	m.updateTsets(tgs)
	m.reload()

	activeTargets := m.TargetsActive()
	targetsToDisable := []*Target{
		activeTargets["one"][0],
		activeTargets["one"][2],
	}

	// Disable end of run staleness markers for some targets.
	m.DisableEndOfRunStalenessMarkers("one", targetsToDisable)
	// This should be a no-op
	m.DisableEndOfRunStalenessMarkers("non-existent-job", targetsToDisable)

	// Check that the end of run staleness markers are disabled for the correct targets.
	for _, group := range []string{"one", "two"} {
		for _, tg := range activeTargets[group] {
			loop := m.scrapePools[group].loops[tg.hash()].(*scrapeLoop)
			expectedDisabled := slices.Contains(targetsToDisable, tg)
			require.Equal(t, expectedDisabled, loop.disabledEndOfRunStalenessMarkers.Load())
		}
	}
}

// maybeLimitedIsEnoughAppendableV1 panics if anything other than appending sample is invoked
// TODO(bwplotka): Move to storage interface_compact if applicable wider (and error out?)
type maybeLimitedIsEnoughAppendableV1 struct {
	app *storage.LimitedAppenderV1
}

func (a *maybeLimitedIsEnoughAppendableV1) Appender(ctx context.Context) storage.Appender {
	return maybeLimitedV1IsEnoughAppenderV1{a: a.app}
}

type maybeLimitedV1IsEnoughAppenderV1 struct {
	storage.Appender
	a *storage.LimitedAppenderV1
}

func (a *maybeLimitedV1IsEnoughAppenderV1) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	return a.a.Append(ref, l, t, v)
}

func (a *maybeLimitedV1IsEnoughAppenderV1) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return a.a.AppendHistogram(ref, l, t, h, fh)
}

func (a *maybeLimitedV1IsEnoughAppenderV1) Commit() error {
	return a.a.Commit()
}

func (a *maybeLimitedV1IsEnoughAppenderV1) Rollback() error {
	return a.a.Rollback()
}

func runScrapeLoopTestV1(t *testing.T, s *teststorage.TestStorage, expectOutOfOrder bool) {
	// Create an appender for adding samples to the storage.
	app := s.Appender(context.Background())
	capp := &collectResultAppenderV1{next: &maybeLimitedV1IsEnoughAppenderV1{a: app}}
	sl := newBasicScrapeLoop(t, context.Background(), nil, &collectResultAppendableV1{capp}, nil, 0)

	// Current time for generating timestamps.
	now := time.Now()

	// Calculate timestamps for the samples based on the current time.
	now = now.Truncate(time.Minute) // round down the now timestamp to the nearest minute
	timestampInorder1 := now
	timestampOutOfOrder := now.Add(-5 * time.Minute)
	timestampInorder2 := now.Add(5 * time.Minute)

	slApp := sl.appender(context.Background())
	_, _, _, err := slApp.append([]byte(`metric_total{a="1",b="1"} 1`), "text/plain", timestampInorder1)
	require.NoError(t, err)

	_, _, _, err = slApp.append([]byte(`metric_total{a="1",b="1"} 2`), "text/plain", timestampOutOfOrder)
	require.NoError(t, err)

	_, _, _, err = slApp.append([]byte(`metric_total{a="1",b="1"} 3`), "text/plain", timestampInorder2)
	require.NoError(t, err)

	require.NoError(t, slApp.Commit())

	// Query the samples back from the storage.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q, err := s.Querier(time.Time{}.UnixNano(), time.Now().UnixNano())
	require.NoError(t, err)
	defer q.Close()

	// Use a matcher to filter the metric name.
	series := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "metric_total"))

	var results []floatSample
	for series.Next() {
		it := series.At().Iterator(nil)
		for it.Next() == chunkenc.ValFloat {
			t, v := it.At()
			results = append(results, floatSample{
				metric: series.At().Labels(),
				t:      t,
				f:      v,
			})
		}
		require.NoError(t, it.Err())
	}
	require.NoError(t, series.Err())

	// Define the expected results
	want := []floatSample{
		{
			metric: labels.FromStrings("__name__", "metric_total", "a", "1", "b", "1"),
			t:      timestamp.FromTime(timestampInorder1),
			f:      1,
		},
		{
			metric: labels.FromStrings("__name__", "metric_total", "a", "1", "b", "1"),
			t:      timestamp.FromTime(timestampInorder2),
			f:      3,
		},
	}

	if expectOutOfOrder {
		require.NotEqual(t, want, results, "Expected results to include out-of-order sample:\n%s", results)
	} else {
		require.Equal(t, want, results, "Appended samples not as expected:\n%s", results)
	}
}

// Regression test against https://github.com/prometheus/prometheus/issues/15831.
func TestScrapeAppendMetadataUpdate_AppenderV1(t *testing.T) {
	const (
		scrape1 = `# TYPE test_metric counter
# HELP test_metric some help text
# UNIT test_metric metric
test_metric_total 1
# TYPE test_metric2 gauge
# HELP test_metric2 other help text
test_metric2{foo="bar"} 2
# TYPE test_metric3 gauge
# HELP test_metric3 this represents tricky case of "broken" text that is not trivial to detect
test_metric3_metric4{foo="bar"} 2
# EOF`
		scrape2 = `# TYPE test_metric counter
# HELP test_metric different help text
test_metric_total 11
# TYPE test_metric2 gauge
# HELP test_metric2 other help text
# UNIT test_metric2 metric2
test_metric2{foo="bar"} 22
# EOF`
	)

	// Create an appender for adding samples to the storage.
	capp := &collectResultAppenderV1{}
	sl := newBasicScrapeLoop(t, context.Background(), nil, &collectResultAppendableV1{capp}, nil, 0)

	now := time.Now()
	slApp := sl.appender(context.Background())
	_, _, _, err := slApp.append([]byte(scrape1), "application/openmetrics-text", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	testutil.RequireEqualWithOptions(t, []metadataEntry{
		{metric: labels.FromStrings("__name__", "test_metric_total"), m: metadata.Metadata{Type: "counter", Unit: "metric", Help: "some help text"}},
		{metric: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), m: metadata.Metadata{Type: "gauge", Unit: "", Help: "other help text"}},
	}, capp.resultMetadata, []cmp.Option{cmp.Comparer(metadataEntryEqual)})
	capp.resultMetadata = nil

	// Next (the same) scrape should not add new metadata entries.
	slApp = sl.appender(context.Background())
	_, _, _, err = slApp.append([]byte(scrape1), "application/openmetrics-text", now.Add(15*time.Second))
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	testutil.RequireEqualWithOptions(t, []metadataEntry(nil), capp.resultMetadata, []cmp.Option{cmp.Comparer(metadataEntryEqual)})

	slApp = sl.appender(context.Background())
	_, _, _, err = slApp.append([]byte(scrape2), "application/openmetrics-text", now.Add(15*time.Second))
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	testutil.RequireEqualWithOptions(t, []metadataEntry{
		{metric: labels.FromStrings("__name__", "test_metric_total"), m: metadata.Metadata{Type: "counter", Unit: "metric", Help: "different help text"}}, // Here, technically we should have no unit, but it's a known limitation of the current implementation.
		{metric: labels.FromStrings("__name__", "test_metric2", "foo", "bar"), m: metadata.Metadata{Type: "gauge", Unit: "metric2", Help: "other help text"}},
	}, capp.resultMetadata, []cmp.Option{cmp.Comparer(metadataEntryEqual)})
}

func TestScrapeReportMetadataUpdate_AppenderV1(t *testing.T) {
	// Create an appender for adding samples to the storage.
	capp := &collectResultAppenderV1{}
	sl := newBasicScrapeLoop(t, context.Background(), nopScraper{}, &collectResultAppendableV1{capp}, nil, 0)
	now := time.Now()
	slApp := sl.appender(context.Background())

	require.NoError(t, sl.report(slApp, now, 2*time.Second, 1, 1, 1, 512, nil))
	require.NoError(t, slApp.Commit())
	testutil.RequireEqualWithOptions(t, []metadataEntry{
		{metric: labels.FromStrings("__name__", "up"), m: scrapeHealthMetric.Metadata},
		{metric: labels.FromStrings("__name__", "scrape_duration_seconds"), m: scrapeDurationMetric.Metadata},
		{metric: labels.FromStrings("__name__", "scrape_samples_scraped"), m: scrapeSamplesMetric.Metadata},
		{metric: labels.FromStrings("__name__", "scrape_samples_post_metric_relabeling"), m: samplesPostRelabelMetric.Metadata},
		{metric: labels.FromStrings("__name__", "scrape_series_added"), m: scrapeSeriesAddedMetric.Metadata},
	}, capp.resultMetadata, []cmp.Option{cmp.Comparer(metadataEntryEqual)})
}

func TestScrapePoolAppender_AppenderV1(t *testing.T) {
	cfg := &config.ScrapeConfig{
		MetricNameValidationScheme: model.UTF8Validation,
		MetricNameEscapingScheme:   model.AllowUTF8,
	}
	app := &nopAppendableV1{}
	sp, _ := newScrapePool(cfg, app, nil, 0, nil, nil, &Options{}, newTestScrapeMetrics(t))

	loop := sp.newLoop(scrapeLoopOptions{
		target: &Target{},
	})
	appl, ok := loop.(*scrapeLoop)
	require.True(t, ok, "Expected scrapeLoop but got %T", loop)

	wrapped := appenderV1(appl.appendableV1.Appender(context.Background()), 0, 0, histogram.ExponentialSchemaMax)

	tl, ok := wrapped.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", wrapped)

	_, ok = tl.Appender.(nopAppenderV1)
	require.True(t, ok, "Expected base appender but got %T", tl.Appender)

	sampleLimit := 100
	loop = sp.newLoop(scrapeLoopOptions{
		target:      &Target{},
		sampleLimit: sampleLimit,
	})
	appl, ok = loop.(*scrapeLoop)
	require.True(t, ok, "Expected scrapeLoop but got %T", loop)

	wrapped = appenderV1(appl.appendableV1.Appender(context.Background()), sampleLimit, 0, histogram.ExponentialSchemaMax)

	sl, ok := wrapped.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", wrapped)

	tl, ok = sl.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", sl.Appender)

	_, ok = tl.Appender.(nopAppenderV1)
	require.True(t, ok, "Expected base appender but got %T", tl.Appender)

	wrapped = appenderV1(appl.appendableV1.Appender(context.Background()), sampleLimit, 100, histogram.ExponentialSchemaMax)

	bl, ok := wrapped.(*bucketLimitAppender)
	require.True(t, ok, "Expected bucketLimitAppender but got %T", wrapped)

	sl, ok = bl.Appender.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", bl)

	tl, ok = sl.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", sl.Appender)

	_, ok = tl.Appender.(nopAppenderV1)
	require.True(t, ok, "Expected base appender but got %T", tl.Appender)

	wrapped = appenderV1(appl.appendableV1.Appender(context.Background()), sampleLimit, 100, 0)

	ml, ok := wrapped.(*maxSchemaAppender)
	require.True(t, ok, "Expected maxSchemaAppender but got %T", wrapped)

	bl, ok = ml.Appender.(*bucketLimitAppender)
	require.True(t, ok, "Expected bucketLimitAppender but got %T", wrapped)

	sl, ok = bl.Appender.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", bl)

	tl, ok = sl.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", sl.Appender)

	_, ok = tl.Appender.(nopAppenderV1)
	require.True(t, ok, "Expected base appender but got %T", tl.Appender)
}

func TestScrapeLoopStop_AppenderV1(t *testing.T) {
	var (
		signal  = make(chan struct{}, 1)
		scraper = &testScraper{}
	)

	// Since we're writing samples directly below we need to provide a protocol fallback.
	capp := &collectResultAppenderV1{}
	sl := newBasicScrapeLoopWithFallback(t, context.Background(), scraper, &collectResultAppendableV1{capp}, nil, 10*time.Millisecond, "text/plain")

	// Terminate loop after 2 scrapes.
	numScrapes := 0

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++
		if numScrapes == 2 {
			go sl.stop()
			<-sl.ctx.Done()
		}
		w.Write([]byte("metric_a 42\n"))
		return ctx.Err()
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape wasn't stopped.")
	}

	// We expected 1 actual sample for each scrape plus 5 for report samples.
	// At least 2 scrapes were made, plus the final stale markers.
	require.GreaterOrEqual(t, len(capp.resultFloats), 6*3, "Expected at least 3 scrapes with 6 samples each.")
	require.Zero(t, len(capp.resultFloats)%6, "There is a scrape with missing samples.")
	// All samples in a scrape must have the same timestamp.
	var ts int64
	for i, s := range capp.resultFloats {
		switch {
		case i%6 == 0:
			ts = s.t
		case s.t != ts:
			t.Fatalf("Unexpected multiple timestamps within single scrape")
		}
	}
	// All samples from the last scrape must be stale markers.
	for _, s := range capp.resultFloats[len(capp.resultFloats)-5:] {
		require.True(t, value.IsStaleNaN(s.f), "Appended last sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(s.f))
	}
}

func TestScrapeLoopRun_AppenderV1(t *testing.T) {
	t.Parallel()
	var (
		signal = make(chan struct{}, 1)
		errc   = make(chan error)

		scraper       = &testScraper{}
		scrapeMetrics = newTestScrapeMetrics(t)
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		nopAppendableV1{},
		nil,
		nil,
		nil,
		0,
		true,
		false,
		true,
		0, 0, histogram.ExponentialSchemaMax,
		nil,
		time.Second,
		time.Hour,
		false,
		false,
		false,
		false,
		false,
		false,
		false,
		nil,
		false,
		scrapeMetrics,
		false,
		model.UTF8Validation,
		model.NoEscaping,
		"",
	)

	// The loop must terminate during the initial offset if the context
	// is canceled.
	scraper.offsetDur = time.Hour

	go func() {
		sl.run(errc)
		signal <- struct{}{}
	}()

	// Wait to make sure we are actually waiting on the offset.
	time.Sleep(1 * time.Second)

	cancel()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Cancellation during initial offset failed.")
	case err := <-errc:
		require.FailNow(t, "Unexpected error", "err: %s", err)
	}

	// The provided timeout must cause cancellation of the context passed down to the
	// scraper. The scraper has to respect the context.
	scraper.offsetDur = 0

	block := make(chan struct{})
	scraper.scrapeFunc = func(ctx context.Context, _ io.Writer) error {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}

	ctx, cancel = context.WithCancel(context.Background())
	sl = newBasicScrapeLoop(t, ctx, scraper, nopAppendableV1{}, nil, time.Second)
	sl.timeout = 100 * time.Millisecond

	go func() {
		sl.run(errc)
		signal <- struct{}{}
	}()

	select {
	case err := <-errc:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "Expected timeout error but got none.")
	}

	// We already caught the timeout error and are certainly in the loop.
	// Let the scrapes returns immediately to cause no further timeout errors
	// and check whether canceling the parent context terminates the loop.
	close(block)
	cancel()

	select {
	case <-signal:
		// Loop terminated as expected.
	case err := <-errc:
		require.FailNow(t, "Unexpected error", "err: %s", err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "Loop did not terminate on context cancellation")
	}
}

func TestScrapeLoopForcedErr_AppenderV1(t *testing.T) {
	var (
		signal = make(chan struct{}, 1)
		errc   = make(chan error)

		scraper = &testScraper{}
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newBasicScrapeLoop(t, ctx, scraper, nopAppendableV1{}, nil, time.Second)

	forcedErr := errors.New("forced err")
	sl.setForcedError(forcedErr)

	scraper.scrapeFunc = func(context.Context, io.Writer) error {
		require.FailNow(t, "Should not be scraped.")
		return nil
	}

	go func() {
		sl.run(errc)
		signal <- struct{}{}
	}()

	select {
	case err := <-errc:
		require.ErrorIs(t, err, forcedErr)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "Expected forced error but got none.")
	}
	cancel()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Scrape not stopped.")
	}
}

func TestScrapeLoopMetadata_AppenderV1(t *testing.T) {
	var (
		signal        = make(chan struct{})
		scraper       = &testScraper{}
		scrapeMetrics = newTestScrapeMetrics(t)
		cache         = newScrapeCache(scrapeMetrics)
	)
	defer close(signal)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		nopAppendableV1{},
		nil,
		cache,
		labels.NewSymbolTable(),
		0,
		true,
		false,
		true,
		0, 0, histogram.ExponentialSchemaMax,
		nil,
		0,
		0,
		false,
		false,
		false,
		false,
		false,
		false,
		false,
		nil,
		false,
		scrapeMetrics,
		false,
		model.UTF8Validation,
		model.NoEscaping,
		"",
	)
	defer cancel()

	slApp := sl.appender(ctx)
	total, _, _, err := slApp.append([]byte(`# TYPE test_metric counter
# HELP test_metric some help text
# UNIT test_metric metric
test_metric_total 1
# TYPE test_metric_no_help gauge
# HELP test_metric_no_type other help text
# EOF`), "application/openmetrics-text", time.Now())
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	require.Equal(t, 1, total)

	md, ok := cache.GetMetadata("test_metric")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, model.MetricTypeCounter, md.Type, "unexpected metric type")
	require.Equal(t, "some help text", md.Help)
	require.Equal(t, "metric", md.Unit)

	md, ok = cache.GetMetadata("test_metric_no_help")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, model.MetricTypeGauge, md.Type, "unexpected metric type")
	require.Empty(t, md.Help)
	require.Empty(t, md.Unit)

	md, ok = cache.GetMetadata("test_metric_no_type")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, model.MetricTypeUnknown, md.Type, "unexpected metric type")
	require.Equal(t, "other help text", md.Help)
	require.Empty(t, md.Unit)
}

func TestScrapeLoopSeriesAdded_AppenderV1(t *testing.T) {
	ctx, sl := simpleTestScrapeLoopV1(t)

	slApp := sl.appender(ctx)
	total, added, seriesAdded, err := slApp.append([]byte("test_metric 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 1, seriesAdded)

	slApp = sl.appender(ctx)
	total, added, seriesAdded, err = slApp.append([]byte("test_metric 1\n"), "text/plain", time.Time{})
	require.NoError(t, slApp.Commit())
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 0, seriesAdded)
}

func TestScrapeLoopFailWithInvalidLabelsAfterRelabel_AppenderV1(t *testing.T) {
	ctx := t.Context()

	target := &Target{
		labels: labels.FromStrings("pod_label_invalid_012\xff", "test"),
	}
	relabelConfig := []*relabel.Config{{
		Action:               relabel.LabelMap,
		Regex:                relabel.MustNewRegexp("pod_label_invalid_(.+)"),
		Separator:            ";",
		Replacement:          "$1",
		NameValidationScheme: model.UTF8Validation,
	}}
	ctx, sl := simpleTestScrapeLoopV1(t)
	sl.sampleMutator = func(l labels.Labels) labels.Labels {
		return mutateSampleLabels(l, target, true, relabelConfig)
	}

	slApp := sl.appender(ctx)
	total, added, seriesAdded, err := slApp.append([]byte("test_metric 1\n"), "text/plain", time.Time{})
	require.ErrorContains(t, err, "invalid metric name or label names")
	require.NoError(t, slApp.Rollback())
	require.Equal(t, 1, total)
	require.Equal(t, 0, added)
	require.Equal(t, 0, seriesAdded)
}

func TestScrapeLoopFailLegacyUnderUTF8_AppenderV1(t *testing.T) {
	ctx := t.Context()

	ctx, sl := simpleTestScrapeLoopV1(t)
	sl.validationScheme = model.LegacyValidation

	slApp := sl.appender(ctx)
	total, added, seriesAdded, err := slApp.append([]byte("{\"test.metric\"} 1\n"), "text/plain", time.Time{})
	require.ErrorContains(t, err, "invalid metric name or label names")
	require.NoError(t, slApp.Rollback())
	require.Equal(t, 1, total)
	require.Equal(t, 0, added)
	require.Equal(t, 0, seriesAdded)

	// When scrapeloop has validation set to UTF-8, the metric is allowed.
	sl.validationScheme = model.UTF8Validation

	slApp = sl.appender(ctx)
	total, added, seriesAdded, err = slApp.append([]byte("{\"test.metric\"} 1\n"), "text/plain", time.Time{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 1, seriesAdded)
}
*/
