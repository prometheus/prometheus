// Copyright 2017 The Prometheus Authors
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
// limitations under the License.package remote

package remote

import (
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
)

// Storage represents all the remote read and write endpoints.  It implements
// storage.Storage.
type Storage struct {
	logger log.Logger
	mtx    sync.RWMutex

	// For writes
	queues []*QueueManager

	// For reads
	clients        []*Client
	externalLabels model.LabelSet
}

func NewStorage(l log.Logger) *Storage {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &Storage{logger: l}
}

// ApplyConfig updates the state as the new config requires.
func (s *Storage) ApplyConfig(conf *config.Config) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// Update write queues

	newQueues := []*QueueManager{}
	// TODO: we should only stop & recreate queues which have changes,
	// as this can be quite disruptive.
	for i, rwConf := range conf.RemoteWriteConfigs {
		c, err := NewClient(i, &clientConfig{
			url:              rwConf.URL,
			timeout:          rwConf.RemoteTimeout,
			httpClientConfig: rwConf.HTTPClientConfig,
		})
		if err != nil {
			return err
		}
		newQueues = append(newQueues, NewQueueManager(
			s.logger,
			defaultQueueManagerConfig,
			conf.GlobalConfig.ExternalLabels,
			rwConf.WriteRelabelConfigs,
			c,
		))
	}

	for _, q := range s.queues {
		q.Stop()
	}

	s.queues = newQueues
	for _, q := range s.queues {
		q.Start()
	}

	// Update read clients

	clients := []*Client{}
	for i, rrConf := range conf.RemoteReadConfigs {
		c, err := NewClient(i, &clientConfig{
			url:              rrConf.URL,
			timeout:          rrConf.RemoteTimeout,
			httpClientConfig: rrConf.HTTPClientConfig,
		})
		if err != nil {
			return err
		}
		clients = append(clients, c)
	}

	s.clients = clients
	s.externalLabels = conf.GlobalConfig.ExternalLabels

	return nil
}

// Close the background processing of the storage queues.
func (s *Storage) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, q := range s.queues {
		q.Stop()
	}

	return nil
}
