package storage

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/util/logging"
)

// RemoteRead represents all the remote read endpoints.
type RemoteRead struct {
	logger *logging.Deduper
	mtx    sync.Mutex

	// For reads.
	queryables             []storage.SampleAndChunkQueryable
	localStartTimeCallback func() (int64, error)
	noOpStorage
}

// NewRemoteRead returns a RemoteRead.
func NewRemoteRead(
	l log.Logger,
	reg prometheus.Registerer,
	stCallback func() (int64, error),
	walDir string,
	flushDeadline time.Duration,
) *RemoteRead {
	if l == nil {
		l = log.NewNopLogger()
	}
	logger := logging.Dedupe(l, 1*time.Minute)

	s := &RemoteRead{
		logger:                 logger,
		localStartTimeCallback: stCallback,
	}

	return s
}

// ApplyConfig updates the state as the new config requires.
func (s *RemoteRead) ApplyConfig(conf *config.Config) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// Update read clients
	readHashes := make(map[string]struct{})
	queryables := make([]storage.SampleAndChunkQueryable, 0, len(conf.RemoteReadConfigs))
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

		c, err := remote.NewReadClient(name, &remote.ClientConfig{
			URL:              rrConf.URL,
			Timeout:          rrConf.RemoteTimeout,
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
		queryables = append(queryables, remote.NewSampleAndChunkQueryableClient(
			c,
			externalLabels,
			labelsToEqualityMatchers(rrConf.RequiredMatchers),
			rrConf.ReadRecent,
			s.localStartTimeCallback,
		))
	}
	s.queryables = queryables

	return nil
}

// // StartTime implements the Storage interface.
// func (s *RemoteRead) StartTime() (int64, error) {
// 	return int64(model.Latest), nil
// }

// // Appender implements the Storage interface.
// func (s *RemoteRead) Appender(_ context.Context) storage.Appender {
// 	return noOpAppender{}
// }

// Querier returns a storage.MergeQuerier combining the remote client queriers
// of each configured remote read endpoint.
// Returned querier will never return error as all queryables are assumed best effort.
// Additionally all returned queriers ensure that its Select's SeriesSets have ready data after first `Next` invoke.
// This is because Prometheus (fanout and secondary queries) can't handle the stream failing half way through by design.
func (s *RemoteRead) Querier(mint, maxt int64) (storage.Querier, error) {
	s.mtx.Lock()
	queryables := s.queryables
	s.mtx.Unlock()

	queriers := make([]storage.Querier, 0, len(queryables))
	for _, queryable := range queryables {
		q, err := queryable.Querier(mint, maxt)
		if err != nil {
			return nil, err
		}
		queriers = append(queriers, q)
	}
	return storage.NewMergeQuerier(nil, queriers, storage.ChainedSeriesMerge), nil
}

// ChunkQuerier returns a storage.MergeQuerier combining the remote client queriers
// of each configured remote read endpoint.
func (s *RemoteRead) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	s.mtx.Lock()
	queryables := s.queryables
	s.mtx.Unlock()

	queriers := make([]storage.ChunkQuerier, 0, len(queryables))
	for _, queryable := range queryables {
		q, err := queryable.ChunkQuerier(mint, maxt)
		if err != nil {
			return nil, err
		}
		queriers = append(queriers, q)
	}
	return storage.NewMergeChunkQuerier(
		nil,
		queriers,
		storage.NewCompactingChunkSeriesMerger(storage.ChainedSeriesMerge),
	), nil
}

// Close the background processing of the storage queues.
func (s *RemoteRead) Close() error {
	s.logger.Stop()
	return nil
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
func toHash(data interface{}) (string, error) {
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(bytes)
	return hex.EncodeToString(hash[:]), nil
}
