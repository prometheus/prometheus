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
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
)

func TestNewScrapePool(t *testing.T) {
	var (
		app = &nopAppender{}
		cfg = &config.ScrapeConfig{}
		sp  = newScrapePool(cfg, app)
	)

	if a, ok := sp.appender.(*nopAppender); !ok || a != app {
		t.Fatalf("Wrong sample appender")
	}
	if sp.config != cfg {
		t.Fatalf("Wrong scrape config")
	}
	if sp.newLoop == nil {
		t.Fatalf("newLoop function not initialized")
	}
}

type testLoop struct {
	startFunc func(interval, timeout time.Duration, errc chan<- error)
	stopFunc  func()
}

func (l *testLoop) run(interval, timeout time.Duration, errc chan<- error) {
	l.startFunc(interval, timeout, errc)
}

func (l *testLoop) stop() {
	l.stopFunc()
}

func TestScrapePoolStop(t *testing.T) {
	sp := &scrapePool{
		targets: map[uint64]*Target{},
		loops:   map[uint64]loop{},
	}
	var mtx sync.Mutex
	stopped := map[uint64]bool{}
	numTargets := 20

	// Stopping the scrape pool must call stop() on all scrape loops,
	// clean them and the respective targets up. It must wait until each loop's
	// stop function returned before returning itself.

	for i := 0; i < numTargets; i++ {
		t := &Target{
			labels: model.LabelSet{
				model.AddressLabel: model.LabelValue(fmt.Sprintf("example.com:%d", i)),
			},
		}
		l := &testLoop{}
		l.stopFunc = func() {
			time.Sleep(time.Duration(i*20) * time.Millisecond)

			mtx.Lock()
			stopped[t.hash()] = true
			mtx.Unlock()
		}

		sp.targets[t.hash()] = t
		sp.loops[t.hash()] = l
	}

	done := make(chan struct{})
	stopTime := time.Now()

	go func() {
		sp.stop()
		close(done)
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("scrapeLoop.stop() did not return as expected")
	case <-done:
		// This should have taken at least as long as the last target slept.
		if time.Since(stopTime) < time.Duration(numTargets*20)*time.Millisecond {
			t.Fatalf("scrapeLoop.stop() exited before all targets stopped")
		}
	}

	mtx.Lock()
	if len(stopped) != numTargets {
		t.Fatalf("Expected 20 stopped loops, got %d", len(stopped))
	}
	mtx.Unlock()

	if len(sp.targets) > 0 {
		t.Fatalf("Targets were not cleared on stopping: %d left", len(sp.targets))
	}
	if len(sp.loops) > 0 {
		t.Fatalf("Loops were not cleared on stopping: %d left", len(sp.loops))
	}
}

func TestScrapePoolReload(t *testing.T) {
	var mtx sync.Mutex
	numTargets := 20

	stopped := map[uint64]bool{}

	reloadCfg := &config.ScrapeConfig{
		ScrapeInterval: model.Duration(3 * time.Second),
		ScrapeTimeout:  model.Duration(2 * time.Second),
	}
	// On starting to run, new loops created on reload check whether their preceeding
	// equivalents have been stopped.
	newLoop := func(ctx context.Context, s scraper, app, reportApp storage.SampleAppender) loop {
		l := &testLoop{}
		l.startFunc = func(interval, timeout time.Duration, errc chan<- error) {
			if interval != 3*time.Second {
				t.Errorf("Expected scrape interval %d but got %d", 3*time.Second, interval)
			}
			if timeout != 2*time.Second {
				t.Errorf("Expected scrape timeout %d but got %d", 2*time.Second, timeout)
			}
			mtx.Lock()
			if !stopped[s.(*targetScraper).hash()] {
				t.Errorf("Scrape loop for %v not stopped yet", s.(*targetScraper))
			}
			mtx.Unlock()
		}
		return l
	}
	sp := &scrapePool{
		targets: map[uint64]*Target{},
		loops:   map[uint64]loop{},
		newLoop: newLoop,
	}

	// Reloading a scrape pool with a new scrape configuration must stop all scrape
	// loops and start new ones. A new loop must not be started before the preceeding
	// one terminated.

	for i := 0; i < numTargets; i++ {
		t := &Target{
			labels: model.LabelSet{
				model.AddressLabel: model.LabelValue(fmt.Sprintf("example.com:%d", i)),
			},
		}
		l := &testLoop{}
		l.stopFunc = func() {
			time.Sleep(time.Duration(i*20) * time.Millisecond)

			mtx.Lock()
			stopped[t.hash()] = true
			mtx.Unlock()
		}

		sp.targets[t.hash()] = t
		sp.loops[t.hash()] = l
	}
	done := make(chan struct{})

	beforeTargets := map[uint64]*Target{}
	for h, t := range sp.targets {
		beforeTargets[h] = t
	}

	reloadTime := time.Now()

	go func() {
		sp.reload(reloadCfg)
		close(done)
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("scrapeLoop.reload() did not return as expected")
	case <-done:
		// This should have taken at least as long as the last target slept.
		if time.Since(reloadTime) < time.Duration(numTargets*20)*time.Millisecond {
			t.Fatalf("scrapeLoop.stop() exited before all targets stopped")
		}
	}

	mtx.Lock()
	if len(stopped) != numTargets {
		t.Fatalf("Expected 20 stopped loops, got %d", len(stopped))
	}
	mtx.Unlock()

	if !reflect.DeepEqual(sp.targets, beforeTargets) {
		t.Fatalf("Reloading affected target states unexpectedly")
	}
	if len(sp.loops) != numTargets {
		t.Fatalf("Expected %d loops after reload but got %d", numTargets, len(sp.loops))
	}
}

func TestScrapePoolReportAppender(t *testing.T) {
	cfg := &config.ScrapeConfig{
		MetricRelabelConfigs: []*config.RelabelConfig{
			{}, {}, {},
		},
	}
	target := newTestTarget("example.com:80", 10*time.Millisecond, nil)
	app := &nopAppender{}

	sp := newScrapePool(cfg, app)

	cfg.HonorLabels = false
	wrapped := sp.reportAppender(target)

	rl, ok := wrapped.(ruleLabelsAppender)
	if !ok {
		t.Fatalf("Expected ruleLabelsAppender but got %T", wrapped)
	}
	if rl.SampleAppender != app {
		t.Fatalf("Expected base appender but got %T", rl.SampleAppender)
	}

	cfg.HonorLabels = true
	wrapped = sp.reportAppender(target)

	hl, ok := wrapped.(ruleLabelsAppender)
	if !ok {
		t.Fatalf("Expected ruleLabelsAppender but got %T", wrapped)
	}
	if hl.SampleAppender != app {
		t.Fatalf("Expected base appender but got %T", hl.SampleAppender)
	}
}

func TestScrapePoolSampleAppender(t *testing.T) {
	cfg := &config.ScrapeConfig{
		MetricRelabelConfigs: []*config.RelabelConfig{
			{}, {}, {},
		},
	}

	target := newTestTarget("example.com:80", 10*time.Millisecond, nil)
	app := &nopAppender{}

	sp := newScrapePool(cfg, app)

	cfg.HonorLabels = false
	wrapped := sp.sampleAppender(target)

	rl, ok := wrapped.(ruleLabelsAppender)
	if !ok {
		t.Fatalf("Expected ruleLabelsAppender but got %T", wrapped)
	}
	re, ok := rl.SampleAppender.(relabelAppender)
	if !ok {
		t.Fatalf("Expected relabelAppender but got %T", rl.SampleAppender)
	}
	if re.SampleAppender != app {
		t.Fatalf("Expected base appender but got %T", re.SampleAppender)
	}

	cfg.HonorLabels = true
	wrapped = sp.sampleAppender(target)

	hl, ok := wrapped.(honorLabelsAppender)
	if !ok {
		t.Fatalf("Expected honorLabelsAppender but got %T", wrapped)
	}
	re, ok = hl.SampleAppender.(relabelAppender)
	if !ok {
		t.Fatalf("Expected relabelAppender but got %T", hl.SampleAppender)
	}
	if re.SampleAppender != app {
		t.Fatalf("Expected base appender but got %T", re.SampleAppender)
	}
}

func TestScrapeLoopStop(t *testing.T) {
	scraper := &testScraper{}
	sl := newScrapeLoop(context.Background(), scraper, nil, nil)

	// The scrape pool synchronizes on stopping scrape loops. However, new scrape
	// loops are syarted asynchronously. Thus it's possible, that a loop is stopped
	// again before having started properly.
	// Stopping not-yet-started loops must block until the run method was called and exited.
	// The run method must exit immediately.

	stopDone := make(chan struct{})
	go func() {
		sl.stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		t.Fatalf("Stopping terminated before run exited successfully")
	case <-time.After(500 * time.Millisecond):
	}

	// Running the scrape loop must exit before calling the scraper even once.
	scraper.scrapeFunc = func(context.Context, time.Time) (model.Samples, error) {
		t.Fatalf("scraper was called for terminated scrape loop")
		return nil, nil
	}

	runDone := make(chan struct{})
	go func() {
		sl.run(0, 0, nil)
		close(runDone)
	}()

	select {
	case <-runDone:
	case <-time.After(1 * time.Second):
		t.Fatalf("Running terminated scrape loop did not exit")
	}

	select {
	case <-stopDone:
	case <-time.After(1 * time.Second):
		t.Fatalf("Stopping did not terminate after running exited")
	}
}

func TestScrapeLoopRun(t *testing.T) {
	var (
		signal = make(chan struct{})
		errc   = make(chan error)

		scraper   = &testScraper{}
		app       = &nopAppender{}
		reportApp = &nopAppender{}
	)
	defer close(signal)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx, scraper, app, reportApp)

	// The loop must terminate during the initial offset if the context
	// is canceled.
	scraper.offsetDur = time.Hour

	go func() {
		sl.run(time.Second, time.Hour, errc)
		signal <- struct{}{}
	}()

	// Wait to make sure we are actually waiting on the offset.
	time.Sleep(1 * time.Second)

	cancel()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Cancelation during initial offset failed")
	case err := <-errc:
		t.Fatalf("Unexpected error: %s", err)
	}

	// The provided timeout must cause cancelation of the context passed down to the
	// scraper. The scraper has to respect the context.
	scraper.offsetDur = 0

	block := make(chan struct{})
	scraper.scrapeFunc = func(ctx context.Context, ts time.Time) (model.Samples, error) {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, nil
	}

	ctx, cancel = context.WithCancel(context.Background())
	sl = newScrapeLoop(ctx, scraper, app, reportApp)

	go func() {
		sl.run(time.Second, 100*time.Millisecond, errc)
		signal <- struct{}{}
	}()

	select {
	case err := <-errc:
		if err != context.DeadlineExceeded {
			t.Fatalf("Expected timeout error but got: %s", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Expected timeout error but got none")
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
		t.Fatalf("Unexpected error: %s", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("Loop did not terminate on context cancelation")
	}
}

func TestTargetScraperScrapeOK(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
			w.Write([]byte("metric_a 1\nmetric_b 2\n"))
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: &Target{
			labels: model.LabelSet{
				model.SchemeLabel:  model.LabelValue(serverURL.Scheme),
				model.AddressLabel: model.LabelValue(serverURL.Host),
			},
		},
		client: http.DefaultClient,
	}
	now := time.Now()

	samples, err := ts.scrape(context.Background(), now)
	if err != nil {
		t.Fatalf("Unexpected scrape error: %s", err)
	}

	expectedSamples := model.Samples{
		{
			Metric:    model.Metric{"__name__": "metric_a"},
			Timestamp: model.TimeFromUnixNano(now.UnixNano()),
			Value:     1,
		},
		{
			Metric:    model.Metric{"__name__": "metric_b"},
			Timestamp: model.TimeFromUnixNano(now.UnixNano()),
			Value:     2,
		},
	}
	sort.Sort(expectedSamples)
	sort.Sort(samples)

	if !reflect.DeepEqual(samples, expectedSamples) {
		t.Errorf("Scraped samples did not match served metrics")
		t.Errorf("Expected: %v", expectedSamples)
		t.Fatalf("Got: %v", samples)
	}
}

func TestTargetScrapeScrapeCancel(t *testing.T) {
	block := make(chan struct{})

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-block
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: &Target{
			labels: model.LabelSet{
				model.SchemeLabel:  model.LabelValue(serverURL.Scheme),
				model.AddressLabel: model.LabelValue(serverURL.Host),
			},
		},
		client: http.DefaultClient,
	}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	go func() {
		if _, err := ts.scrape(ctx, time.Now()); err != context.Canceled {
			t.Fatalf("Expected context cancelation error but got: %s", err)
		}
		close(done)
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape function did not return unexpectedly")
	case <-done:
	}
	// If this is closed in a defer above the function the test server
	// does not terminate and the test doens't complete.
	close(block)
}

func TestTargetScrapeScrapeNotFound(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}

	ts := &targetScraper{
		Target: &Target{
			labels: model.LabelSet{
				model.SchemeLabel:  model.LabelValue(serverURL.Scheme),
				model.AddressLabel: model.LabelValue(serverURL.Host),
			},
		},
		client: http.DefaultClient,
	}

	if _, err := ts.scrape(context.Background(), time.Now()); !strings.Contains(err.Error(), "404") {
		t.Fatalf("Expected \"404 NotFound\" error but got: %s", err)
	}
}

// testScraper implements the scraper interface and allows setting values
// returned by its methods. It also allows setting a custom scrape function.
type testScraper struct {
	offsetDur time.Duration

	lastStart    time.Time
	lastDuration time.Duration
	lastError    error

	samples    model.Samples
	scrapeErr  error
	scrapeFunc func(context.Context, time.Time) (model.Samples, error)
}

func (ts *testScraper) offset(interval time.Duration) time.Duration {
	return ts.offsetDur
}

func (ts *testScraper) report(start time.Time, duration time.Duration, err error) {
	ts.lastStart = start
	ts.lastDuration = duration
	ts.lastError = err
}

func (ts *testScraper) scrape(ctx context.Context, t time.Time) (model.Samples, error) {
	if ts.scrapeFunc != nil {
		return ts.scrapeFunc(ctx, t)
	}
	return ts.samples, ts.scrapeErr
}
