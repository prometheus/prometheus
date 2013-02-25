// Copyright 2013 Prometheus Team
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
	"github.com/prometheus/prometheus/retrieval/format"
	"testing"
	"time"
)

func TestTargetScrapeUpdatesState(t *testing.T) {
	testTarget := target{
		scheduler: literalScheduler{},
		state:     UNKNOWN,
		address:   "bad schema",
	}
	testTarget.Scrape(time.Time{}, make(chan format.Result))
	if testTarget.state != UNREACHABLE {
		t.Errorf("Expected target state %v, actual: %v", UNREACHABLE, testTarget.state)
	}
}
