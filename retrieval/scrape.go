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
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage"
)

// scraper retrieves samples.
type scraper interface {
	// Sends samples to ch until an error occurrs, the context is canceled
	// or no further samples can be retrieved. The channel is closed before returning.
	Scrape(ctx context.Context, ch chan<- model.Vector) error
}

// Scrape pools runs scrapers for a set of targets where a target, identified by
// a label set, is scraped exactly once.
type ScrapePool struct {
	appender storage.SampleAppender

	ctx    context.Context
	cancel func()

	// A constructor function for new loops.
	newLoop func(storage.SampleAppender, *Target, *TargetStatus) loop

	mtx    sync.RWMutex
	loops  map[model.Fingerprint]loop
	states map[model.Fingerprint]*TargetStatus

	// running keeps track of running loops.
	running sync.WaitGroup
}

func NewScrapePool(app storage.SampleAppender) *ScrapePool {
	sp := &ScrapePool{
		appender: app,
		states:   map[model.Fingerprint]*TargetStatus{},
		loops:    map[model.Fingerprint]loop{},
		newLoop:  newScrapeLoop,
	}
	sp.ctx, sp.cancel = context.WithCancel(context.Background())

	return sp
}

func (sp *ScrapePool) Stop() {
	if sp.cancel != nil {
		sp.cancel()
	}
	sp.running.Wait()
}

// Sync the running scrapers with the provided list of targets.
func (sp *ScrapePool) Sync(rawTargets []*Target) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	targets := map[model.Fingerprint]*Target{}
	for _, t := range rawTargets {
		targets[t.fingerprint()] = t
	}

	// Stop loops for removed targets and remove them after
	// they exited succesfully.
	for fp, l := range sp.loops {
		if _, ok := targets[fp]; ok {
			continue
		}
		delete(sp.states, fp)

		go func(fp model.Fingerprint, l loop) {
			l.stop()

			// Cleanup after ourselves if there isn't a new loop
			// for the fingerprint already.
			sp.mtx.Lock()
			if sp.loops[fp] == l {
				delete(sp.loops, fp)
			}
			sp.mtx.Unlock()
		}(fp, l)
	}

	// Start loops for the target set. Synchronously stop existing loops
	// for previous targets.
	for fp, t := range targets {
		// Create states for new targets.
		state, ok := sp.states[fp]
		if !ok {
			state = &TargetStatus{}
			sp.states[fp] = state
		}

		oldLoop, ok := sp.loops[fp]
		loop := sp.newLoop(sp.appender, t, state)

		sp.running.Add(1)
		go func(t *Target) {
			if ok {
				oldLoop.stop()
			}
			loop.run(sp.ctx, t.interval(), t.timeout(), nil)
			sp.running.Done()
		}(t)

		sp.loops[fp] = loop
	}
}

type loop interface {
	run(ctx context.Context, interval, timeout time.Duration, errc chan<- error)
	stop()
}

// scrapeLoop manages scraping cycles.
type scrapeLoop struct {
	scraper        scraper
	state          *TargetStatus
	appender       storage.SampleAppender
	reportAppender storage.SampleAppender

	// A function to calculate an initial offset for a given
	// scrape interval.
	offset func(time.Duration) time.Duration

	stopc, done chan struct{}
	mtx         sync.RWMutex
}

func newScrapeLoop(app storage.SampleAppender, target *Target, state *TargetStatus) loop {
	return &scrapeLoop{
		scraper:        target,
		appender:       target.wrapAppender(app, true),
		reportAppender: target.wrapAppender(app, false),
		state:          state,
		offset:         target.offset,
	}
}

func (sl *scrapeLoop) active() bool {
	if sl.done == nil {
		return false
	}

	select {
	case <-sl.done:
		return false
	default:
		return true
	}
}

func (sl *scrapeLoop) run(ctx context.Context, interval, timeout time.Duration, errc chan<- error) {
	sl.mtx.Lock()

	if sl.active() {
		sl.mtx.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(ctx)

	sl.stopc = make(chan struct{})
	sl.done = make(chan struct{})

	defer cancel()
	defer close(sl.done)

	sl.mtx.Unlock()

	// If no offset function is provided, default to starting immediately.
	if sl.offset == nil {
		sl.offset = func(time.Duration) time.Duration { return 0 }
	}

	select {
	case <-time.After(sl.offset(interval)):
		// Continue after a scraping offset.
	case <-ctx.Done():
		return
	case <-sl.stopc:
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		start := time.Now()

		err := sl.scrape(ctx, timeout)
		if err != nil {
			log.Debugf("Scraping error: %s", err)
		}

		sl.report(start, time.Since(start), err)
		if errc != nil {
			errc <- err
		}

		// First check without the ticker to not initiate
		// another scrape if termination was requested.
		select {
		case <-sl.stopc:
			return
		case <-ctx.Done():
			return
		default:
		}

		select {
		case <-sl.stopc:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// stop finishes any pending scrapes and then kills the loop.
func (sl *scrapeLoop) stop() {
	sl.mtx.Lock()
	defer sl.mtx.Unlock()

	if !sl.active() {
		return
	}
	close(sl.stopc)
	<-sl.done
}

// scrape executes a single Scrape and appends the retrieved samples.
func (sl *scrapeLoop) scrape(ctx context.Context, timeout time.Duration) error {
	var (
		ch   = make(chan model.Vector)
		done = make(chan struct{})
	)
	// Receive and append all the samples retrieved by the scraper.
	go func() {
		defer close(done)

		var (
			samples model.Vector
			ok      bool
		)
		for {
			select {
			case samples, ok = <-ch:
				if !ok {
					return
				}
			case <-ctx.Done():
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}

			// TODO(fabxc): Change the SampleAppender interface to return an error
			// so we can proceed based on the status and don't leak goroutines trying
			// to append a single sample after dropping all the other ones.
			for _, smpl := range samples {
				sl.appender.Append(smpl)
			}
		}
	}()

	scrapeCtx, _ := context.WithTimeout(ctx, timeout)

	err := sl.scraper.Scrape(scrapeCtx, ch)
	<-done

	return err
}

func (sl *scrapeLoop) report(start time.Time, duration time.Duration, err error) {
	sl.state.setLastScrape(start)
	sl.state.setLastError(err)

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
		Value:     model.SampleValue(float64(duration) / float64(time.Second)),
	}

	sl.reportAppender.Append(healthSample)
	sl.reportAppender.Append(durationSample)
}
