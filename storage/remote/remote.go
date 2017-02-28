<<<<<<< HEAD
// Copyright 2016 The Prometheus Authors
=======
// Copyright 2017 The Prometheus Authors
>>>>>>> master
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
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
<<<<<<< HEAD
	"github.com/prometheus/prometheus/relabel"
=======
>>>>>>> master
)

// Storage allows queueing samples for remote writes.
type Storage struct {
<<<<<<< HEAD
	mtx            sync.RWMutex
	externalLabels model.LabelSet
	conf           config.RemoteWriteConfig

	queue *StorageQueueManager
}

// New returns a new remote Storage.
func New() *Storage {
	return &Storage{}
=======
	mtx    sync.RWMutex
	queues []*QueueManager
>>>>>>> master
}

// ApplyConfig updates the state as the new config requires.
func (s *Storage) ApplyConfig(conf *config.Config) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

<<<<<<< HEAD
	// TODO: we should only stop & recreate queues which have changes,
	// as this can be quite disruptive.
	var newQueue *StorageQueueManager

	if conf.RemoteWriteConfig.URL != nil {
		c, err := NewClient(conf.RemoteWriteConfig)
		if err != nil {
			return err
		}
		newQueue = NewStorageQueueManager(c, nil)
	}

	if s.queue != nil {
		s.queue.Stop()
	}
	s.queue = newQueue
	s.conf = conf.RemoteWriteConfig
	s.externalLabels = conf.GlobalConfig.ExternalLabels
	if s.queue != nil {
		s.queue.Start()
=======
	newQueues := []*QueueManager{}
	// TODO: we should only stop & recreate queues which have changes,
	// as this can be quite disruptive.
	for i, rwConf := range conf.RemoteWriteConfigs {
		c, err := NewClient(i, rwConf)
		if err != nil {
			return err
		}
		newQueues = append(newQueues, NewQueueManager(QueueManagerConfig{
			Client:         c,
			ExternalLabels: conf.GlobalConfig.ExternalLabels,
			RelabelConfigs: rwConf.WriteRelabelConfigs,
		}))
	}

	for _, q := range s.queues {
		q.Stop()
	}

	s.queues = newQueues
	for _, q := range s.queues {
		q.Start()
>>>>>>> master
	}
	return nil
}

// Stop the background processing of the storage queues.
func (s *Storage) Stop() {
	if s.queue != nil {
		s.queue.Stop()
	}
}

// Append implements storage.SampleAppender. Always returns nil.
func (s *Storage) Append(smpl *model.Sample) error {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

<<<<<<< HEAD
	if s.queue == nil {
		return nil
	}

	var snew model.Sample
	snew = *smpl
	snew.Metric = smpl.Metric.Clone()

	for ln, lv := range s.externalLabels {
		if _, ok := smpl.Metric[ln]; !ok {
			snew.Metric[ln] = lv
		}
	}
	snew.Metric = model.Metric(
		relabel.Process(model.LabelSet(snew.Metric), s.conf.WriteRelabelConfigs...))

	if snew.Metric == nil {
		return nil
	}
	s.queue.Append(&snew)
=======
	for _, q := range s.queues {
		q.Append(smpl)
	}
>>>>>>> master
	return nil
}

// NeedsThrottling implements storage.SampleAppender. It will always return
// false as a remote storage drops samples on the floor if backlogging instead
// of asking for throttling.
func (s *Storage) NeedsThrottling() bool {
	return false
}
