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
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/client_golang/extraction"

	pb "github.com/prometheus/prometheus/config/generated"

	"github.com/prometheus/prometheus/config"
)

type fakeTarget struct {
	scrapeCount int
	lastScrape  time.Time
	interval    time.Duration
}

func (t fakeTarget) LastError() error {
	return nil
}

func (t fakeTarget) URL() string {
	return "fake"
}

func (t fakeTarget) GlobalURL() string {
	return t.URL()
}

func (t fakeTarget) BaseLabels() clientmodel.LabelSet {
	return clientmodel.LabelSet{}
}

func (t fakeTarget) Interval() time.Duration {
	return t.interval
}

func (t fakeTarget) LastScrape() time.Time {
	return t.lastScrape
}

func (t fakeTarget) scrape(i extraction.Ingester) error {
	t.scrapeCount++

	return nil
}

func (t fakeTarget) RunScraper(ingester extraction.Ingester, interval time.Duration) {
	return
}

func (t fakeTarget) StopScraper() {
	return
}

func (t fakeTarget) State() TargetState {
	return Alive
}

func (t *fakeTarget) SetBaseLabelsFrom(newTarget Target) {}

func testTargetManager(t testing.TB) {
	targetManager := NewTargetManager(nopIngester{})
	testJob1 := config.JobConfig{
		JobConfig: pb.JobConfig{
			Name:           proto.String("test_job1"),
			ScrapeInterval: proto.String("1m"),
		},
	}
	testJob2 := config.JobConfig{
		JobConfig: pb.JobConfig{
			Name:           proto.String("test_job2"),
			ScrapeInterval: proto.String("1m"),
		},
	}

	target1GroupA := &fakeTarget{
		interval: time.Minute,
	}
	target2GroupA := &fakeTarget{
		interval: time.Minute,
	}

	targetManager.AddTarget(testJob1, target1GroupA)
	targetManager.AddTarget(testJob1, target2GroupA)

	target1GroupB := &fakeTarget{
		interval: time.Minute * 2,
	}

	targetManager.AddTarget(testJob2, target1GroupB)
}

func TestTargetManager(t *testing.T) {
	testTargetManager(t)
}

func BenchmarkTargetManager(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testTargetManager(b)
	}
}
