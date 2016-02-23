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
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	// "github.com/prometheus/prometheus/storage"
)

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
	scraper.scrapeFunc = func(ctx context.Context) (model.Samples, error) {
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

// testScraper implements the scraper interface and allows setting values
// returned by its methods. It also allows setting a custom scrape function.
type testScraper struct {
	offsetDur time.Duration

	lastStart    time.Time
	lastDuration time.Duration
	lastError    error

	samples    model.Samples
	scrapeErr  error
	scrapeFunc func(context.Context) (model.Samples, error)
}

func (ts *testScraper) offset(interval time.Duration) time.Duration {
	return ts.offsetDur
}

func (ts *testScraper) report(start time.Time, duration time.Duration, err error) {
	ts.lastStart = start
	ts.lastDuration = duration
	ts.lastError = err
}

func (ts *testScraper) scrape(ctx context.Context) (model.Samples, error) {
	if ts.scrapeFunc != nil {
		return ts.scrapeFunc(ctx)
	}
	return ts.samples, ts.scrapeErr
}
