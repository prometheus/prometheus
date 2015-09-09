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
	"math/rand"
	"sync"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage"
)

// Scraper retrieves samples.
type Scraper interface {
	// Sends samples to ch until an error occurrs, the context is canceled
	// or no further samples can be retrieved. The channel is closed before returning.
	Scrape(ctx context.Context, ch chan<- *model.Sample) error
}

// Scrape pools runs scrapers for a set of targets where a target, identified by
// a label set is scraped exactly once.
type ScrapePool struct {
	appender storage.SampleAppender

	ctx     context.Context
	cancel  func()
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
func (sp *ScrapePool) Sync(targets []*Target) {
	sp.mtx.Lock()
	defer sp.mtx.Unlock()

	// Shutdown all running loops after their current scrape
	// is finished.
	for _, l := range sp.loops {
		go l.stop()
	}

	// Old loops are shutting down, create new from the targets.
	sp.loops = map[model.Fingerprint]loop{}

	for _, t := range targets {
		fp := t.fingerprint()

		// Create states for new targets.
		state, ok := sp.states[fp]
		if !ok {
			state = &TargetStatus{}
			sp.states[fp] = state
		}

		// Create and start loops the first time we see a target.
		if _, ok := sp.loops[fp]; !ok {
			loop := sp.newLoop(sp.appender, t, state)
			sp.loops[fp] = loop

			sp.running.Add(1)
			go func(t *Target) {
				loop.run(sp.ctx, t.interval(), t.timeout(), nil)
				sp.running.Done()
			}(t)
		}
	}

	// Drop states of targets that no longer exist.
	for fp := range sp.states {
		if _, ok := sp.loops[fp]; !ok {
			delete(sp.states, fp)
		}
	}
}

type loop interface {
	run(ctx context.Context, interval, timeout time.Duration, errc chan<- error)
	stop()
}

// scrapeLoop manages scraping cycles.
type scrapeLoop struct {
	scraper  Scraper
	state    *TargetStatus
	appender storage.SampleAppender

	cancel      func()
	stopc, done chan struct{}
	mtx         sync.RWMutex
}

func newScrapeLoop(app storage.SampleAppender, target *Target, state *TargetStatus) loop {
	return &scrapeLoop{
		scraper:  target,
		appender: target.wrapAppender(app),
		state:    state,
	}
}

func (sl *scrapeLoop) active() bool {
	return sl.cancel != nil
}

func (sl *scrapeLoop) run(ctx context.Context, interval, timeout time.Duration, errc chan<- error) {
	if sl.active() {
		return
	}

	ctx, sl.cancel = context.WithCancel(ctx)

	sl.stopc = make(chan struct{})
	sl.done = make(chan struct{})

	defer func() {
		close(sl.done)
		sl.cancel = nil
	}()

	select {
	case <-time.After(time.Duration(float64(interval) * rand.Float64())):
		// Continue after random scraping offset.
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

		select {
		case <-sl.stopc:
			return
		case <-ctx.Done():
			return
		default:
		}

		<-ticker.C
	}
}

// kill terminates the scraping loop immediately.
func (sl *scrapeLoop) kill() {
	if !sl.active() {
		return
	}
	sl.cancel()
}

// stop finishes any pending scrapes and then kills the loop.
func (sl *scrapeLoop) stop() {
	if !sl.active() {
		return
	}
	close(sl.stopc)
	<-sl.done

	sl.kill()
}

// scrape executes a single Scrape and appends the retrieved samples.
func (sl *scrapeLoop) scrape(ctx context.Context, timeout time.Duration) error {
	ch := make(chan *model.Sample, ingestedSamplesCap)

	// Receive and append all the samples retrieved by the scraper.
	go func() {
		var (
			smpl *model.Sample
			ok   bool
		)
		for {
			select {
			case smpl, ok = <-ch:
				if !ok {
					return
				}
			case <-ctx.Done():
				return
			}
			// TODO(fabxc): Change the SampleAppender interface to return an error
			// so we can proceed based on the status and don't leak goroutines trying
			// to append a single sample after dropping all the other ones.
			sl.appender.Append(smpl)
		}
	}()

	ctx, _ = context.WithTimeout(ctx, timeout)

	return sl.scraper.Scrape(ctx, ch)
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

	sl.appender.Append(healthSample)
	sl.appender.Append(durationSample)
}
