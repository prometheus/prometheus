// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"net/http"
	"time"

	"github.com/prometheus/prometheus/storage/remote"
)

// QueueRetriever provides a list of remote write queues
type QueueRetriever interface {
	Queues() []*remote.QueueManager
}

// RemoteWritesDiscovery exposes all of the configured remote writes and their status
type RemoteWritesDiscovery struct {
	Queues []*RemoteWriteQueue `json:"queues"`
}

// RemoteWrite represents a single configured remote write and its status
type RemoteWriteQueue struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`

	ShardsMax     int `json:"shardsMax"`
	ShardsMin     int `json:"shardsMin"`
	ShardsCurrent int `json:"shardsCurrent"`

	ShardingCalculations remote.ShardingCalculations `json:"shardingCalculations"`
	IsResharding         bool                        `json:"isResharding"`

	Shards []*RemoteWriteShard `json:"shards"`
}

// RemoteWriteShard represents a single shard of a remote write queue and its state
type RemoteWriteShard struct {
	PendingSamples int `json:"pendingSamples"`

	LastError        string    `json:"lastError"`
	LastSentTime     time.Time `json:"lastSentTime"`
	LastSentDuration float64   `json:"lastSentDuration"`
}

// remoteWrite handles an API request for fetching all of the configured remote writes
func (api *API) remoteWrite(r *http.Request) apiFuncResult {
	queues := api.queueRetriever(r.Context()).Queues()
	res := RemoteWritesDiscovery{
		Queues: make([]*RemoteWriteQueue, 0, len(queues)),
	}

	var wrn []error

	for _, q := range queues {
		rRQ := &RemoteWriteQueue{}
		sC := q.StoreClient()

		rRQ.Name = sC.Name()
		rRQ.Endpoint = sC.Endpoint()

		cfg := q.Config()
		rRQ.ShardsMax = cfg.MaxShards
		rRQ.ShardsMin = cfg.MinShards
		rRQ.ShardsCurrent = q.CurrentShardNum()
		rRQ.IsResharding = q.IsResharding()
		rRQ.ShardingCalculations = q.ShardingCalculations()

		// for _, shard := ranges rRQ.shards {
		// 		TODO: fetch various shard data.
		rRQ.Shards = append(rRQ.Shards, &RemoteWriteShard{
			PendingSamples: 1337,

			LastError:        "example err",
			LastSentTime:     time.Now(),
			LastSentDuration: 100.123,
		})
		// }

		res.Queues = append(res.Queues, rRQ)
	}

	return apiFuncResult{res, nil, wrn, nil}
}
