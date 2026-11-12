// Copyright The Prometheus Authors
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
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/logging"
)

// String constants for instrumentation.
const (
	namespace  = "prometheus"
	subsystem  = "remote_storage"
	remoteName = "remote_name"
	endpoint   = "url"
)

type ReadyScrapeManager interface {
	Get() (*scrape.Manager, error)
}

// startTimeCallback is a callback func that return the oldest timestamp stored in a storage.
type startTimeCallback func() (int64, error)

// Storage represents all the remote read and write endpoints.  It implements
// storage.Storage.
type Storage struct {
	deduper *logging.Deduper
	logger  *slog.Logger
	mtx     sync.Mutex

	rws *WriteStorage

	// For reads.
	queryables []storage.SampleAndChunkQueryable
	// required holds, for each queryable, whether its errors must fail the query.
	required               []bool
	localStartTimeCallback startTimeCallback
}

var (
	_ storage.Storage                 = &Storage{}
	_ storage.RequiredQueriersStorage = &Storage{}
)

// NewStorage returns a remote.Storage.
func NewStorage(l *slog.Logger, reg prometheus.Registerer, stCallback startTimeCallback, walDir string, flushDeadline time.Duration, sm ReadyScrapeManager, enableTypeAndUnitLabels, enableMetadataWALRecords, enableSTStorage bool) *Storage {
	if l == nil {
		l = promslog.NewNopLogger()
	}
	deduper := logging.Dedupe(l, 1*time.Minute)
	logger := slog.New(deduper)

	s := &Storage{
		logger:                 logger,
		deduper:                deduper,
		localStartTimeCallback: stCallback,
	}
	s.rws = NewWriteStorage(s.logger, reg, walDir, flushDeadline, sm, enableTypeAndUnitLabels, enableMetadataWALRecords, enableSTStorage)
	return s
}

func (s *Storage) Notify() {
	s.rws.Notify()
}

// ApplyConfig updates the state as the new config requires.
func (s *Storage) ApplyConfig(conf *config.Config) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if err := s.rws.ApplyConfig(conf); err != nil {
		return err
	}

	// Update read clients
	readHashes := make(map[string]struct{})
	queryables := make([]storage.SampleAndChunkQueryable, 0, len(conf.RemoteReadConfigs))
	required := make([]bool, 0, len(conf.RemoteReadConfigs))
	for _, rrConf := range conf.RemoteReadConfigs {
		hash, err := toHash(rrConf)
		if err != nil {
			return err
		}

		// Don't allow duplicate remote read configs.
		if _, ok := readHashes[hash]; ok {
			return fmt.Errorf("duplicate remote read configs are not allowed, found duplicate for URL: %s", rrConf.URL)
		}
		readHashes[hash] = struct{}{}

		// Set the queue name to the config hash if the user has not set
		// a name in their remote write config so we can still differentiate
		// between queues that have the same remote write endpoint.
		name := hash[:6]
		if rrConf.Name != "" {
			name = rrConf.Name
		}

		c, err := NewReadClient(name, &ClientConfig{
			URL:              rrConf.URL,
			Timeout:          rrConf.RemoteTimeout,
			ChunkedReadLimit: rrConf.ChunkedReadLimit,
			HTTPClientConfig: rrConf.HTTPClientConfig,
			Headers:          rrConf.Headers,
		})
		if err != nil {
			return err
		}

		externalLabels := conf.GlobalConfig.ExternalLabels
		if !rrConf.FilterExternalLabels {
			externalLabels = labels.EmptyLabels()
		}
		queryables = append(queryables, NewSampleAndChunkQueryableClient(
			c,
			externalLabels,
			labelsToEqualityMatchers(rrConf.RequiredMatchers),
			rrConf.ReadRecent,
			s.localStartTimeCallback,
		))
		required = append(required, rrConf.Required)
	}
	s.queryables = queryables
	s.required = required

	return nil
}

// StartTime implements the Storage interface.
func (*Storage) StartTime() (int64, error) {
	return int64(model.Latest), nil
}

// Querier returns a storage.MergeQuerier combining the remote client queriers
// of each configured remote read endpoint.
// Returned querier will never return error as all queryables are treated as best effort here.
// Use SplitQueriers to honour remote read endpoints marked as required.
// Additionally all returned queriers ensure that its Select's SeriesSets have ready data after first `Next` invoke.
// This is because Prometheus (fanout and secondary queries) can't handle the stream failing half way through by design.
func (s *Storage) Querier(mint, maxt int64) (storage.Querier, error) {
	required, bestEffort, err := s.SplitQueriers(mint, maxt)
	if err != nil {
		return nil, err
	}
	return storage.NewMergeQuerier(nil, append(required, bestEffort...), storage.ChainedSeriesMerge), nil
}

// SplitQueriers implements storage.RequiredQueriersStorage. It returns the
// queriers of remote read endpoints marked as required separately from the
// best effort ones.
func (s *Storage) SplitQueriers(mint, maxt int64) (required, bestEffort []storage.Querier, err error) {
	s.mtx.Lock()
	queryables, isRequired := s.queryables, s.required
	s.mtx.Unlock()

	for i, queryable := range queryables {
		q, qErr := queryable.Querier(mint, maxt)
		if qErr != nil {
			errs := []error{qErr}
			for _, open := range append(required, bestEffort...) {
				errs = append(errs, open.Close())
			}
			return nil, nil, errors.Join(errs...)
		}
		if isRequired[i] {
			required = append(required, q)
		} else {
			bestEffort = append(bestEffort, q)
		}
	}
	return required, bestEffort, nil
}

// CostSamplingQueryable reports an empty source only when no remote readers are configured.
func (s *Storage) CostSamplingQueryable() (storage.SampleAndChunkQueryable, bool) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return nil, len(s.queryables) == 0
}

// ChunkQuerier returns a storage.MergeQuerier combining the remote client queriers
// of each configured remote read endpoint.
func (s *Storage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	required, bestEffort, err := s.SplitChunkQueriers(mint, maxt)
	if err != nil {
		return nil, err
	}
	return storage.NewMergeChunkQuerier(nil, append(required, bestEffort...), storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge)), nil
}

// SplitChunkQueriers implements storage.RequiredQueriersStorage.
func (s *Storage) SplitChunkQueriers(mint, maxt int64) (required, bestEffort []storage.ChunkQuerier, err error) {
	s.mtx.Lock()
	queryables, isRequired := s.queryables, s.required
	s.mtx.Unlock()

	for i, queryable := range queryables {
		q, qErr := queryable.ChunkQuerier(mint, maxt)
		if qErr != nil {
			errs := []error{qErr}
			for _, open := range append(required, bestEffort...) {
				errs = append(errs, open.Close())
			}
			return nil, nil, errors.Join(errs...)
		}
		if isRequired[i] {
			required = append(required, q)
		} else {
			bestEffort = append(bestEffort, q)
		}
	}
	return required, bestEffort, nil
}

// Appender implements storage.Storage.
func (s *Storage) Appender(ctx context.Context) storage.Appender {
	return s.rws.Appender(ctx)
}

// AppenderV2 implements storage.Storage.
func (s *Storage) AppenderV2(ctx context.Context) storage.AppenderV2 {
	return s.rws.AppenderV2(ctx)
}

// LowestSentTimestamp returns the lowest sent timestamp across all queues.
func (s *Storage) LowestSentTimestamp() int64 {
	return s.rws.LowestSentTimestamp()
}

// Close the background processing of the storage queues.
func (s *Storage) Close() error {
	s.deduper.Stop()
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.rws.Close()
}

func labelsToEqualityMatchers(ls model.LabelSet) []*labels.Matcher {
	ms := make([]*labels.Matcher, 0, len(ls))
	for k, v := range ls {
		ms = append(ms, &labels.Matcher{
			Type:  labels.MatchEqual,
			Name:  string(k),
			Value: string(v),
		})
	}
	return ms
}

// Used for hashing configs and diff'ing hashes in ApplyConfig.
func toHash(data any) (string, error) {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(bytes)
	return hex.EncodeToString(hash[:]), nil
}
