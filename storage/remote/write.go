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
// limitations under the License.

package remote

import (
	"sync"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

// Writer allows queueing samples for remote writes.
type Writer struct {
	mtx    sync.RWMutex
	queues []*QueueManager
}

// ApplyConfig updates the state as the new config requires.
func (w *Writer) ApplyConfig(conf *config.Config) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

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
			defaultQueueManagerConfig,
			conf.GlobalConfig.ExternalLabels,
			rwConf.WriteRelabelConfigs,
			c,
		))
	}

	for _, q := range w.queues {
		q.Stop()
	}

	w.queues = newQueues
	for _, q := range w.queues {
		q.Start()
	}
	return nil
}

// Stop the background processing of the storage queues.
func (w *Writer) Stop() {
	for _, q := range w.queues {
		q.Stop()
	}
}

// Append implements storage.SampleAppender. Always returns nil.
func (w *Writer) Append(smpl *model.Sample) error {
	w.mtx.RLock()
	defer w.mtx.RUnlock()

	for _, q := range w.queues {
		q.Append(smpl)
	}
	return nil
}

// NeedsThrottling implements storage.SampleAppender. It will always return
// false as a remote storage drops samples on the floor if backlogging instead
// of asking for throttling.
func (w *Writer) NeedsThrottling() bool {
	return false
}
