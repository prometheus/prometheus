// Copyright 2013 The Prometheus Authors
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
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
)

type nopAppender struct{}

func (a nopAppender) Append(*clientmodel.Sample) {
}

type slowAppender struct{}

func (a slowAppender) Append(*clientmodel.Sample) {
	time.Sleep(time.Millisecond)
	return
}

type collectResultAppender struct {
	result clientmodel.Samples
}

func (a *collectResultAppender) Append(s *clientmodel.Sample) {
	for ln, lv := range s.Metric {
		if len(lv) == 0 {
			delete(s.Metric, ln)
		}
	}
	a.result = append(a.result, s)
}

// fakeTargetProvider implements a TargetProvider and allows manual injection
// of TargetGroups through the update channel.
type fakeTargetProvider struct {
	sources []string
	update  chan *config.TargetGroup
}

func (tp *fakeTargetProvider) Run(ch chan<- *config.TargetGroup, done <-chan struct{}) {
	defer close(ch)
	for {
		select {
		case tg := <-tp.update:
			ch <- tg
		case <-done:
			return
		}
	}
}

func (tp *fakeTargetProvider) Sources() []string {
	return tp.sources
}
