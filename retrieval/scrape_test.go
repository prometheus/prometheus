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
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestNewScrapePool(t *testing.T) {
	var (
		app = &nopAppendable{}
		cfg = &config.ScrapeConfig{}
		sp  = newScrapePool(cfg, app, nil)
	)

	if a, ok := sp.appendable.(*nopAppendable); !ok || a != app {
		t.Fatalf("Wrong sample appender")
	}
	if sp.config != cfg {
		t.Fatalf("Wrong scrape config")
	}
	if sp.newLoop == nil {
		t.Fatalf("newLoop function not initialized")
	}
}

func TestDroppedTargetsList(t *testing.T) {
	var (
		app = &nopAppendable{}
		cfg = &config.ScrapeConfig{
			JobName:        "dropMe",
			ScrapeInterval: model.Duration(1),
			RelabelConfigs: []*config.RelabelConfig{
				{
					Action:       config.RelabelDrop,
					Regex:        mustNewRegexp("dropMe"),
					SourceLabels: model.LabelNames{"job"},
				},
			},
		}
		tgs = []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					model.LabelSet{model.AddressLabel: "127.0.0.1:9090"},
				},
			},
		}
		sp                     = newScrapePool(cfg, app, nil)
		expectedLabelSetString = "{__address__=\"127.0.0.1:9090\", __metrics_path__=\"\", __scheme__=\"\", job=\"dropMe\"}"
		expectedLength         = 1
	)
	sp.Sync(tgs)
	sp.Sync(tgs)
	if len(sp.droppedTargets) != expectedLength {
		t.Fatalf("Length of dropped targets exceeded expected length, expected %v, got %v", expectedLength, len(sp.droppedTargets))
	}
	if sp.droppedTargets[0].DiscoveredLabels().String() != expectedLabelSetString {
		t.Fatalf("Got %v, expected %v", sp.droppedTargets[0].DiscoveredLabels().String(), expectedLabelSetString)
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
		cancel:  func() {},
	}
	var mtx sync.Mutex
	stopped := map[uint64]bool{}
	numTargets := 20

	// Stopping the scrape pool must call stop() on all scrape loops,
	// clean them and the respective targets up. It must wait until each loop's
	// stop function returned before returning itself.

	for i := 0; i < numTargets; i++ {
		t := &Target{
			labels: labels.FromStrings(model.AddressLabel, fmt.Sprintf("example.com:%d", i)),
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
	// On starting to run, new loops created on reload check whether their preceding
	// equivalents have been stopped.
	newLoop := func(_ *Target, s scraper) loop {
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
		appendable: &nopAppendable{},
		targets:    map[uint64]*Target{},
		loops:      map[uint64]loop{},
		newLoop:    newLoop,
		logger:     nil,
	}

	// Reloading a scrape pool with a new scrape configuration must stop all scrape
	// loops and start new ones. A new loop must not be started before the preceding
	// one terminated.

	for i := 0; i < numTargets; i++ {
		t := &Target{
			labels: labels.FromStrings(model.AddressLabel, fmt.Sprintf("example.com:%d", i)),
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

func TestScrapePoolAppender(t *testing.T) {
	cfg := &config.ScrapeConfig{}
	app := &nopAppendable{}
	sp := newScrapePool(cfg, app, nil)

	wrapped := sp.appender()

	tl, ok := wrapped.(*timeLimitAppender)
	if !ok {
		t.Fatalf("Expected timeLimitAppender but got %T", wrapped)
	}
	if _, ok := tl.Appender.(nopAppender); !ok {
		t.Fatalf("Expected base appender but got %T", tl.Appender)
	}

	cfg.SampleLimit = 100

	wrapped = sp.appender()

	sl, ok := wrapped.(*limitAppender)
	if !ok {
		t.Fatalf("Expected limitAppender but got %T", wrapped)
	}
	tl, ok = sl.Appender.(*timeLimitAppender)
	if !ok {
		t.Fatalf("Expected limitAppender but got %T", sl.Appender)
	}
	if _, ok := tl.Appender.(nopAppender); !ok {
		t.Fatalf("Expected base appender but got %T", tl.Appender)
	}
}

func TestScrapeLoopStopBeforeRun(t *testing.T) {
	scraper := &testScraper{}

	sl := newScrapeLoop(context.Background(),
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		nil,
	)

	// The scrape pool synchronizes on stopping scrape loops. However, new scrape
	// loops are started asynchronously. Thus it's possible, that a loop is stopped
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
	scraper.scrapeFunc = func(context.Context, io.Writer) error {
		t.Fatalf("scraper was called for terminated scrape loop")
		return nil
	}

	runDone := make(chan struct{})
	go func() {
		sl.run(1, 0, nil)
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

func nopMutator(l labels.Labels) labels.Labels { return l }

func TestScrapeLoopStop(t *testing.T) {
	var (
		signal   = make(chan struct{})
		appender = &collectResultAppender{}
		scraper  = &testScraper{}
		app      = func() storage.Appender { return appender }
	)
	defer close(signal)

	sl := newScrapeLoop(context.Background(),
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
	)

	// Terminate loop after 2 scrapes.
	numScrapes := 0

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++
		if numScrapes == 2 {
			go sl.stop()
		}
		w.Write([]byte("metric_a 42\n"))
		return nil
	}

	go func() {
		sl.run(10*time.Millisecond, time.Hour, nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	// We expected 1 actual sample for each scrape plus 4 for report samples.
	// At least 2 scrapes were made, plus the final stale markers.
	if len(appender.result) < 5*3 || len(appender.result)%5 != 0 {
		t.Fatalf("Expected at least 3 scrapes with 4 samples each, got %d samples", len(appender.result))
	}
	// All samples in a scrape must have the same timestmap.
	var ts int64
	for i, s := range appender.result {
		if i%5 == 0 {
			ts = s.t
		} else if s.t != ts {
			t.Fatalf("Unexpected multiple timestamps within single scrape")
		}
	}
	// All samples from the last scrape must be stale markers.
	for _, s := range appender.result[len(appender.result)-5:] {
		if !value.IsStaleNaN(s.v) {
			t.Fatalf("Appended last sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(s.v))
		}
	}
}

func TestScrapeLoopRun(t *testing.T) {
	var (
		signal = make(chan struct{})
		errc   = make(chan error)

		scraper = &testScraper{}
		app     = func() storage.Appender { return &nopAppender{} }
	)
	defer close(signal)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
	)

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
	scraper.scrapeFunc = func(ctx context.Context, _ io.Writer) error {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}

	ctx, cancel = context.WithCancel(context.Background())
	sl = newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
	)

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

func TestScrapeLoopRunCreatesStaleMarkersOnFailedScrape(t *testing.T) {
	appender := &collectResultAppender{}
	var (
		signal  = make(chan struct{})
		scraper = &testScraper{}
		app     = func() storage.Appender { return appender }
	)
	defer close(signal)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
	)
	// Succeed once, several failures, then stop.
	numScrapes := 0

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++

		if numScrapes == 1 {
			w.Write([]byte("metric_a 42\n"))
			return nil
		} else if numScrapes == 5 {
			cancel()
		}
		return fmt.Errorf("scrape failed")
	}

	go func() {
		sl.run(10*time.Millisecond, time.Hour, nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	// 1 successfully scraped sample, 1 stale marker after first fail, 4 report samples for
	// each scrape successful or not.
	if len(appender.result) != 22 {
		t.Fatalf("Appended samples not as expected. Wanted: %d samples Got: %d", 22, len(appender.result))
	}
	if appender.result[0].v != 42.0 {
		t.Fatalf("Appended first sample not as expected. Wanted: %f Got: %f", appender.result[0].v, 42.0)
	}
	if !value.IsStaleNaN(appender.result[5].v) {
		t.Fatalf("Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(appender.result[5].v))
	}
}

func TestScrapeLoopRunCreatesStaleMarkersOnParseFailure(t *testing.T) {
	appender := &collectResultAppender{}
	var (
		signal     = make(chan struct{})
		scraper    = &testScraper{}
		app        = func() storage.Appender { return appender }
		numScrapes = 0
	)
	defer close(signal)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
	)

	// Succeed once, several failures, then stop.
	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++

		if numScrapes == 1 {
			w.Write([]byte("metric_a 42\n"))
			return nil
		} else if numScrapes == 2 {
			w.Write([]byte("7&-\n"))
			return nil
		} else if numScrapes == 3 {
			cancel()
		}
		return fmt.Errorf("scrape failed")
	}

	go func() {
		sl.run(10*time.Millisecond, time.Hour, nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	// 1 successfully scraped sample, 1 stale marker after first fail, 4 report samples for
	// each scrape successful or not.
	if len(appender.result) != 14 {
		t.Fatalf("Appended samples not as expected. Wanted: %d samples Got: %d", 22, len(appender.result))
	}
	if appender.result[0].v != 42.0 {
		t.Fatalf("Appended first sample not as expected. Wanted: %f Got: %f", appender.result[0].v, 42.0)
	}
	if !value.IsStaleNaN(appender.result[5].v) {
		t.Fatalf("Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(appender.result[5].v))
	}
}

func TestScrapeLoopAppend(t *testing.T) {
	app := &collectResultAppender{}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func() storage.Appender { return app },
	)

	now := time.Now()
	_, _, err := sl.append([]byte("metric_a 1\nmetric_b NaN\n"), now)
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}

	ingestedNaN := math.Float64bits(app.result[1].v)
	if ingestedNaN != value.NormalNaN {
		t.Fatalf("Appended NaN samples wasn't as expected. Wanted: %x Got: %x", value.NormalNaN, ingestedNaN)
	}

	// DeepEqual will report NaNs as being different, so replace with a different value.
	app.result[1].v = 42
	want := []sample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      timestamp.FromTime(now),
			v:      1,
		},
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_b"),
			t:      timestamp.FromTime(now),
			v:      42,
		},
	}
	if !reflect.DeepEqual(want, app.result) {
		t.Fatalf("Appended samples not as expected. Wanted: %+v Got: %+v", want, app.result)
	}
}

func TestScrapeLoopAppendSampleLimit(t *testing.T) {
	resApp := &collectResultAppender{}
	app := &limitAppender{Appender: resApp, limit: 1}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func() storage.Appender { return app },
	)

	// Get the value of the Counter before performing the append.
	beforeMetric := dto.Metric{}
	err := targetScrapeSampleLimit.Write(&beforeMetric)
	if err != nil {
		t.Fatal(err)
	}
	beforeMetricValue := beforeMetric.GetCounter().GetValue()

	now := time.Now()
	_, _, err = sl.append([]byte("metric_a 1\nmetric_b 1\nmetric_c 1\n"), now)
	if err != errSampleLimit {
		t.Fatalf("Did not see expected sample limit error: %s", err)
	}

	// Check that the Counter has been incremented a simgle time for the scrape,
	// not multiple times for each sample.
	metric := dto.Metric{}
	err = targetScrapeSampleLimit.Write(&metric)
	if err != nil {
		t.Fatal(err)
	}
	value := metric.GetCounter().GetValue()
	if (value - beforeMetricValue) != 1 {
		t.Fatal("Unexpected change of sample limit metric: %f", (value - beforeMetricValue))
	}

	// And verify that we got the samples that fit under the limit.
	want := []sample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      timestamp.FromTime(now),
			v:      1,
		},
	}
	if !reflect.DeepEqual(want, resApp.result) {
		t.Fatalf("Appended samples not as expected. Wanted: %+v Got: %+v", want, resApp.result)
	}
}

func TestScrapeLoop_ChangingMetricString(t *testing.T) {
	// This is a regression test for the scrape loop cache not properly maintaining
	// IDs when the string representation of a metric changes across a scrape. Thus
	// we use a real storage appender here.
	s := testutil.NewStorage(t)
	defer s.Close()

	app, err := s.Appender()
	if err != nil {
		t.Error(err)
	}
	capp := &collectResultAppender{next: app}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func() storage.Appender { return capp },
	)

	now := time.Now()
	_, _, err = sl.append([]byte(`metric_a{a="1",b="1"} 1`), now)
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}
	_, _, err = sl.append([]byte(`metric_a{b="1",a="1"} 2`), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}

	// DeepEqual will report NaNs as being different, so replace with a different value.
	want := []sample{
		{
			metric: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			t:      timestamp.FromTime(now),
			v:      1,
		},
		{
			metric: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			t:      timestamp.FromTime(now.Add(time.Minute)),
			v:      2,
		},
	}
	if !reflect.DeepEqual(want, capp.result) {
		t.Fatalf("Appended samples not as expected. Wanted: %+v Got: %+v", want, capp.result)
	}
}

func TestScrapeLoopAppendStaleness(t *testing.T) {
	app := &collectResultAppender{}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func() storage.Appender { return app },
	)

	now := time.Now()
	_, _, err := sl.append([]byte("metric_a 1\n"), now)
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}
	_, _, err = sl.append([]byte(""), now.Add(time.Second))
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}

	ingestedNaN := math.Float64bits(app.result[1].v)
	if ingestedNaN != value.StaleNaN {
		t.Fatalf("Appended stale sample wasn't as expected. Wanted: %x Got: %x", value.StaleNaN, ingestedNaN)
	}

	// DeepEqual will report NaNs as being different, so replace with a different value.
	app.result[1].v = 42
	want := []sample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      timestamp.FromTime(now),
			v:      1,
		},
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      timestamp.FromTime(now.Add(time.Second)),
			v:      42,
		},
	}
	if !reflect.DeepEqual(want, app.result) {
		t.Fatalf("Appended samples not as expected. Wanted: %+v Got: %+v", want, app.result)
	}

}

func TestScrapeLoopAppendNoStalenessIfTimestamp(t *testing.T) {
	app := &collectResultAppender{}
	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func() storage.Appender { return app },
	)

	now := time.Now()
	_, _, err := sl.append([]byte("metric_a 1 1000\n"), now)
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}
	_, _, err = sl.append([]byte(""), now.Add(time.Second))
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}

	want := []sample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      1000,
			v:      1,
		},
	}
	if !reflect.DeepEqual(want, app.result) {
		t.Fatalf("Appended samples not as expected. Wanted: %+v Got: %+v", want, app.result)
	}
}

func TestScrapeLoopRunReportsTargetDownOnScrapeError(t *testing.T) {
	var (
		scraper  = &testScraper{}
		appender = &collectResultAppender{}
		app      = func() storage.Appender { return appender }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
	)

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		cancel()
		return fmt.Errorf("scrape failed")
	}

	sl.run(10*time.Millisecond, time.Hour, nil)

	if appender.result[0].v != 0 {
		t.Fatalf("bad 'up' value; want 0, got %v", appender.result[0].v)
	}
}

func TestScrapeLoopRunReportsTargetDownOnInvalidUTF8(t *testing.T) {
	var (
		scraper  = &testScraper{}
		appender = &collectResultAppender{}
		app      = func() storage.Appender { return appender }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
	)

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		cancel()
		w.Write([]byte("a{l=\"\xff\"} 1\n"))
		return nil
	}

	sl.run(10*time.Millisecond, time.Hour, nil)

	if appender.result[0].v != 0 {
		t.Fatalf("bad 'up' value; want 0, got %v", appender.result[0].v)
	}
}

type errorAppender struct {
	collectResultAppender
}

func (app *errorAppender) Add(lset labels.Labels, t int64, v float64) (uint64, error) {
	switch lset.Get(model.MetricNameLabel) {
	case "out_of_order":
		return 0, storage.ErrOutOfOrderSample
	case "amend":
		return 0, storage.ErrDuplicateSampleForTimestamp
	case "out_of_bounds":
		return 0, storage.ErrOutOfBounds
	default:
		return app.collectResultAppender.Add(lset, t, v)
	}
}

func (app *errorAppender) AddFast(lset labels.Labels, ref uint64, t int64, v float64) error {
	return app.collectResultAppender.AddFast(lset, ref, t, v)
}

func TestScrapeLoopAppendGracefullyIfAmendOrOutOfOrderOrOutOfBounds(t *testing.T) {
	app := &errorAppender{}

	sl := newScrapeLoop(context.Background(),
		nil,
		nil, nil,
		nopMutator,
		nopMutator,
		func() storage.Appender { return app },
	)

	now := time.Unix(1, 0)
	_, _, err := sl.append([]byte("out_of_order 1\namend 1\nnormal 1\nout_of_bounds 1\n"), now)
	if err != nil {
		t.Fatalf("Unexpected append error: %s", err)
	}
	want := []sample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "normal"),
			t:      timestamp.FromTime(now),
			v:      1,
		},
	}
	if !reflect.DeepEqual(want, app.result) {
		t.Fatalf("Appended samples not as expected. Wanted: %+v Got: %+v", want, app.result)
	}
}

func TestScrapeLoopOutOfBoundsTimeError(t *testing.T) {
	app := &collectResultAppender{}
	sl := newScrapeLoop(context.Background(),
		nil,
		nil, nil,
		nopMutator,
		nopMutator,
		func() storage.Appender {
			return &timeLimitAppender{
				Appender: app,
				maxTime:  timestamp.FromTime(time.Now().Add(10 * time.Minute)),
			}
		},
	)

	now := time.Now().Add(20 * time.Minute)
	total, added, err := sl.append([]byte("normal 1\n"), now)
	if total != 1 {
		t.Error("expected 1 metric")
		return
	}

	if added != 0 {
		t.Error("no metric should be added")
	}

	if err != nil {
		t.Errorf("expect no error, got %s", err.Error())
	}
}

func TestTargetScraperScrapeOK(t *testing.T) {
	const (
		configTimeout   = 1500 * time.Millisecond
		expectedTimeout = "1.500000"
	)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			accept := r.Header.Get("Accept")
			if !strings.HasPrefix(accept, "text/plain;") {
				t.Errorf("Expected Accept header to prefer text/plain, got %q", accept)
			}

			timeout := r.Header.Get("X-Prometheus-Scrape-Timeout-Seconds")
			if timeout != expectedTimeout {
				t.Errorf("Expected scrape timeout header %q, got %q", expectedTimeout, timeout)
			}

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
			labels: labels.FromStrings(
				model.SchemeLabel, serverURL.Scheme,
				model.AddressLabel, serverURL.Host,
			),
		},
		client:  http.DefaultClient,
		timeout: configTimeout,
	}
	var buf bytes.Buffer

	if err := ts.scrape(context.Background(), &buf); err != nil {
		t.Fatalf("Unexpected scrape error: %s", err)
	}
	require.Equal(t, "metric_a 1\nmetric_b 2\n", buf.String())
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
			labels: labels.FromStrings(
				model.SchemeLabel, serverURL.Scheme,
				model.AddressLabel, serverURL.Host,
			),
		},
		client: http.DefaultClient,
	}
	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error)

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	go func() {
		if err := ts.scrape(ctx, ioutil.Discard); err != context.Canceled {
			errc <- fmt.Errorf("Expected context cancelation error but got: %s", err)
		}
		close(errc)
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape function did not return unexpectedly")
	case err := <-errc:
		if err != nil {
			t.Fatalf(err.Error())
		}
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
			labels: labels.FromStrings(
				model.SchemeLabel, serverURL.Scheme,
				model.AddressLabel, serverURL.Host,
			),
		},
		client: http.DefaultClient,
	}

	if err := ts.scrape(context.Background(), ioutil.Discard); !strings.Contains(err.Error(), "404") {
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

	scrapeErr  error
	scrapeFunc func(context.Context, io.Writer) error
}

func (ts *testScraper) offset(interval time.Duration) time.Duration {
	return ts.offsetDur
}

func (ts *testScraper) report(start time.Time, duration time.Duration, err error) {
	ts.lastStart = start
	ts.lastDuration = duration
	ts.lastError = err
}

func (ts *testScraper) scrape(ctx context.Context, w io.Writer) error {
	if ts.scrapeFunc != nil {
		return ts.scrapeFunc(ctx, w)
	}
	return ts.scrapeErr
}
