// Copyright 2015 The Prometheus Authors
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
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
)

func TestNewScrapePool(t *testing.T) {
	app := &nopAppender{}
	sp := NewScrapePool(app)

	if !reflect.DeepEqual(app, sp.appender) {
		t.Fatalf("Wrong sample appender")
	}

	if sp.ctx == nil || sp.cancel == nil {
		t.Fatalf("Context not initialized")
	}

	if sp.newLoop == nil {
		t.Fatalf("newLoop function not initializated")
	}
}

func TestScrapePoolStop(t *testing.T) {
	sp := NewScrapePool(&nopAppender{})

	sp.Stop()

	select {
	case <-sp.ctx.Done():
	default:
		t.Fatalf("Context was not canceled")
	}
}

type testLoop struct {
	target           *Target
	status           *TargetStatus
	started, stopped int
}

func newTestLoop(app storage.SampleAppender, t *Target, s *TargetStatus) loop {
	return &testLoop{target: t, status: s}
}

func (l *testLoop) run(ctx context.Context, interval, timeout time.Duration, errc chan<- error) {
	l.started++
}

func (l *testLoop) stop() {
	l.stopped++
}

func TestScrapePoolSync(t *testing.T) {
	cfg := &config.ScrapeConfig{
		ScrapeInterval: config.Duration(1 * time.Hour),
		ScrapeTimeout:  config.Duration(1 * time.Hour),
	}
	targets := []Target{
		{
			labels: model.LabelSet{
				model.MetricsPathLabel: "/metrics",
				model.AddressLabel:     "example.com:8888",
				model.SchemeLabel:      "http",
			},
			scrapeConfig: cfg,
		}, {
			labels: model.LabelSet{
				model.MetricsPathLabel: "/metrics",
				model.AddressLabel:     "example.com:8889",
				model.SchemeLabel:      "http",
			},
			scrapeConfig: cfg,
		},
	}

	suite := []struct {
		all    []*Target
		unique []*Target
	}{
		{
			all:    []*Target{},
			unique: []*Target{},
		},
		{
			all:    []*Target{&targets[0], &targets[1]},
			unique: []*Target{&targets[0], &targets[1]},
		},
		{
			all:    []*Target{&targets[0], &targets[1], &targets[0]},
			unique: []*Target{&targets[0], &targets[1]},
		},
		{
			all:    []*Target{&targets[0]},
			unique: []*Target{&targets[0]},
		},
		{
			all:    nil,
			unique: nil,
		},
	}

	sp := NewScrapePool(&nopAppender{})
	sp.newLoop = newTestLoop

	var (
		lastLoops  map[model.Fingerprint]loop
		lastStates map[model.Fingerprint]*TargetStatus
	)

	for i, step := range suite {
		sp.Sync(step.all)

		// Let loops stop and start and Sync is not blocking until this is done.
		time.Sleep(5 * time.Millisecond)

		if len(sp.loops) != len(step.unique) {
			t.Fatalf("step %d: wrong number of unique targets: want %d, got %d", i, len(step.unique), len(sp.loops))
		}
		if len(sp.states) != len(step.unique) {
			t.Fatalf("step %d: wrong number of scrape states: want %d, got %d", i, len(step.unique), len(sp.states))
		}

		for _, l := range sp.loops {
			tl, ok := l.(*testLoop)
			if !ok {
				t.Fatalf("step %d: wrong loop type instantiated", i)
			}

			if tl.started != 1 {
				t.Fatalf("step %d: loop was not started exactly once, got %d", i, tl.started)
			}
			if tl.stopped != 0 {
				t.Fatalf("step %d: loop stopped unexpectedly", i)
			}

			found := false
			for _, target := range step.unique {
				if target.fingerprint() != tl.target.fingerprint() {
					continue
				}
				found = true

				// Check whether the loop is instantiated with the correct state.
				if tl.status != sp.states[target.fingerprint()] {
					t.Fatalf("step %d: unexpected status in loop: got %p, want %p", i, tl.status, sp.states[target.fingerprint()])
				}
				break
			}
			if !found {
				t.Fatalf("step %d: unexpected target %q")
			}
		}

		for _, l := range lastLoops {
			tl := l.(*testLoop)

			if tl.stopped != 1 || tl.started != 1 {
				t.Fatalf("step %d: removed loop not stopped properly", i)
			}
		}

		for fp, oldState := range lastStates {
			if newState, ok := sp.states[fp]; ok {
				if oldState != newState {
					t.Fatalf("step %d: state was not kept for target", i)
				}
			}
		}

		lastLoops = sp.loops
		lastStates = sp.states
	}
}

type testScraper struct {
	block, active chan struct{}
	samples       []*model.Sample
}

func newTestScraper() *testScraper {
	return &testScraper{
		block:  make(chan struct{}, 1),
		active: make(chan struct{}),
	}
}

func (s *testScraper) Scrape(ctx context.Context, ch chan<- *model.Sample) error {
	defer close(ch)

	s.active <- struct{}{}

	for _, smpl := range s.samples {
		select {
		case ch <- smpl:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	select {
	case <-s.block:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func TestScrapeLoopRun(t *testing.T) {
	var (
		scraper  = newTestScraper()
		interval = 1 * time.Microsecond
		timeout  = 1 * time.Second
		errc     = make(chan error)
	)

	sl := &scrapeLoop{
		scraper:  scraper,
		appender: nopAppender{},
		state:    &TargetStatus{},
	}

	checkStatus := func(h TargetHealth) {
		if sl.state.health != h {
			if h == HealthGood {
				t.Errorf("Unexpected scrape error: %q", sl.state.LastError())
			}
			t.Fatalf("Expected target health %q, got %q", h, sl.state.Health())
		}
		if sl.state.health != HealthUnknown && sl.state.LastScrape().IsZero() {
			t.Fatalf("Missing timestamp for scrape")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the loop but cancel the base context before.
	// It must exit immediately.
	t.Log("Test loop on canceled context")

	cancel()

	exited := make(chan struct{})
	go func() {
		sl.run(ctx, interval, timeout, errc)
		close(exited)
	}()

	select {
	case <-scraper.active:
		t.Fatalf("Scraper called unexpectedly")
	case <-time.After(timeout):
		t.Fatalf("Expected scrape loop to exit")
	case <-exited:
	}

	checkStatus(HealthUnknown)

	// Perform regular scrapes.
	t.Log("Test loop for regular scrapes")

	ctx, cancel = context.WithCancel(context.Background())
	go sl.run(ctx, interval, timeout, errc)

	for i := 0; i < 100; i++ {
		select {
		case <-scraper.active:
		case <-time.After(timeout):
			t.Fatalf("Expected scrape was not initiated")
		}
		scraper.block <- struct{}{}

		// The error of a scrape (including nil) is sent on errc
		// after the scrape status was updated.
		<-errc
		checkStatus(HealthGood)
	}

	// Start loop again but hard-kill it, the scraper should be signaled to
	// exit. The test scraper returns without us unblocking it in that case.
	t.Log("Test hard-killing scraper")

	select {
	case <-scraper.active:
	case <-time.After(timeout):
		t.Fatalf("Expected scrape was not initiated")
	}

	sl.kill()

	select {
	case err := <-errc:
		if err != context.Canceled {
			t.Fatalf("Unexpected error: %s", err)
		}
	case <-time.After(timeout):
		t.Fatalf("Scrape was not canceled")
	}

	checkStatus(HealthBad)

	select {
	case <-sl.done:
	case <-time.After(timeout):
		t.Fatalf("Loop did not terminate for canceled context")
	}

	// Start loop again and stop, the scraper should be signaled to
	// exit but finish its scrape.
	t.Log("Test soft-killing scraper")

	ctx, cancel = context.WithCancel(context.Background())
	go sl.run(ctx, interval, timeout, errc)

	select {
	case <-scraper.active:
	case <-time.After(timeout):
		t.Fatalf("Expected scrape was not initiated")
	}

	go sl.stop()
	// Wait for stop signal to be sent.
	<-sl.stopc

	scraper.block <- struct{}{}

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
	case <-time.After(timeout):
		t.Fatalf("Expected scraper to exit on kill")
	}

	checkStatus(HealthGood)
}

func TestScrapeLoopReport(t *testing.T) {
	sl := &scrapeLoop{
		scraper: &testScraper{},
		state:   &TargetStatus{},
	}

	suite := []struct {
		start    model.Time
		duration time.Duration
		err      error
	}{
		{
			start:    model.TimeFromUnix(time.Now().Unix()),
			duration: 3 * time.Second,
		},
		{
			start:    model.TimeFromUnix(time.Now().Unix()),
			duration: 5 * time.Second,
			err:      fmt.Errorf("error"),
		},
	}

	for _, test := range suite {
		appender := &collectAppender{}
		sl.appender = appender

		sl.report(test.start.Time(), test.duration, test.err)

		if sl.state.LastError() != test.err {
			t.Fatalf("Expected error %q but got %q", test.err, sl.state.LastError())
		}

		if sl.state.LastScrape() != test.start.Time() {
			t.Fatalf("Expeceted test start at %v but got %v", test.start.Time(), sl.state.LastScrape())
		}

		if len(appender.samples) != 2 {
			t.Fatalf("Expected two samples, got %d", len(appender.samples))
		}

		expHealth := model.SampleValue(0)
		if test.err == nil {
			expHealth = 1
		}

		expected := &model.Sample{
			Metric: model.Metric{
				model.MetricNameLabel: scrapeHealthMetricName,
			},
			Timestamp: test.start,
			Value:     expHealth,
		}

		actual := appender.samples[0]

		if !actual.Equal(expected) {
			t.Fatalf("Expected and actual samples not equal, want: %v, got: %v", expected, actual)
		}

		expected = &model.Sample{
			Metric: model.Metric{
				model.MetricNameLabel: scrapeDurationMetricName,
			},
			Timestamp: test.start,
			Value:     model.SampleValue(test.duration.Seconds()),
		}

		actual = appender.samples[1]

		if !actual.Equal(expected) {
			t.Fatalf("Expected and actual samples not equal, want: %v, got %v", expected, actual)
		}
	}
}

type redirectAppender struct {
	ch chan *model.Sample
}

func newRedirectAppender() *redirectAppender {
	return &redirectAppender{ch: make(chan *model.Sample)}
}

func (a *redirectAppender) Append(s *model.Sample) {
	a.ch <- s
}

func TestScrapeLoopScrape(t *testing.T) {
	var (
		scraper  = newTestScraper()
		appender = newRedirectAppender()
		timeout  = 1 * time.Second
		errc     = make(chan error)
	)
	sl := &scrapeLoop{
		scraper:  scraper,
		appender: appender,
		state:    &TargetStatus{},
	}

	scraper.samples = make([]*model.Sample, 2*ingestedSamplesCap)

	// Perform a regular scrape where all appends succeed immediately.
	t.Log("Test regular scraping")

	go func() {
		errc <- sl.scrape(context.Background(), timeout)
	}()

	<-scraper.active
	// Receive all samples.
	for i := 0; i < 2*ingestedSamplesCap; i++ {
		select {
		case <-appender.ch:
		case <-time.After(timeout):
			t.Fatalf("Timeout on expecting sample %d", i)
		}
	}

	select {
	case <-appender.ch:
		t.Fatalf("Received more sample than sent by the scraper")
	default:
	}
	scraper.block <- struct{}{}

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Unexpected scrape error: %s", err)
		}
	case <-time.After(3 * timeout):
		t.Fatalf("Timeout on scrape")
	}

	// Test appending with context being canceled at random points in time.
	t.Log("Test scraping with context cancelation")

	for numIngested := 0; numIngested < 2*ingestedSamplesCap; numIngested += 8 {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			errc <- sl.scrape(ctx, timeout)
		}()

		<-scraper.active
		//
		for i := 0; i < numIngested; i++ {
			select {
			case <-appender.ch:
			case <-time.After(timeout):
				t.Fatalf("Timeout on expecting sample %d", i)
			}
		}
		cancel()

		// With the current SampleAppender behavior we cannot terminate
		// mid-appending. We may have to consume one more sample before the
		// cancelation takes effect.
		select {
		case <-appender.ch:
		default:
		}

		select {
		case <-appender.ch:
			t.Fatalf("Received more samples after context cancelation")
		default:
		}

		select {
		case err := <-errc:
			if err != context.Canceled {
				t.Fatalf("Unexpected scrape error: %s", err)
			}
		case <-time.After(3 * timeout):
			t.Fatalf("Timeout on scrape")
		}
	}
}
