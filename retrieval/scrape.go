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

// import (
// 	"sync"
// 	"time"

// 	"github.com/prometheus/common/log"
// 	"github.com/prometheus/common/model"
// 	"golang.org/x/net/context"

// 	"github.com/prometheus/prometheus/config"
// 	"github.com/prometheus/prometheus/storage"
// )

// type scraper interface {
// 	scrape(context.Context) error
// 	report(start time.Time, dur time.Duration, err error) error
// }

// type scrapePool struct {
// 	mtx     sync.RWMutex
// 	targets map[model.Fingerprint]*Target
// 	loops   map[model.Fingerprint]loop

// 	config *config.ScrapeConfig

// 	newLoop func(context.Context)
// }

// func newScrapePool(c *config.ScrapeConfig) *scrapePool {
// 	return &scrapePool{config: c}
// }

// func (sp *scrapePool) sync(targets []*Target) {
// 	sp.mtx.Lock()
// 	defer sp.mtx.Unlock()

// 	uniqueTargets := make(map[string]*Target{}, len(targets))

// 	for _, t := range targets {
// 		uniqueTargets[t.fingerprint()] = t
// 	}

// 	sp.targets = uniqueTargets
// }

// type scrapeLoop struct {
// 	scraper scraper
// 	mtx     sync.RWMutex
// }

// func newScrapeLoop(ctx context.Context)

// func (sl *scrapeLoop) update() {}

// func (sl *scrapeLoop) run(ctx context.Context) {
// 	var wg sync.WaitGroup

// 	wg.Wait()
// }

// func (sl *scrapeLoop) stop() {

// }
