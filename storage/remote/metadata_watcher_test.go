// Copyright 2020 The Prometheus Authors
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

package remote

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/scrape"
)

var (
	interval = model.Duration(1 * time.Millisecond)
	deadline = 1 * time.Millisecond
)

// TestMetaStore satisfies the MetricMetadataStore interface.
// It is used to inject specific metadata as part of a test case.
type TestMetaStore struct {
	Metadata []scrape.MetricMetadata
}

func (s *TestMetaStore) ListMetadata() []scrape.MetricMetadata {
	return s.Metadata
}

func (s *TestMetaStore) GetMetadata(mfName string) (scrape.MetricMetadata, bool) {
	for _, m := range s.Metadata {
		if mfName == m.MetricFamily {
			return m, true
		}
	}

	return scrape.MetricMetadata{}, false
}

func (*TestMetaStore) SizeMetadata() int   { return 0 }
func (*TestMetaStore) LengthMetadata() int { return 0 }

type writeMetadataToMock struct {
	metadataAppended int
}

func (mwtm *writeMetadataToMock) AppendWatcherMetadata(_ context.Context, m []scrape.MetricMetadata) {
	mwtm.metadataAppended += len(m)
}

func newMetadataWriteToMock() *writeMetadataToMock {
	return &writeMetadataToMock{}
}

type scrapeManagerMock struct {
	manager *scrape.Manager
	ready   bool
}

func (smm *scrapeManagerMock) Get() (*scrape.Manager, error) {
	if smm.ready {
		return smm.manager, nil
	}

	return nil, errors.New("not ready")
}

type fakeManager struct {
	activeTargets map[string][]*scrape.Target
}

func (fm *fakeManager) TargetsActive() map[string][]*scrape.Target {
	return fm.activeTargets
}

func TestWatchScrapeManager_NotReady(t *testing.T) {
	wt := newMetadataWriteToMock()
	smm := &scrapeManagerMock{
		ready: false,
	}

	mw := NewMetadataWatcher(nil, smm, "", wt, interval, deadline)
	require.False(t, mw.ready())

	mw.collect()

	require.Equal(t, 0, wt.metadataAppended)
}

func TestWatchScrapeManager_ReadyForCollection(t *testing.T) {
	wt := newMetadataWriteToMock()

	metadata := &TestMetaStore{
		Metadata: []scrape.MetricMetadata{
			{
				MetricFamily: "prometheus_tsdb_head_chunks_created",
				Type:         model.MetricTypeCounter,
				Help:         "Total number",
				Unit:         "",
			},
			{
				MetricFamily: "prometheus_remote_storage_retried_samples",
				Type:         model.MetricTypeCounter,
				Help:         "Total number",
				Unit:         "",
			},
		},
	}
	metadataDup := &TestMetaStore{
		Metadata: []scrape.MetricMetadata{
			{
				MetricFamily: "prometheus_tsdb_head_chunks_created",
				Type:         model.MetricTypeCounter,
				Help:         "Total number",
				Unit:         "",
			},
		},
	}

	target := &scrape.Target{}
	target.SetMetadataStore(metadata)
	targetWithDup := &scrape.Target{}
	targetWithDup.SetMetadataStore(metadataDup)

	manager := &fakeManager{
		activeTargets: map[string][]*scrape.Target{
			"job": {target},
			"dup": {targetWithDup},
		},
	}

	smm := &scrapeManagerMock{
		ready: true,
	}

	mw := NewMetadataWatcher(nil, smm, "", wt, interval, deadline)
	mw.manager = manager

	mw.collect()

	require.Equal(t, 2, wt.metadataAppended)
}
