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

package scrape

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeak(m)
}

func TestNewScrapePool(t *testing.T) {
	var (
		app   = &nopAppendable{}
		cfg   = &config.ScrapeConfig{}
		sp, _ = newScrapePool(cfg, app, 0, nil, &Options{})
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
			RelabelConfigs: []*relabel.Config{
				{
					Action:       relabel.Drop,
					Regex:        relabel.MustNewRegexp("dropMe"),
					SourceLabels: model.LabelNames{"job"},
				},
			},
		}
		tgs = []*targetgroup.Group{
			{
				Targets: []model.LabelSet{
					{model.AddressLabel: "127.0.0.1:9090"},
				},
			},
		}
		sp, _                  = newScrapePool(cfg, app, 0, nil, &Options{})
		expectedLabelSetString = "{__address__=\"127.0.0.1:9090\", __scrape_interval__=\"0s\", __scrape_timeout__=\"0s\", job=\"dropMe\"}"
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

// TestDiscoveredLabelsUpdate checks that DiscoveredLabels are updated
// even when new labels don't affect the target `hash`.
func TestDiscoveredLabelsUpdate(t *testing.T) {
	sp := &scrapePool{}
	// These are used when syncing so need this to avoid a panic.
	sp.config = &config.ScrapeConfig{
		ScrapeInterval: model.Duration(1),
		ScrapeTimeout:  model.Duration(1),
	}
	sp.activeTargets = make(map[uint64]*Target)
	t1 := &Target{
		discoveredLabels: labels.FromStrings("label", "name"),
	}
	sp.activeTargets[t1.hash()] = t1

	t2 := &Target{
		discoveredLabels: labels.FromStrings("labelNew", "nameNew"),
	}
	sp.sync([]*Target{t2})

	require.Equal(t, t2.DiscoveredLabels(), sp.activeTargets[t1.hash()].DiscoveredLabels())
}

type testLoop struct {
	startFunc    func(interval, timeout time.Duration, errc chan<- error)
	stopFunc     func()
	forcedErr    error
	forcedErrMtx sync.Mutex
	runOnce      bool
	interval     time.Duration
	timeout      time.Duration
}

func (l *testLoop) run(errc chan<- error) {
	if l.runOnce {
		panic("loop must be started only once")
	}
	l.runOnce = true
	l.startFunc(l.interval, l.timeout, errc)
}

func (l *testLoop) disableEndOfRunStalenessMarkers() {
}

func (l *testLoop) setForcedError(err error) {
	l.forcedErrMtx.Lock()
	defer l.forcedErrMtx.Unlock()
	l.forcedErr = err
}

func (l *testLoop) getForcedError() error {
	l.forcedErrMtx.Lock()
	defer l.forcedErrMtx.Unlock()
	return l.forcedErr
}

func (l *testLoop) stop() {
	l.stopFunc()
}

func (l *testLoop) getCache() *scrapeCache {
	return nil
}

func TestScrapePoolStop(t *testing.T) {
	sp := &scrapePool{
		activeTargets: map[uint64]*Target{},
		loops:         map[uint64]loop{},
		cancel:        func() {},
		client:        http.DefaultClient,
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
		d := time.Duration((i+1)*20) * time.Millisecond
		l.stopFunc = func() {
			time.Sleep(d)

			mtx.Lock()
			stopped[t.hash()] = true
			mtx.Unlock()
		}

		sp.activeTargets[t.hash()] = t
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
	require.Equal(t, numTargets, len(stopped), "Unexpected number of stopped loops")
	mtx.Unlock()

	require.Equal(t, 0, len(sp.activeTargets), "Targets were not cleared on stopping: %d left", len(sp.activeTargets))
	require.Equal(t, 0, len(sp.loops), "Loops were not cleared on stopping: %d left", len(sp.loops))
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
	newLoop := func(opts scrapeLoopOptions) loop {
		l := &testLoop{interval: time.Duration(reloadCfg.ScrapeInterval), timeout: time.Duration(reloadCfg.ScrapeTimeout)}
		l.startFunc = func(interval, timeout time.Duration, errc chan<- error) {
			require.Equal(t, 3*time.Second, interval, "Unexpected scrape interval")
			require.Equal(t, 2*time.Second, timeout, "Unexpected scrape timeout")

			mtx.Lock()
			targetScraper := opts.scraper.(*targetScraper)
			require.True(t, stopped[targetScraper.hash()], "Scrape loop for %v not stopped yet", targetScraper)
			mtx.Unlock()
		}
		return l
	}
	sp := &scrapePool{
		appendable:    &nopAppendable{},
		activeTargets: map[uint64]*Target{},
		loops:         map[uint64]loop{},
		newLoop:       newLoop,
		logger:        nil,
		client:        http.DefaultClient,
	}

	// Reloading a scrape pool with a new scrape configuration must stop all scrape
	// loops and start new ones. A new loop must not be started before the preceding
	// one terminated.

	for i := 0; i < numTargets; i++ {
		labels := labels.FromStrings(model.AddressLabel, fmt.Sprintf("example.com:%d", i))
		t := &Target{
			labels:           labels,
			discoveredLabels: labels,
		}
		l := &testLoop{}
		d := time.Duration((i+1)*20) * time.Millisecond
		l.stopFunc = func() {
			time.Sleep(d)

			mtx.Lock()
			stopped[t.hash()] = true
			mtx.Unlock()
		}

		sp.activeTargets[t.hash()] = t
		sp.loops[t.hash()] = l
	}
	done := make(chan struct{})

	beforeTargets := map[uint64]*Target{}
	for h, t := range sp.activeTargets {
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
	require.Equal(t, numTargets, len(stopped), "Unexpected number of stopped loops")
	mtx.Unlock()

	require.Equal(t, sp.activeTargets, beforeTargets, "Reloading affected target states unexpectedly")
	require.Equal(t, numTargets, len(sp.loops), "Unexpected number of stopped loops after reload")
}

func TestScrapePoolReloadPreserveRelabeledIntervalTimeout(t *testing.T) {
	reloadCfg := &config.ScrapeConfig{
		ScrapeInterval: model.Duration(3 * time.Second),
		ScrapeTimeout:  model.Duration(2 * time.Second),
	}
	newLoop := func(opts scrapeLoopOptions) loop {
		l := &testLoop{interval: opts.interval, timeout: opts.timeout}
		l.startFunc = func(interval, timeout time.Duration, errc chan<- error) {
			require.Equal(t, 5*time.Second, interval, "Unexpected scrape interval")
			require.Equal(t, 3*time.Second, timeout, "Unexpected scrape timeout")
		}
		return l
	}
	sp := &scrapePool{
		appendable: &nopAppendable{},
		activeTargets: map[uint64]*Target{
			1: {
				labels: labels.FromStrings(model.ScrapeIntervalLabel, "5s", model.ScrapeTimeoutLabel, "3s"),
			},
		},
		loops: map[uint64]loop{
			1: noopLoop(),
		},
		newLoop: newLoop,
		logger:  nil,
		client:  http.DefaultClient,
	}

	err := sp.reload(reloadCfg)
	if err != nil {
		t.Fatalf("unable to reload configuration: %s", err)
	}
}

func TestScrapePoolTargetLimit(t *testing.T) {
	var wg sync.WaitGroup
	// On starting to run, new loops created on reload check whether their preceding
	// equivalents have been stopped.
	newLoop := func(opts scrapeLoopOptions) loop {
		wg.Add(1)
		l := &testLoop{
			startFunc: func(interval, timeout time.Duration, errc chan<- error) {
				wg.Done()
			},
			stopFunc: func() {},
		}
		return l
	}
	sp := &scrapePool{
		appendable:    &nopAppendable{},
		activeTargets: map[uint64]*Target{},
		loops:         map[uint64]loop{},
		newLoop:       newLoop,
		logger:        log.NewNopLogger(),
		client:        http.DefaultClient,
	}

	tgs := []*targetgroup.Group{}
	for i := 0; i < 50; i++ {
		tgs = append(tgs,
			&targetgroup.Group{
				Targets: []model.LabelSet{
					{model.AddressLabel: model.LabelValue(fmt.Sprintf("127.0.0.1:%d", 9090+i))},
				},
			},
		)
	}

	var limit uint
	reloadWithLimit := func(l uint) {
		limit = l
		require.NoError(t, sp.reload(&config.ScrapeConfig{
			ScrapeInterval: model.Duration(3 * time.Second),
			ScrapeTimeout:  model.Duration(2 * time.Second),
			TargetLimit:    l,
		}))
	}

	var targets int
	loadTargets := func(n int) {
		targets = n
		sp.Sync(tgs[:n])
	}

	validateIsRunning := func() {
		wg.Wait()
		for _, l := range sp.loops {
			require.True(t, l.(*testLoop).runOnce, "loop should be running")
		}
	}

	validateErrorMessage := func(shouldErr bool) {
		for _, l := range sp.loops {
			lerr := l.(*testLoop).getForcedError()
			if shouldErr {
				require.NotNil(t, lerr, "error was expected for %d targets with a limit of %d", targets, limit)
				require.Equal(t, fmt.Sprintf("target_limit exceeded (number of targets: %d, limit: %d)", targets, limit), lerr.Error())
			} else {
				require.Equal(t, nil, lerr)
			}
		}
	}

	reloadWithLimit(0)
	loadTargets(50)
	validateIsRunning()

	// Simulate an initial config with a limit.
	sp.config.TargetLimit = 30
	limit = 30
	loadTargets(50)
	validateIsRunning()
	validateErrorMessage(true)

	reloadWithLimit(50)
	validateIsRunning()
	validateErrorMessage(false)

	reloadWithLimit(40)
	validateIsRunning()
	validateErrorMessage(true)

	loadTargets(30)
	validateIsRunning()
	validateErrorMessage(false)

	loadTargets(40)
	validateIsRunning()
	validateErrorMessage(false)

	loadTargets(41)
	validateIsRunning()
	validateErrorMessage(true)

	reloadWithLimit(0)
	validateIsRunning()
	validateErrorMessage(false)

	reloadWithLimit(51)
	validateIsRunning()
	validateErrorMessage(false)

	tgs = append(tgs,
		&targetgroup.Group{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue("127.0.0.1:1090")},
			},
		},
		&targetgroup.Group{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue("127.0.0.1:1090")},
			},
		},
	)

	sp.Sync(tgs)
	validateIsRunning()
	validateErrorMessage(false)
}

func TestScrapePoolAppender(t *testing.T) {
	cfg := &config.ScrapeConfig{}
	app := &nopAppendable{}
	sp, _ := newScrapePool(cfg, app, 0, nil, &Options{})

	loop := sp.newLoop(scrapeLoopOptions{
		target: &Target{},
	})
	appl, ok := loop.(*scrapeLoop)
	require.True(t, ok, "Expected scrapeLoop but got %T", loop)

	wrapped := appender(appl.appender(context.Background()), 0, 0)

	tl, ok := wrapped.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", wrapped)

	_, ok = tl.Appender.(nopAppender)
	require.True(t, ok, "Expected base appender but got %T", tl.Appender)

	sampleLimit := 100
	loop = sp.newLoop(scrapeLoopOptions{
		target:      &Target{},
		sampleLimit: sampleLimit,
	})
	appl, ok = loop.(*scrapeLoop)
	require.True(t, ok, "Expected scrapeLoop but got %T", loop)

	wrapped = appender(appl.appender(context.Background()), sampleLimit, 0)

	sl, ok := wrapped.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", wrapped)

	tl, ok = sl.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", sl.Appender)

	_, ok = tl.Appender.(nopAppender)
	require.True(t, ok, "Expected base appender but got %T", tl.Appender)

	wrapped = appender(appl.appender(context.Background()), sampleLimit, 100)

	bl, ok := wrapped.(*bucketLimitAppender)
	require.True(t, ok, "Expected bucketLimitAppender but got %T", wrapped)

	sl, ok = bl.Appender.(*limitAppender)
	require.True(t, ok, "Expected limitAppender but got %T", bl)

	tl, ok = sl.Appender.(*timeLimitAppender)
	require.True(t, ok, "Expected timeLimitAppender but got %T", sl.Appender)

	_, ok = tl.Appender.(nopAppender)
	require.True(t, ok, "Expected base appender but got %T", tl.Appender)
}

func TestScrapePoolRaces(t *testing.T) {
	interval, _ := model.ParseDuration("1s")
	timeout, _ := model.ParseDuration("500ms")
	newConfig := func() *config.ScrapeConfig {
		return &config.ScrapeConfig{ScrapeInterval: interval, ScrapeTimeout: timeout}
	}
	sp, _ := newScrapePool(newConfig(), &nopAppendable{}, 0, nil, &Options{})
	tgts := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: "127.0.0.1:9090"},
				{model.AddressLabel: "127.0.0.2:9090"},
				{model.AddressLabel: "127.0.0.3:9090"},
				{model.AddressLabel: "127.0.0.4:9090"},
				{model.AddressLabel: "127.0.0.5:9090"},
				{model.AddressLabel: "127.0.0.6:9090"},
				{model.AddressLabel: "127.0.0.7:9090"},
				{model.AddressLabel: "127.0.0.8:9090"},
			},
		},
	}

	sp.Sync(tgts)
	active := sp.ActiveTargets()
	dropped := sp.DroppedTargets()
	expectedActive, expectedDropped := len(tgts[0].Targets), 0

	require.Equal(t, expectedActive, len(active), "Invalid number of active targets")
	require.Equal(t, expectedDropped, len(dropped), "Invalid number of dropped targets")

	for i := 0; i < 20; i++ {
		time.Sleep(10 * time.Millisecond)
		sp.reload(newConfig())
	}
	sp.stop()
}

func TestScrapePoolScrapeLoopsStarted(t *testing.T) {
	var wg sync.WaitGroup
	newLoop := func(opts scrapeLoopOptions) loop {
		wg.Add(1)
		l := &testLoop{
			startFunc: func(interval, timeout time.Duration, errc chan<- error) {
				wg.Done()
			},
			stopFunc: func() {},
		}
		return l
	}
	sp := &scrapePool{
		appendable:    &nopAppendable{},
		activeTargets: map[uint64]*Target{},
		loops:         map[uint64]loop{},
		newLoop:       newLoop,
		logger:        nil,
		client:        http.DefaultClient,
	}

	tgs := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue("127.0.0.1:9090")},
			},
		},
		{
			Targets: []model.LabelSet{
				{model.AddressLabel: model.LabelValue("127.0.0.1:9090")},
			},
		},
	}

	require.NoError(t, sp.reload(&config.ScrapeConfig{
		ScrapeInterval: model.Duration(3 * time.Second),
		ScrapeTimeout:  model.Duration(2 * time.Second),
	}))
	sp.Sync(tgs)

	require.Equal(t, 1, len(sp.loops))

	wg.Wait()
	for _, l := range sp.loops {
		require.True(t, l.(*testLoop).runOnce, "loop should be running")
	}
}

func TestScrapeLoopStopBeforeRun(t *testing.T) {
	scraper := &testScraper{}

	sl := newScrapeLoop(context.Background(),
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		nil, nil, 0,
		true,
		0, 0,
		nil,
		1,
		0,
		false,
		false,
		false,
		nil,
		false,
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
		sl.run(nil)
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
		signal   = make(chan struct{}, 1)
		appender = &collectResultAppender{}
		scraper  = &testScraper{}
		app      = func(ctx context.Context) storage.Appender { return appender }
	)

	sl := newScrapeLoop(context.Background(),
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

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
		t.Fatalf("Scrape wasn't stopped.")
	}

	// We expected 1 actual sample for each scrape plus 5 for report samples.
	// At least 2 scrapes were made, plus the final stale markers.
	if len(appender.resultFloats) < 6*3 || len(appender.resultFloats)%6 != 0 {
		t.Fatalf("Expected at least 3 scrapes with 6 samples each, got %d samples", len(appender.resultFloats))
	}
	// All samples in a scrape must have the same timestamp.
	var ts int64
	for i, s := range appender.resultFloats {
		switch {
		case i%6 == 0:
			ts = s.t
		case s.t != ts:
			t.Fatalf("Unexpected multiple timestamps within single scrape")
		}
	}
	// All samples from the last scrape must be stale markers.
	for _, s := range appender.resultFloats[len(appender.resultFloats)-5:] {
		if !value.IsStaleNaN(s.f) {
			t.Fatalf("Appended last sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(s.f))
		}
	}
}

func TestScrapeLoopRun(t *testing.T) {
	var (
		signal = make(chan struct{}, 1)
		errc   = make(chan error)

		scraper = &testScraper{}
		app     = func(ctx context.Context) storage.Appender { return &nopAppender{} }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		time.Second,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
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
		t.Fatalf("Cancellation during initial offset failed")
	case err := <-errc:
		t.Fatalf("Unexpected error: %s", err)
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
	sl = newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		time.Second,
		100*time.Millisecond,
		false,
		false,
		false,
		nil,
		false,
	)

	go func() {
		sl.run(errc)
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
		t.Fatalf("Loop did not terminate on context cancellation")
	}
}

func TestScrapeLoopForcedErr(t *testing.T) {
	var (
		signal = make(chan struct{}, 1)
		errc   = make(chan error)

		scraper = &testScraper{}
		app     = func(ctx context.Context) storage.Appender { return &nopAppender{} }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		time.Second,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

	forcedErr := fmt.Errorf("forced err")
	sl.setForcedError(forcedErr)

	scraper.scrapeFunc = func(context.Context, io.Writer) error {
		t.Fatalf("should not be scraped")
		return nil
	}

	go func() {
		sl.run(errc)
		signal <- struct{}{}
	}()

	select {
	case err := <-errc:
		if err != forcedErr {
			t.Fatalf("Expected forced error but got: %s", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Expected forced error but got none")
	}
	cancel()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape not stopped")
	}
}

func TestScrapeLoopMetadata(t *testing.T) {
	var (
		signal  = make(chan struct{})
		scraper = &testScraper{}
		cache   = newScrapeCache()
	)
	defer close(signal)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { return nopAppender{} },
		cache,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)
	defer cancel()

	slApp := sl.appender(ctx)
	total, _, _, err := sl.append(slApp, []byte(`# TYPE test_metric counter
# HELP test_metric some help text
# UNIT test_metric metric
test_metric 1
# TYPE test_metric_no_help gauge
# HELP test_metric_no_type other help text
# EOF`), "application/openmetrics-text", time.Now())
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	require.Equal(t, 1, total)

	md, ok := cache.GetMetadata("test_metric")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, textparse.MetricTypeCounter, md.Type, "unexpected metric type")
	require.Equal(t, "some help text", md.Help)
	require.Equal(t, "metric", md.Unit)

	md, ok = cache.GetMetadata("test_metric_no_help")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, textparse.MetricTypeGauge, md.Type, "unexpected metric type")
	require.Equal(t, "", md.Help)
	require.Equal(t, "", md.Unit)

	md, ok = cache.GetMetadata("test_metric_no_type")
	require.True(t, ok, "expected metadata to be present")
	require.Equal(t, textparse.MetricTypeUnknown, md.Type, "unexpected metric type")
	require.Equal(t, "other help text", md.Help)
	require.Equal(t, "", md.Unit)
}

func simpleTestScrapeLoop(t testing.TB) (context.Context, *scrapeLoop) {
	// Need a full storage for correct Add/AddFast semantics.
	s := teststorage.New(t)
	t.Cleanup(func() { s.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		&testScraper{},
		nil, nil,
		nopMutator,
		nopMutator,
		s.Appender,
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)
	t.Cleanup(func() { cancel() })

	return ctx, sl
}

func TestScrapeLoopSeriesAdded(t *testing.T) {
	ctx, sl := simpleTestScrapeLoop(t)

	slApp := sl.appender(ctx)
	total, added, seriesAdded, err := sl.append(slApp, []byte("test_metric 1\n"), "", time.Time{})
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 1, seriesAdded)

	slApp = sl.appender(ctx)
	total, added, seriesAdded, err = sl.append(slApp, []byte("test_metric 1\n"), "", time.Time{})
	require.NoError(t, slApp.Commit())
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 0, seriesAdded)
}

func TestScrapeLoopFailWithInvalidLabelsAfterRelabel(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	target := &Target{
		labels: labels.FromStrings("pod_label_invalid_012", "test"),
	}
	relabelConfig := []*relabel.Config{{
		Action:      relabel.LabelMap,
		Regex:       relabel.MustNewRegexp("pod_label_invalid_(.+)"),
		Separator:   ";",
		Replacement: "$1",
	}}
	sl := newScrapeLoop(ctx,
		&testScraper{},
		nil, nil,
		func(l labels.Labels) labels.Labels {
			return mutateSampleLabels(l, target, true, relabelConfig)
		},
		nopMutator,
		s.Appender,
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	slApp := sl.appender(ctx)
	total, added, seriesAdded, err := sl.append(slApp, []byte("test_metric 1\n"), "", time.Time{})
	require.ErrorContains(t, err, "invalid metric name or label names")
	require.NoError(t, slApp.Rollback())
	require.Equal(t, 1, total)
	require.Equal(t, 0, added)
	require.Equal(t, 0, seriesAdded)
}

func makeTestMetrics(n int) []byte {
	// Construct a metrics string to parse
	sb := bytes.Buffer{}
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "# TYPE metric_a gauge\n")
		fmt.Fprintf(&sb, "# HELP metric_a help text\n")
		fmt.Fprintf(&sb, "metric_a{foo=\"%d\",bar=\"%d\"} 1\n", i, i*100)
	}
	return sb.Bytes()
}

func BenchmarkScrapeLoopAppend(b *testing.B) {
	ctx, sl := simpleTestScrapeLoop(b)

	slApp := sl.appender(ctx)
	metrics := makeTestMetrics(100)
	ts := time.Time{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts = ts.Add(time.Second)
		_, _, _, _ = sl.append(slApp, metrics, "", ts)
	}
}

func BenchmarkScrapeLoopAppendOM(b *testing.B) {
	ctx, sl := simpleTestScrapeLoop(b)

	slApp := sl.appender(ctx)
	metrics := makeTestMetrics(100)
	ts := time.Time{}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ts = ts.Add(time.Second)
		_, _, _, _ = sl.append(slApp, metrics, "application/openmetrics-text", ts)
	}
}

func TestScrapeLoopRunCreatesStaleMarkersOnFailedScrape(t *testing.T) {
	appender := &collectResultAppender{}
	var (
		signal  = make(chan struct{}, 1)
		scraper = &testScraper{}
		app     = func(ctx context.Context) storage.Appender { return appender }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)
	// Succeed once, several failures, then stop.
	numScrapes := 0

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++

		switch numScrapes {
		case 1:
			w.Write([]byte("metric_a 42\n"))
			return nil
		case 5:
			cancel()
		}
		return errors.New("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	// 1 successfully scraped sample, 1 stale marker after first fail, 5 report samples for
	// each scrape successful or not.
	require.Equal(t, 27, len(appender.resultFloats), "Appended samples not as expected:\n%s", appender)
	require.Equal(t, 42.0, appender.resultFloats[0].f, "Appended first sample not as expected")
	require.True(t, value.IsStaleNaN(appender.resultFloats[6].f),
		"Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(appender.resultFloats[6].f))
}

func TestScrapeLoopRunCreatesStaleMarkersOnParseFailure(t *testing.T) {
	appender := &collectResultAppender{}
	var (
		signal     = make(chan struct{}, 1)
		scraper    = &testScraper{}
		app        = func(ctx context.Context) storage.Appender { return appender }
		numScrapes = 0
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

	// Succeed once, several failures, then stop.
	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++
		switch numScrapes {
		case 1:
			w.Write([]byte("metric_a 42\n"))
			return nil
		case 2:
			w.Write([]byte("7&-\n"))
			return nil
		case 3:
			cancel()
		}
		return errors.New("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	// 1 successfully scraped sample, 1 stale marker after first fail, 5 report samples for
	// each scrape successful or not.
	require.Equal(t, 17, len(appender.resultFloats), "Appended samples not as expected:\n%s", appender)
	require.Equal(t, 42.0, appender.resultFloats[0].f, "Appended first sample not as expected")
	require.True(t, value.IsStaleNaN(appender.resultFloats[6].f),
		"Appended second sample not as expected. Wanted: stale NaN Got: %x", math.Float64bits(appender.resultFloats[6].f))
}

func TestScrapeLoopCache(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	appender := &collectResultAppender{}
	var (
		signal  = make(chan struct{}, 1)
		scraper = &testScraper{}
		app     = func(ctx context.Context) storage.Appender { appender.next = s.Appender(ctx); return appender }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

	numScrapes := 0

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		switch numScrapes {
		case 1, 2:
			if _, ok := sl.cache.series["metric_a"]; !ok {
				t.Errorf("metric_a missing from cache after scrape %d", numScrapes)
			}
			if _, ok := sl.cache.series["metric_b"]; !ok {
				t.Errorf("metric_b missing from cache after scrape %d", numScrapes)
			}
		case 3:
			if _, ok := sl.cache.series["metric_a"]; !ok {
				t.Errorf("metric_a missing from cache after scrape %d", numScrapes)
			}
			if _, ok := sl.cache.series["metric_b"]; ok {
				t.Errorf("metric_b present in cache after scrape %d", numScrapes)
			}
		}

		numScrapes++
		switch numScrapes {
		case 1:
			w.Write([]byte("metric_a 42\nmetric_b 43\n"))
			return nil
		case 3:
			w.Write([]byte("metric_a 44\n"))
			return nil
		case 4:
			cancel()
		}
		return fmt.Errorf("scrape failed")
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	// 1 successfully scraped sample, 1 stale marker after first fail, 5 report samples for
	// each scrape successful or not.
	require.Equal(t, 26, len(appender.resultFloats), "Appended samples not as expected:\n%s", appender)
}

func TestScrapeLoopCacheMemoryExhaustionProtection(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	sapp := s.Appender(context.Background())

	appender := &collectResultAppender{next: sapp}
	var (
		signal  = make(chan struct{}, 1)
		scraper = &testScraper{}
		app     = func(ctx context.Context) storage.Appender { return appender }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

	numScrapes := 0

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++
		if numScrapes < 5 {
			s := ""
			for i := 0; i < 500; i++ {
				s = fmt.Sprintf("%smetric_%d_%d 42\n", s, i, numScrapes)
			}
			w.Write([]byte(fmt.Sprintf(s + "&")))
		} else {
			cancel()
		}
		return nil
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}

	if len(sl.cache.series) > 2000 {
		t.Fatalf("More than 2000 series cached. Got: %d", len(sl.cache.series))
	}
}

func TestScrapeLoopAppend(t *testing.T) {
	tests := []struct {
		title           string
		honorLabels     bool
		scrapeLabels    string
		discoveryLabels []string
		expLset         labels.Labels
		expValue        float64
	}{
		{
			// When "honor_labels" is not set
			// label name collision is handler by adding a prefix.
			title:           "Label name collision",
			honorLabels:     false,
			scrapeLabels:    `metric{n="1"} 0`,
			discoveryLabels: []string{"n", "2"},
			expLset:         labels.FromStrings("__name__", "metric", "exported_n", "1", "n", "2"),
			expValue:        0,
		}, {
			// When "honor_labels" is not set
			// exported label from discovery don't get overwritten
			title:           "Label name collision",
			honorLabels:     false,
			scrapeLabels:    `metric 0`,
			discoveryLabels: []string{"n", "2", "exported_n", "2"},
			expLset:         labels.FromStrings("__name__", "metric", "n", "2", "exported_n", "2"),
			expValue:        0,
		}, {
			// Labels with no value need to be removed as these should not be ingested.
			title:           "Delete Empty labels",
			honorLabels:     false,
			scrapeLabels:    `metric{n=""} 0`,
			discoveryLabels: nil,
			expLset:         labels.FromStrings("__name__", "metric"),
			expValue:        0,
		}, {
			// Honor Labels should ignore labels with the same name.
			title:           "Honor Labels",
			honorLabels:     true,
			scrapeLabels:    `metric{n1="1" n2="2"} 0`,
			discoveryLabels: []string{"n1", "0"},
			expLset:         labels.FromStrings("__name__", "metric", "n1", "1", "n2", "2"),
			expValue:        0,
		}, {
			title:           "Stale - NaN",
			honorLabels:     false,
			scrapeLabels:    `metric NaN`,
			discoveryLabels: nil,
			expLset:         labels.FromStrings("__name__", "metric"),
			expValue:        float64(value.NormalNaN),
		},
	}

	for _, test := range tests {
		app := &collectResultAppender{}

		discoveryLabels := &Target{
			labels: labels.FromStrings(test.discoveryLabels...),
		}

		sl := newScrapeLoop(context.Background(),
			nil, nil, nil,
			func(l labels.Labels) labels.Labels {
				return mutateSampleLabels(l, discoveryLabels, test.honorLabels, nil)
			},
			func(l labels.Labels) labels.Labels {
				return mutateReportSampleLabels(l, discoveryLabels)
			},
			func(ctx context.Context) storage.Appender { return app },
			nil,
			0,
			true,
			0, 0,
			nil,
			0,
			0,
			false,
			false,
			false,
			nil,
			false,
		)

		now := time.Now()

		slApp := sl.appender(context.Background())
		_, _, _, err := sl.append(slApp, []byte(test.scrapeLabels), "", now)
		require.NoError(t, err)
		require.NoError(t, slApp.Commit())

		expected := []floatSample{
			{
				metric: test.expLset,
				t:      timestamp.FromTime(now),
				f:      test.expValue,
			},
		}

		// When the expected value is NaN
		// DeepEqual will report NaNs as being different,
		// so replace it with the expected one.
		if test.expValue == float64(value.NormalNaN) {
			app.resultFloats[0].f = expected[0].f
		}

		t.Logf("Test:%s", test.title)
		require.Equal(t, expected, app.resultFloats)
	}
}

func TestScrapeLoopAppendForConflictingPrefixedLabels(t *testing.T) {
	testcases := map[string]struct {
		targetLabels  []string
		exposedLabels string
		expected      []string
	}{
		"One target label collides with existing label": {
			targetLabels:  []string{"foo", "2"},
			exposedLabels: `metric{foo="1"} 0`,
			expected:      []string{"__name__", "metric", "exported_foo", "1", "foo", "2"},
		},

		"One target label collides with existing label, plus target label already with prefix 'exported'": {
			targetLabels:  []string{"foo", "2", "exported_foo", "3"},
			exposedLabels: `metric{foo="1"} 0`,
			expected:      []string{"__name__", "metric", "exported_exported_foo", "1", "exported_foo", "3", "foo", "2"},
		},
		"One target label collides with existing label, plus existing label already with prefix 'exported": {
			targetLabels:  []string{"foo", "3"},
			exposedLabels: `metric{foo="1" exported_foo="2"} 0`,
			expected:      []string{"__name__", "metric", "exported_exported_foo", "1", "exported_foo", "2", "foo", "3"},
		},
		"One target label collides with existing label, both already with prefix 'exported'": {
			targetLabels:  []string{"exported_foo", "2"},
			exposedLabels: `metric{exported_foo="1"} 0`,
			expected:      []string{"__name__", "metric", "exported_exported_foo", "1", "exported_foo", "2"},
		},
		"Two target labels collide with existing labels, both with and without prefix 'exported'": {
			targetLabels:  []string{"foo", "3", "exported_foo", "4"},
			exposedLabels: `metric{foo="1" exported_foo="2"} 0`,
			expected: []string{
				"__name__", "metric", "exported_exported_foo", "1", "exported_exported_exported_foo",
				"2", "exported_foo", "4", "foo", "3",
			},
		},
		"Extreme example": {
			targetLabels:  []string{"foo", "0", "exported_exported_foo", "1", "exported_exported_exported_foo", "2"},
			exposedLabels: `metric{foo="3" exported_foo="4" exported_exported_exported_foo="5"} 0`,
			expected: []string{
				"__name__", "metric",
				"exported_exported_exported_exported_exported_foo", "5",
				"exported_exported_exported_exported_foo", "3",
				"exported_exported_exported_foo", "2",
				"exported_exported_foo", "1",
				"exported_foo", "4",
				"foo", "0",
			},
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			app := &collectResultAppender{}
			sl := newScrapeLoop(context.Background(), nil, nil, nil,
				func(l labels.Labels) labels.Labels {
					return mutateSampleLabels(l, &Target{labels: labels.FromStrings(tc.targetLabels...)}, false, nil)
				},
				nil,
				func(ctx context.Context) storage.Appender { return app }, nil, 0, true, 0, 0, nil, 0, 0, false, false, false, nil, false,
			)
			slApp := sl.appender(context.Background())
			_, _, _, err := sl.append(slApp, []byte(tc.exposedLabels), "", time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC))
			require.NoError(t, err)

			require.NoError(t, slApp.Commit())

			require.Equal(t, []floatSample{
				{
					metric: labels.FromStrings(tc.expected...),
					t:      timestamp.FromTime(time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)),
					f:      0,
				},
			}, app.resultFloats)
		})
	}
}

func TestScrapeLoopAppendCacheEntryButErrNotFound(t *testing.T) {
	// collectResultAppender's AddFast always returns ErrNotFound if we don't give it a next.
	app := &collectResultAppender{}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	fakeRef := storage.SeriesRef(1)
	expValue := float64(1)
	metric := []byte(`metric{n="1"} 1`)
	p, warning := textparse.New(metric, "", false)
	require.NoError(t, warning)

	var lset labels.Labels
	p.Next()
	p.Metric(&lset)
	hash := lset.Hash()

	// Create a fake entry in the cache
	sl.cache.addRef(metric, fakeRef, lset, hash)
	now := time.Now()

	slApp := sl.appender(context.Background())
	_, _, _, err := sl.append(slApp, metric, "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	expected := []floatSample{
		{
			metric: lset,
			t:      timestamp.FromTime(now),
			f:      expValue,
		},
	}

	require.Equal(t, expected, app.resultFloats)
}

func TestScrapeLoopAppendSampleLimit(t *testing.T) {
	resApp := &collectResultAppender{}
	app := &limitAppender{Appender: resApp, limit: 1}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		func(l labels.Labels) labels.Labels {
			if l.Has("deleteme") {
				return labels.EmptyLabels()
			}
			return l
		},
		nopMutator,
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		app.limit, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	// Get the value of the Counter before performing the append.
	beforeMetric := dto.Metric{}
	err := targetScrapeSampleLimit.Write(&beforeMetric)
	require.NoError(t, err)

	beforeMetricValue := beforeMetric.GetCounter().GetValue()

	now := time.Now()
	slApp := sl.appender(context.Background())
	total, added, seriesAdded, err := sl.append(app, []byte("metric_a 1\nmetric_b 1\nmetric_c 1\n"), "", now)
	if err != errSampleLimit {
		t.Fatalf("Did not see expected sample limit error: %s", err)
	}
	require.NoError(t, slApp.Rollback())
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 1, seriesAdded)

	// Check that the Counter has been incremented a single time for the scrape,
	// not multiple times for each sample.
	metric := dto.Metric{}
	err = targetScrapeSampleLimit.Write(&metric)
	require.NoError(t, err)

	value := metric.GetCounter().GetValue()
	change := value - beforeMetricValue
	require.Equal(t, 1.0, change, "Unexpected change of sample limit metric: %f", change)

	// And verify that we got the samples that fit under the limit.
	want := []floatSample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      timestamp.FromTime(now),
			f:      1,
		},
	}
	require.Equal(t, want, resApp.rolledbackFloats, "Appended samples not as expected:\n%s", appender)

	now = time.Now()
	slApp = sl.appender(context.Background())
	total, added, seriesAdded, err = sl.append(slApp, []byte("metric_a 1\nmetric_b 1\nmetric_c{deleteme=\"yes\"} 1\nmetric_d 1\nmetric_e 1\nmetric_f 1\nmetric_g 1\nmetric_h{deleteme=\"yes\"} 1\nmetric_i{deleteme=\"yes\"} 1\n"), "", now)
	if err != errSampleLimit {
		t.Fatalf("Did not see expected sample limit error: %s", err)
	}
	require.NoError(t, slApp.Rollback())
	require.Equal(t, 9, total)
	require.Equal(t, 6, added)
	require.Equal(t, 0, seriesAdded)
}

func TestScrapeLoop_HistogramBucketLimit(t *testing.T) {
	resApp := &collectResultAppender{}
	app := &bucketLimitAppender{Appender: resApp, limit: 2}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		func(l labels.Labels) labels.Labels {
			if l.Has("deleteme") {
				return labels.EmptyLabels()
			}
			return l
		},
		nopMutator,
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		app.limit, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	metric := dto.Metric{}
	err := targetScrapeNativeHistogramBucketLimit.Write(&metric)
	require.NoError(t, err)
	beforeMetricValue := metric.GetCounter().GetValue()

	nativeHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:                      "testing",
			Name:                           "example_native_histogram",
			Help:                           "This is used for testing",
			ConstLabels:                    map[string]string{"some": "value"},
			NativeHistogramBucketFactor:    1.1, // 10% increase from bucket to bucket
			NativeHistogramMaxBucketNumber: 100, // intentionally higher than the limit we'll use in the scraper
		},
		[]string{"size"},
	)
	registry := prometheus.NewRegistry()
	registry.Register(nativeHistogram)
	nativeHistogram.WithLabelValues("S").Observe(1.0)
	nativeHistogram.WithLabelValues("M").Observe(1.0)
	nativeHistogram.WithLabelValues("L").Observe(1.0)
	nativeHistogram.WithLabelValues("M").Observe(10.0)
	nativeHistogram.WithLabelValues("L").Observe(10.0) // in different bucket since > 1*1.1

	gathered, err := registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, gathered)

	histogramMetricFamily := gathered[0]
	msg, err := MetricFamilyToProtobuf(histogramMetricFamily)
	require.NoError(t, err)

	now := time.Now()
	total, added, seriesAdded, err := sl.append(app, msg, "application/vnd.google.protobuf", now)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 3, seriesAdded)

	err = targetScrapeNativeHistogramBucketLimit.Write(&metric)
	require.NoError(t, err)
	metricValue := metric.GetCounter().GetValue()
	require.Equal(t, beforeMetricValue, metricValue)
	beforeMetricValue = metricValue

	nativeHistogram.WithLabelValues("L").Observe(100.0) // in different bucket since > 10*1.1

	gathered, err = registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, gathered)

	histogramMetricFamily = gathered[0]
	msg, err = MetricFamilyToProtobuf(histogramMetricFamily)
	require.NoError(t, err)

	now = time.Now()
	total, added, seriesAdded, err = sl.append(app, msg, "application/vnd.google.protobuf", now)
	if err != errBucketLimit {
		t.Fatalf("Did not see expected histogram bucket limit error: %s", err)
	}
	require.NoError(t, app.Rollback())
	require.Equal(t, 3, total)
	require.Equal(t, 3, added)
	require.Equal(t, 0, seriesAdded)

	err = targetScrapeNativeHistogramBucketLimit.Write(&metric)
	require.NoError(t, err)
	metricValue = metric.GetCounter().GetValue()
	require.Equal(t, beforeMetricValue+1, metricValue)
}

func TestScrapeLoop_ChangingMetricString(t *testing.T) {
	// This is a regression test for the scrape loop cache not properly maintaining
	// IDs when the string representation of a metric changes across a scrape. Thus
	// we use a real storage appender here.
	s := teststorage.New(t)
	defer s.Close()

	capp := &collectResultAppender{}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { capp.next = s.Appender(ctx); return capp },
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Now()
	slApp := sl.appender(context.Background())
	_, _, _, err := sl.append(slApp, []byte(`metric_a{a="1",b="1"} 1`), "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	slApp = sl.appender(context.Background())
	_, _, _, err = sl.append(slApp, []byte(`metric_a{b="1",a="1"} 2`), "", now.Add(time.Minute))
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	// DeepEqual will report NaNs as being different, so replace with a different value.
	want := []floatSample{
		{
			metric: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			t:      timestamp.FromTime(now),
			f:      1,
		},
		{
			metric: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			t:      timestamp.FromTime(now.Add(time.Minute)),
			f:      2,
		},
	}
	require.Equal(t, want, capp.resultFloats, "Appended samples not as expected:\n%s", appender)
}

func TestScrapeLoopAppendStaleness(t *testing.T) {
	app := &collectResultAppender{}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Now()
	slApp := sl.appender(context.Background())
	_, _, _, err := sl.append(slApp, []byte("metric_a 1\n"), "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	slApp = sl.appender(context.Background())
	_, _, _, err = sl.append(slApp, []byte(""), "", now.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	ingestedNaN := math.Float64bits(app.resultFloats[1].f)
	require.Equal(t, value.StaleNaN, ingestedNaN, "Appended stale sample wasn't as expected")

	// DeepEqual will report NaNs as being different, so replace with a different value.
	app.resultFloats[1].f = 42
	want := []floatSample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      timestamp.FromTime(now),
			f:      1,
		},
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      timestamp.FromTime(now.Add(time.Second)),
			f:      42,
		},
	}
	require.Equal(t, want, app.resultFloats, "Appended samples not as expected:\n%s", appender)
}

func TestScrapeLoopAppendNoStalenessIfTimestamp(t *testing.T) {
	app := &collectResultAppender{}
	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Now()
	slApp := sl.appender(context.Background())
	_, _, _, err := sl.append(slApp, []byte("metric_a 1 1000\n"), "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	slApp = sl.appender(context.Background())
	_, _, _, err = sl.append(slApp, []byte(""), "", now.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	want := []floatSample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "metric_a"),
			t:      1000,
			f:      1,
		},
	}
	require.Equal(t, want, app.resultFloats, "Appended samples not as expected:\n%s", appender)
}

func TestScrapeLoopAppendExemplar(t *testing.T) {
	tests := []struct {
		title           string
		scrapeText      string
		contentType     string
		discoveryLabels []string
		floats          []floatSample
		histograms      []histogramSample
		exemplars       []exemplar.Exemplar
	}{
		{
			title:           "Metric without exemplars",
			scrapeText:      "metric_total{n=\"1\"} 0\n# EOF",
			contentType:     "application/openmetrics-text",
			discoveryLabels: []string{"n", "2"},
			floats: []floatSample{{
				metric: labels.FromStrings("__name__", "metric_total", "exported_n", "1", "n", "2"),
				f:      0,
			}},
		},
		{
			title:           "Metric with exemplars",
			scrapeText:      "metric_total{n=\"1\"} 0 # {a=\"abc\"} 1.0\n# EOF",
			contentType:     "application/openmetrics-text",
			discoveryLabels: []string{"n", "2"},
			floats: []floatSample{{
				metric: labels.FromStrings("__name__", "metric_total", "exported_n", "1", "n", "2"),
				f:      0,
			}},
			exemplars: []exemplar.Exemplar{
				{Labels: labels.FromStrings("a", "abc"), Value: 1},
			},
		},
		{
			title:           "Metric with exemplars and TS",
			scrapeText:      "metric_total{n=\"1\"} 0 # {a=\"abc\"} 1.0 10000\n# EOF",
			contentType:     "application/openmetrics-text",
			discoveryLabels: []string{"n", "2"},
			floats: []floatSample{{
				metric: labels.FromStrings("__name__", "metric_total", "exported_n", "1", "n", "2"),
				f:      0,
			}},
			exemplars: []exemplar.Exemplar{
				{Labels: labels.FromStrings("a", "abc"), Value: 1, Ts: 10000000, HasTs: true},
			},
		},
		{
			title: "Two metrics and exemplars",
			scrapeText: `metric_total{n="1"} 1 # {t="1"} 1.0 10000
metric_total{n="2"} 2 # {t="2"} 2.0 20000
# EOF`,
			contentType: "application/openmetrics-text",
			floats: []floatSample{{
				metric: labels.FromStrings("__name__", "metric_total", "n", "1"),
				f:      1,
			}, {
				metric: labels.FromStrings("__name__", "metric_total", "n", "2"),
				f:      2,
			}},
			exemplars: []exemplar.Exemplar{
				{Labels: labels.FromStrings("t", "1"), Value: 1, Ts: 10000000, HasTs: true},
				{Labels: labels.FromStrings("t", "2"), Value: 2, Ts: 20000000, HasTs: true},
			},
		},
		{
			title: "Native histogram with two exemplars",
			scrapeText: `name: "test_histogram"
help: "Test histogram with many buckets removed to keep it manageable in size."
type: HISTOGRAM
metric: <
  histogram: <
    sample_count: 175
    sample_sum: 0.0008280461746287094
    bucket: <
      cumulative_count: 2
      upper_bound: -0.0004899999999999998
    >
    bucket: <
      cumulative_count: 4
      upper_bound: -0.0003899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "59727"
        >
        value: -0.00039
        timestamp: <
          seconds: 1625851155
          nanos: 146848499
        >
      >
    >
    bucket: <
      cumulative_count: 16
      upper_bound: -0.0002899999999999998
      exemplar: <
        label: <
          name: "dummyID"
          value: "5617"
        >
        value: -0.00029
      >
    >
    schema: 3
    zero_threshold: 2.938735877055719e-39
    zero_count: 2
    negative_span: <
      offset: -162
      length: 1
    >
    negative_span: <
      offset: 23
      length: 4
    >
    negative_delta: 1
    negative_delta: 3
    negative_delta: -2
    negative_delta: -1
    negative_delta: 1
    positive_span: <
      offset: -161
      length: 1
    >
    positive_span: <
      offset: 8
      length: 3
    >
    positive_delta: 1
    positive_delta: 2
    positive_delta: -1
    positive_delta: -1
  >
  timestamp_ms: 1234568
>

`,
			contentType: "application/vnd.google.protobuf",
			histograms: []histogramSample{{
				t: 1234568,
				h: &histogram.Histogram{
					Count:         175,
					ZeroCount:     2,
					Sum:           0.0008280461746287094,
					ZeroThreshold: 2.938735877055719e-39,
					Schema:        3,
					PositiveSpans: []histogram.Span{
						{Offset: -161, Length: 1},
						{Offset: 8, Length: 3},
					},
					NegativeSpans: []histogram.Span{
						{Offset: -162, Length: 1},
						{Offset: 23, Length: 4},
					},
					PositiveBuckets: []int64{1, 2, -1, -1},
					NegativeBuckets: []int64{1, 3, -2, -1, 1},
				},
			}},
			exemplars: []exemplar.Exemplar{
				{Labels: labels.FromStrings("dummyID", "59727"), Value: -0.00039, Ts: 1625851155146, HasTs: true},
				{Labels: labels.FromStrings("dummyID", "5617"), Value: -0.00029, Ts: 1234568, HasTs: false},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			app := &collectResultAppender{}

			discoveryLabels := &Target{
				labels: labels.FromStrings(test.discoveryLabels...),
			}

			sl := newScrapeLoop(context.Background(),
				nil, nil, nil,
				func(l labels.Labels) labels.Labels {
					return mutateSampleLabels(l, discoveryLabels, false, nil)
				},
				func(l labels.Labels) labels.Labels {
					return mutateReportSampleLabels(l, discoveryLabels)
				},
				func(ctx context.Context) storage.Appender { return app },
				nil,
				0,
				true,
				0, 0,
				nil,
				0,
				0,
				false,
				false,
				false,
				nil,
				false,
			)

			now := time.Now()

			for i := range test.floats {
				test.floats[i].t = timestamp.FromTime(now)
			}

			// We need to set the timestamp for expected exemplars that does not have a timestamp.
			for i := range test.exemplars {
				if test.exemplars[i].Ts == 0 {
					test.exemplars[i].Ts = timestamp.FromTime(now)
				}
			}

			buf := &bytes.Buffer{}
			if test.contentType == "application/vnd.google.protobuf" {
				// In case of protobuf, we have to create the binary representation.
				pb := &dto.MetricFamily{}
				// From text to proto message.
				require.NoError(t, proto.UnmarshalText(test.scrapeText, pb))
				// From proto message to binary protobuf.
				protoBuf, err := proto.Marshal(pb)
				require.NoError(t, err)

				// Write first length, then binary protobuf.
				varintBuf := binary.AppendUvarint(nil, uint64(len(protoBuf)))
				buf.Write(varintBuf)
				buf.Write(protoBuf)
			} else {
				buf.WriteString(test.scrapeText)
			}

			_, _, _, err := sl.append(app, buf.Bytes(), test.contentType, now)
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			require.Equal(t, test.floats, app.resultFloats)
			require.Equal(t, test.histograms, app.resultHistograms)
			require.Equal(t, test.exemplars, app.resultExemplars)
		})
	}
}

func TestScrapeLoopAppendExemplarSeries(t *testing.T) {
	scrapeText := []string{`metric_total{n="1"} 1 # {t="1"} 1.0 10000
# EOF`, `metric_total{n="1"} 2 # {t="2"} 2.0 20000
# EOF`}
	samples := []floatSample{{
		metric: labels.FromStrings("__name__", "metric_total", "n", "1"),
		f:      1,
	}, {
		metric: labels.FromStrings("__name__", "metric_total", "n", "1"),
		f:      2,
	}}
	exemplars := []exemplar.Exemplar{
		{Labels: labels.FromStrings("t", "1"), Value: 1, Ts: 10000000, HasTs: true},
		{Labels: labels.FromStrings("t", "2"), Value: 2, Ts: 20000000, HasTs: true},
	}
	discoveryLabels := &Target{
		labels: labels.FromStrings(),
	}

	app := &collectResultAppender{}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		func(l labels.Labels) labels.Labels {
			return mutateSampleLabels(l, discoveryLabels, false, nil)
		},
		func(l labels.Labels) labels.Labels {
			return mutateReportSampleLabels(l, discoveryLabels)
		},
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Now()

	for i := range samples {
		ts := now.Add(time.Second * time.Duration(i))
		samples[i].t = timestamp.FromTime(ts)
	}

	// We need to set the timestamp for expected exemplars that does not have a timestamp.
	for i := range exemplars {
		if exemplars[i].Ts == 0 {
			ts := now.Add(time.Second * time.Duration(i))
			exemplars[i].Ts = timestamp.FromTime(ts)
		}
	}

	for i, st := range scrapeText {
		_, _, _, err := sl.append(app, []byte(st), "application/openmetrics-text", timestamp.Time(samples[i].t))
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	require.Equal(t, samples, app.resultFloats)
	require.Equal(t, exemplars, app.resultExemplars)
}

func TestScrapeLoopRunReportsTargetDownOnScrapeError(t *testing.T) {
	var (
		scraper  = &testScraper{}
		appender = &collectResultAppender{}
		app      = func(ctx context.Context) storage.Appender { return appender }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		cancel()
		return errors.New("scrape failed")
	}

	sl.run(nil)
	require.Equal(t, 0.0, appender.resultFloats[0].f, "bad 'up' value")
}

func TestScrapeLoopRunReportsTargetDownOnInvalidUTF8(t *testing.T) {
	var (
		scraper  = &testScraper{}
		appender = &collectResultAppender{}
		app      = func(ctx context.Context) storage.Appender { return appender }
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		app,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		cancel()
		w.Write([]byte("a{l=\"\xff\"} 1\n"))
		return nil
	}

	sl.run(nil)
	require.Equal(t, 0.0, appender.resultFloats[0].f, "bad 'up' value")
}

type errorAppender struct {
	collectResultAppender
}

func (app *errorAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	switch lset.Get(model.MetricNameLabel) {
	case "out_of_order":
		return 0, storage.ErrOutOfOrderSample
	case "amend":
		return 0, storage.ErrDuplicateSampleForTimestamp
	case "out_of_bounds":
		return 0, storage.ErrOutOfBounds
	default:
		return app.collectResultAppender.Append(ref, lset, t, v)
	}
}

func TestScrapeLoopAppendGracefullyIfAmendOrOutOfOrderOrOutOfBounds(t *testing.T) {
	app := &errorAppender{}

	sl := newScrapeLoop(context.Background(),
		nil,
		nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Unix(1, 0)
	slApp := sl.appender(context.Background())
	total, added, seriesAdded, err := sl.append(slApp, []byte("out_of_order 1\namend 1\nnormal 1\nout_of_bounds 1\n"), "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	want := []floatSample{
		{
			metric: labels.FromStrings(model.MetricNameLabel, "normal"),
			t:      timestamp.FromTime(now),
			f:      1,
		},
	}
	require.Equal(t, want, app.resultFloats, "Appended samples not as expected:\n%s", appender)
	require.Equal(t, 4, total)
	require.Equal(t, 4, added)
	require.Equal(t, 1, seriesAdded)
}

func TestScrapeLoopOutOfBoundsTimeError(t *testing.T) {
	app := &collectResultAppender{}
	sl := newScrapeLoop(context.Background(),
		nil,
		nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender {
			return &timeLimitAppender{
				Appender: app,
				maxTime:  timestamp.FromTime(time.Now().Add(10 * time.Minute)),
			}
		},
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Now().Add(20 * time.Minute)
	slApp := sl.appender(context.Background())
	total, added, seriesAdded, err := sl.append(slApp, []byte("normal 1\n"), "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
	require.Equal(t, 1, total)
	require.Equal(t, 1, added)
	require.Equal(t, 0, seriesAdded)
}

func TestTargetScraperScrapeOK(t *testing.T) {
	const (
		configTimeout   = 1500 * time.Millisecond
		expectedTimeout = "1.5"
	)

	var protobufParsing bool

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if protobufParsing {
				accept := r.Header.Get("Accept")
				if !strings.HasPrefix(accept, "application/vnd.google.protobuf;") {
					t.Errorf("Expected Accept header to prefer application/vnd.google.protobuf, got %q", accept)
				}
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

	runTest := func(acceptHeader string) {
		ts := &targetScraper{
			Target: &Target{
				labels: labels.FromStrings(
					model.SchemeLabel, serverURL.Scheme,
					model.AddressLabel, serverURL.Host,
				),
			},
			client:       http.DefaultClient,
			timeout:      configTimeout,
			acceptHeader: acceptHeader,
		}
		var buf bytes.Buffer

		contentType, err := ts.scrape(context.Background(), &buf)
		require.NoError(t, err)
		require.Equal(t, "text/plain; version=0.0.4", contentType)
		require.Equal(t, "metric_a 1\nmetric_b 2\n", buf.String())
	}

	runTest(scrapeAcceptHeader)
	protobufParsing = true
	runTest(scrapeAcceptHeaderWithProtobuf)
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
		client:       http.DefaultClient,
		acceptHeader: scrapeAcceptHeader,
	}
	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error, 1)

	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	go func() {
		_, err := ts.scrape(ctx, io.Discard)
		switch {
		case err == nil:
			errc <- errors.New("Expected error but got nil")
		case ctx.Err() != context.Canceled:
			errc <- errors.Errorf("Expected context cancellation error but got: %s", ctx.Err())
		default:
			close(errc)
		}
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape function did not return unexpectedly")
	case err := <-errc:
		require.NoError(t, err)
	}
	// If this is closed in a defer above the function the test server
	// doesn't terminate and the test doesn't complete.
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
		client:       http.DefaultClient,
		acceptHeader: scrapeAcceptHeader,
	}

	_, err = ts.scrape(context.Background(), io.Discard)
	require.Contains(t, err.Error(), "404", "Expected \"404 NotFound\" error but got: %s", err)
}

func TestTargetScraperBodySizeLimit(t *testing.T) {
	const (
		bodySizeLimit = 15
		responseBody  = "metric_a 1\nmetric_b 2\n"
	)
	var gzipResponse bool
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
			if gzipResponse {
				w.Header().Set("Content-Encoding", "gzip")
				gw := gzip.NewWriter(w)
				defer gw.Close()
				gw.Write([]byte(responseBody))
				return
			}
			w.Write([]byte(responseBody))
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
		client:        http.DefaultClient,
		bodySizeLimit: bodySizeLimit,
		acceptHeader:  scrapeAcceptHeader,
	}
	var buf bytes.Buffer

	// Target response uncompressed body, scrape with body size limit.
	_, err = ts.scrape(context.Background(), &buf)
	require.ErrorIs(t, err, errBodySizeLimit)
	require.Equal(t, bodySizeLimit, buf.Len())
	// Target response gzip compressed body, scrape with body size limit.
	gzipResponse = true
	buf.Reset()
	_, err = ts.scrape(context.Background(), &buf)
	require.ErrorIs(t, err, errBodySizeLimit)
	require.Equal(t, bodySizeLimit, buf.Len())
	// Target response uncompressed body, scrape without body size limit.
	gzipResponse = false
	buf.Reset()
	ts.bodySizeLimit = 0
	_, err = ts.scrape(context.Background(), &buf)
	require.NoError(t, err)
	require.Equal(t, len(responseBody), buf.Len())
	// Target response gzip compressed body, scrape without body size limit.
	gzipResponse = true
	buf.Reset()
	_, err = ts.scrape(context.Background(), &buf)
	require.NoError(t, err)
	require.Equal(t, len(responseBody), buf.Len())
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

func (ts *testScraper) offset(time.Duration, uint64) time.Duration {
	return ts.offsetDur
}

func (ts *testScraper) Report(start time.Time, duration time.Duration, err error) {
	ts.lastStart = start
	ts.lastDuration = duration
	ts.lastError = err
}

func (ts *testScraper) scrape(ctx context.Context, w io.Writer) (string, error) {
	if ts.scrapeFunc != nil {
		return "", ts.scrapeFunc(ctx, w)
	}
	return "", ts.scrapeErr
}

func TestScrapeLoop_RespectTimestamps(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	app := s.Appender(context.Background())

	capp := &collectResultAppender{next: app}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { return capp },
		nil, 0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Now()
	slApp := sl.appender(context.Background())
	_, _, _, err := sl.append(slApp, []byte(`metric_a{a="1",b="1"} 1 0`), "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	want := []floatSample{
		{
			metric: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			t:      0,
			f:      1,
		},
	}
	require.Equal(t, want, capp.resultFloats, "Appended samples not as expected:\n%s", appender)
}

func TestScrapeLoop_DiscardTimestamps(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	app := s.Appender(context.Background())

	capp := &collectResultAppender{next: app}

	sl := newScrapeLoop(context.Background(),
		nil, nil, nil,
		nopMutator,
		nopMutator,
		func(ctx context.Context) storage.Appender { return capp },
		nil, 0,
		false,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)

	now := time.Now()
	slApp := sl.appender(context.Background())
	_, _, _, err := sl.append(slApp, []byte(`metric_a{a="1",b="1"} 1 0`), "", now)
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	want := []floatSample{
		{
			metric: labels.FromStrings("__name__", "metric_a", "a", "1", "b", "1"),
			t:      timestamp.FromTime(now),
			f:      1,
		},
	}
	require.Equal(t, want, capp.resultFloats, "Appended samples not as expected:\n%s", appender)
}

func TestScrapeLoopDiscardDuplicateLabels(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		&testScraper{},
		nil, nil,
		nopMutator,
		nopMutator,
		s.Appender,
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)
	defer cancel()

	// We add a good and a bad metric to check that both are discarded.
	slApp := sl.appender(ctx)
	_, _, _, err := sl.append(slApp, []byte("test_metric{le=\"500\"} 1\ntest_metric{le=\"600\",le=\"700\"} 1\n"), "", time.Time{})
	require.Error(t, err)
	require.NoError(t, slApp.Rollback())

	q, err := s.Querier(ctx, time.Time{}.UnixNano(), 0)
	require.NoError(t, err)
	series := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
	require.Equal(t, false, series.Next(), "series found in tsdb")
	require.NoError(t, series.Err())

	// We add a good metric to check that it is recorded.
	slApp = sl.appender(ctx)
	_, _, _, err = sl.append(slApp, []byte("test_metric{le=\"500\"} 1\n"), "", time.Time{})
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	q, err = s.Querier(ctx, time.Time{}.UnixNano(), 0)
	require.NoError(t, err)
	series = q.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "le", "500"))
	require.Equal(t, true, series.Next(), "series not found in tsdb")
	require.NoError(t, series.Err())
	require.Equal(t, false, series.Next(), "more than one series found in tsdb")
}

func TestScrapeLoopDiscardUnnamedMetrics(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	app := s.Appender(context.Background())

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(context.Background(),
		&testScraper{},
		nil, nil,
		func(l labels.Labels) labels.Labels {
			if l.Has("drop") {
				return labels.FromStrings("no", "name") // This label set will trigger an error.
			}
			return l
		},
		nopMutator,
		func(ctx context.Context) storage.Appender { return app },
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)
	defer cancel()

	slApp := sl.appender(context.Background())
	_, _, _, err := sl.append(slApp, []byte("nok 1\nnok2{drop=\"drop\"} 1\n"), "", time.Time{})
	require.Error(t, err)
	require.NoError(t, slApp.Rollback())
	require.Equal(t, errNameLabelMandatory, err)

	q, err := s.Querier(ctx, time.Time{}.UnixNano(), 0)
	require.NoError(t, err)
	series := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
	require.Equal(t, false, series.Next(), "series found in tsdb")
	require.NoError(t, series.Err())
}

func TestReusableConfig(t *testing.T) {
	variants := []*config.ScrapeConfig{
		{
			JobName:       "prometheus",
			ScrapeTimeout: model.Duration(15 * time.Second),
		},
		{
			JobName:       "httpd",
			ScrapeTimeout: model.Duration(15 * time.Second),
		},
		{
			JobName:       "prometheus",
			ScrapeTimeout: model.Duration(5 * time.Second),
		},
		{
			JobName:     "prometheus",
			MetricsPath: "/metrics",
		},
		{
			JobName:     "prometheus",
			MetricsPath: "/metrics2",
		},
		{
			JobName:       "prometheus",
			ScrapeTimeout: model.Duration(5 * time.Second),
			MetricsPath:   "/metrics2",
		},
		{
			JobName:        "prometheus",
			ScrapeInterval: model.Duration(5 * time.Second),
			MetricsPath:    "/metrics2",
		},
		{
			JobName:        "prometheus",
			ScrapeInterval: model.Duration(5 * time.Second),
			SampleLimit:    1000,
			MetricsPath:    "/metrics2",
		},
	}

	match := [][]int{
		{0, 2},
		{4, 5},
		{4, 6},
		{4, 7},
		{5, 6},
		{5, 7},
		{6, 7},
	}
	noMatch := [][]int{
		{1, 2},
		{0, 4},
		{3, 4},
	}

	for i, m := range match {
		require.Equal(t, true, reusableCache(variants[m[0]], variants[m[1]]), "match test %d", i)
		require.Equal(t, true, reusableCache(variants[m[1]], variants[m[0]]), "match test %d", i)
		require.Equal(t, true, reusableCache(variants[m[1]], variants[m[1]]), "match test %d", i)
		require.Equal(t, true, reusableCache(variants[m[0]], variants[m[0]]), "match test %d", i)
	}
	for i, m := range noMatch {
		require.Equal(t, false, reusableCache(variants[m[0]], variants[m[1]]), "not match test %d", i)
		require.Equal(t, false, reusableCache(variants[m[1]], variants[m[0]]), "not match test %d", i)
	}
}

func TestReuseScrapeCache(t *testing.T) {
	var (
		app = &nopAppendable{}
		cfg = &config.ScrapeConfig{
			JobName:        "Prometheus",
			ScrapeTimeout:  model.Duration(5 * time.Second),
			ScrapeInterval: model.Duration(5 * time.Second),
			MetricsPath:    "/metrics",
		}
		sp, _ = newScrapePool(cfg, app, 0, nil, &Options{})
		t1    = &Target{
			discoveredLabels: labels.FromStrings("labelNew", "nameNew", "labelNew1", "nameNew1", "labelNew2", "nameNew2"),
		}
		proxyURL, _ = url.Parse("http://localhost:2128")
	)
	defer sp.stop()
	sp.sync([]*Target{t1})

	steps := []struct {
		keep      bool
		newConfig *config.ScrapeConfig
	}{
		{
			keep: true,
			newConfig: &config.ScrapeConfig{
				JobName:        "Prometheus",
				ScrapeInterval: model.Duration(5 * time.Second),
				ScrapeTimeout:  model.Duration(5 * time.Second),
				MetricsPath:    "/metrics",
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:        "Prometheus",
				ScrapeInterval: model.Duration(5 * time.Second),
				ScrapeTimeout:  model.Duration(15 * time.Second),
				MetricsPath:    "/metrics2",
			},
		},
		{
			keep: true,
			newConfig: &config.ScrapeConfig{
				JobName:        "Prometheus",
				SampleLimit:    400,
				ScrapeInterval: model.Duration(5 * time.Second),
				ScrapeTimeout:  model.Duration(15 * time.Second),
				MetricsPath:    "/metrics2",
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:         "Prometheus",
				HonorTimestamps: true,
				SampleLimit:     400,
				ScrapeInterval:  model.Duration(5 * time.Second),
				ScrapeTimeout:   model.Duration(15 * time.Second),
				MetricsPath:     "/metrics2",
			},
		},
		{
			keep: true,
			newConfig: &config.ScrapeConfig{
				JobName:         "Prometheus",
				HonorTimestamps: true,
				SampleLimit:     400,
				HTTPClientConfig: config_util.HTTPClientConfig{
					ProxyConfig: config_util.ProxyConfig{ProxyURL: config_util.URL{URL: proxyURL}},
				},
				ScrapeInterval: model.Duration(5 * time.Second),
				ScrapeTimeout:  model.Duration(15 * time.Second),
				MetricsPath:    "/metrics2",
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:         "Prometheus",
				HonorTimestamps: true,
				HonorLabels:     true,
				SampleLimit:     400,
				ScrapeInterval:  model.Duration(5 * time.Second),
				ScrapeTimeout:   model.Duration(15 * time.Second),
				MetricsPath:     "/metrics2",
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:        "Prometheus",
				ScrapeInterval: model.Duration(5 * time.Second),
				ScrapeTimeout:  model.Duration(15 * time.Second),
				MetricsPath:    "/metrics",
				LabelLimit:     1,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:        "Prometheus",
				ScrapeInterval: model.Duration(5 * time.Second),
				ScrapeTimeout:  model.Duration(15 * time.Second),
				MetricsPath:    "/metrics",
				LabelLimit:     15,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:              "Prometheus",
				ScrapeInterval:       model.Duration(5 * time.Second),
				ScrapeTimeout:        model.Duration(15 * time.Second),
				MetricsPath:          "/metrics",
				LabelLimit:           15,
				LabelNameLengthLimit: 5,
			},
		},
		{
			keep: false,
			newConfig: &config.ScrapeConfig{
				JobName:               "Prometheus",
				ScrapeInterval:        model.Duration(5 * time.Second),
				ScrapeTimeout:         model.Duration(15 * time.Second),
				MetricsPath:           "/metrics",
				LabelLimit:            15,
				LabelNameLengthLimit:  5,
				LabelValueLengthLimit: 7,
			},
		},
	}

	cacheAddr := func(sp *scrapePool) map[uint64]string {
		r := make(map[uint64]string)
		for fp, l := range sp.loops {
			r[fp] = fmt.Sprintf("%p", l.getCache())
		}
		return r
	}

	for i, s := range steps {
		initCacheAddr := cacheAddr(sp)
		sp.reload(s.newConfig)
		for fp, newCacheAddr := range cacheAddr(sp) {
			if s.keep {
				require.Equal(t, initCacheAddr[fp], newCacheAddr, "step %d: old cache and new cache are not the same", i)
			} else {
				require.NotEqual(t, initCacheAddr[fp], newCacheAddr, "step %d: old cache and new cache are the same", i)
			}
		}
		initCacheAddr = cacheAddr(sp)
		sp.reload(s.newConfig)
		for fp, newCacheAddr := range cacheAddr(sp) {
			require.Equal(t, initCacheAddr[fp], newCacheAddr, "step %d: reloading the exact config invalidates the cache", i)
		}
	}
}

func TestScrapeAddFast(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		&testScraper{},
		nil, nil,
		nopMutator,
		nopMutator,
		s.Appender,
		nil,
		0,
		true,
		0, 0,
		nil,
		0,
		0,
		false,
		false,
		false,
		nil,
		false,
	)
	defer cancel()

	slApp := sl.appender(ctx)
	_, _, _, err := sl.append(slApp, []byte("up 1\n"), "", time.Time{})
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())

	// Poison the cache. There is just one entry, and one series in the
	// storage. Changing the ref will create a 'not found' error.
	for _, v := range sl.getCache().series {
		v.ref++
	}

	slApp = sl.appender(ctx)
	_, _, _, err = sl.append(slApp, []byte("up 1\n"), "", time.Time{}.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, slApp.Commit())
}

func TestReuseCacheRace(*testing.T) {
	var (
		app = &nopAppendable{}
		cfg = &config.ScrapeConfig{
			JobName:        "Prometheus",
			ScrapeTimeout:  model.Duration(5 * time.Second),
			ScrapeInterval: model.Duration(5 * time.Second),
			MetricsPath:    "/metrics",
		}
		sp, _ = newScrapePool(cfg, app, 0, nil, &Options{})
		t1    = &Target{
			discoveredLabels: labels.FromStrings("labelNew", "nameNew"),
		}
	)
	defer sp.stop()
	sp.sync([]*Target{t1})

	start := time.Now()
	for i := uint(1); i > 0; i++ {
		if time.Since(start) > 5*time.Second {
			break
		}
		sp.reload(&config.ScrapeConfig{
			JobName:        "Prometheus",
			ScrapeTimeout:  model.Duration(1 * time.Millisecond),
			ScrapeInterval: model.Duration(1 * time.Millisecond),
			MetricsPath:    "/metrics",
			SampleLimit:    i,
		})
	}
}

func TestCheckAddError(t *testing.T) {
	var appErrs appendErrors
	sl := scrapeLoop{l: log.NewNopLogger()}
	sl.checkAddError(nil, nil, nil, storage.ErrOutOfOrderSample, nil, nil, &appErrs)
	require.Equal(t, 1, appErrs.numOutOfOrder)
}

func TestScrapeReportSingleAppender(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	var (
		signal  = make(chan struct{}, 1)
		scraper = &testScraper{}
	)

	ctx, cancel := context.WithCancel(context.Background())
	sl := newScrapeLoop(ctx,
		scraper,
		nil, nil,
		nopMutator,
		nopMutator,
		s.Appender,
		nil,
		0,
		true,
		0, 0,
		nil,
		10*time.Millisecond,
		time.Hour,
		false,
		false,
		false,
		nil,
		false,
	)

	numScrapes := 0

	scraper.scrapeFunc = func(ctx context.Context, w io.Writer) error {
		numScrapes++
		if numScrapes%4 == 0 {
			return fmt.Errorf("scrape failed")
		}
		w.Write([]byte("metric_a 44\nmetric_b 44\nmetric_c 44\nmetric_d 44\n"))
		return nil
	}

	go func() {
		sl.run(nil)
		signal <- struct{}{}
	}()

	start := time.Now()
	for time.Since(start) < 3*time.Second {
		q, err := s.Querier(ctx, time.Time{}.UnixNano(), time.Now().UnixNano())
		require.NoError(t, err)
		series := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".+"))

		c := 0
		for series.Next() {
			i := series.At().Iterator(nil)
			for i.Next() != chunkenc.ValNone {
				c++
			}
		}

		require.Equal(t, 0, c%9, "Appended samples not as expected: %d", c)
		q.Close()
	}
	cancel()

	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("Scrape wasn't stopped.")
	}
}

func TestScrapeReportLimit(t *testing.T) {
	s := teststorage.New(t)
	defer s.Close()

	cfg := &config.ScrapeConfig{
		JobName:        "test",
		SampleLimit:    5,
		Scheme:         "http",
		ScrapeInterval: model.Duration(100 * time.Millisecond),
		ScrapeTimeout:  model.Duration(100 * time.Millisecond),
	}

	var (
		scrapes      int
		scrapedTwice = make(chan bool)
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "metric_a 44\nmetric_b 44\nmetric_c 44\nmetric_d 44\n")
		scrapes++
		if scrapes == 2 {
			close(scrapedTwice)
		}
	}))
	defer ts.Close()

	sp, err := newScrapePool(cfg, s, 0, nil, &Options{})
	require.NoError(t, err)
	defer sp.stop()

	testURL, err := url.Parse(ts.URL)
	require.NoError(t, err)
	sp.Sync([]*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: model.LabelValue(testURL.Host)}},
		},
	})

	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("target was not scraped twice")
	case <-scrapedTwice:
		// If the target has been scraped twice, report samples from the first
		// scrape have been inserted in the database.
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q, err := s.Querier(ctx, time.Time{}.UnixNano(), time.Now().UnixNano())
	require.NoError(t, err)
	defer q.Close()
	series := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "__name__", "up"))

	var found bool
	for series.Next() {
		i := series.At().Iterator(nil)
		for i.Next() == chunkenc.ValFloat {
			_, v := i.At()
			require.Equal(t, 1.0, v)
			found = true
		}
	}

	require.True(t, found)
}

func TestScrapeLoopLabelLimit(t *testing.T) {
	tests := []struct {
		title           string
		scrapeLabels    string
		discoveryLabels []string
		labelLimits     labelLimits
		expectErr       bool
	}{
		{
			title:           "Valid number of labels",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelLimit: 5},
			expectErr:       false,
		}, {
			title:           "Too many labels",
			scrapeLabels:    `metric{l1="1", l2="2", l3="3", l4="4", l5="5", l6="6"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelLimit: 5},
			expectErr:       true,
		}, {
			title:           "Too many labels including discovery labels",
			scrapeLabels:    `metric{l1="1", l2="2", l3="3", l4="4"} 0`,
			discoveryLabels: []string{"l5", "5", "l6", "6"},
			labelLimits:     labelLimits{labelLimit: 5},
			expectErr:       true,
		}, {
			title:           "Valid labels name length",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelNameLengthLimit: 10},
			expectErr:       false,
		}, {
			title:           "Label name too long",
			scrapeLabels:    `metric{label_name_too_long="0"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelNameLengthLimit: 10},
			expectErr:       true,
		}, {
			title:           "Discovery label name too long",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: []string{"label_name_too_long", "0"},
			labelLimits:     labelLimits{labelNameLengthLimit: 10},
			expectErr:       true,
		}, {
			title:           "Valid labels value length",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelValueLengthLimit: 10},
			expectErr:       false,
		}, {
			title:           "Label value too long",
			scrapeLabels:    `metric{l1="label_value_too_long"} 0`,
			discoveryLabels: nil,
			labelLimits:     labelLimits{labelValueLengthLimit: 10},
			expectErr:       true,
		}, {
			title:           "Discovery label value too long",
			scrapeLabels:    `metric{l1="1", l2="2"} 0`,
			discoveryLabels: []string{"l1", "label_value_too_long"},
			labelLimits:     labelLimits{labelValueLengthLimit: 10},
			expectErr:       true,
		},
	}

	for _, test := range tests {
		app := &collectResultAppender{}

		discoveryLabels := &Target{
			labels: labels.FromStrings(test.discoveryLabels...),
		}

		sl := newScrapeLoop(context.Background(),
			nil, nil, nil,
			func(l labels.Labels) labels.Labels {
				return mutateSampleLabels(l, discoveryLabels, false, nil)
			},
			func(l labels.Labels) labels.Labels {
				return mutateReportSampleLabels(l, discoveryLabels)
			},
			func(ctx context.Context) storage.Appender { return app },
			nil,
			0,
			true,
			0, 0,
			&test.labelLimits,
			0,
			0,
			false,
			false,
			false,
			nil,
			false,
		)

		slApp := sl.appender(context.Background())
		_, _, _, err := sl.append(slApp, []byte(test.scrapeLabels), "", time.Now())

		t.Logf("Test:%s", test.title)
		if test.expectErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.NoError(t, slApp.Commit())
		}
	}
}

func TestTargetScrapeIntervalAndTimeoutRelabel(t *testing.T) {
	interval, _ := model.ParseDuration("2s")
	timeout, _ := model.ParseDuration("500ms")
	config := &config.ScrapeConfig{
		ScrapeInterval: interval,
		ScrapeTimeout:  timeout,
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels: model.LabelNames{model.ScrapeIntervalLabel},
				Regex:        relabel.MustNewRegexp("2s"),
				Replacement:  "3s",
				TargetLabel:  model.ScrapeIntervalLabel,
				Action:       relabel.Replace,
			},
			{
				SourceLabels: model.LabelNames{model.ScrapeTimeoutLabel},
				Regex:        relabel.MustNewRegexp("500ms"),
				Replacement:  "750ms",
				TargetLabel:  model.ScrapeTimeoutLabel,
				Action:       relabel.Replace,
			},
		},
	}
	sp, _ := newScrapePool(config, &nopAppendable{}, 0, nil, &Options{})
	tgts := []*targetgroup.Group{
		{
			Targets: []model.LabelSet{{model.AddressLabel: "127.0.0.1:9090"}},
		},
	}

	sp.Sync(tgts)
	defer sp.stop()

	require.Equal(t, "3s", sp.ActiveTargets()[0].labels.Get(model.ScrapeIntervalLabel))
	require.Equal(t, "750ms", sp.ActiveTargets()[0].labels.Get(model.ScrapeTimeoutLabel))
}
