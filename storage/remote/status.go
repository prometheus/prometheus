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
	dto "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
)

// QueueStatus contains snapshot status information for a single remote write queue.
type QueueStatus struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`

	// Timestamps in seconds since epoch.
	HighestTimestamp     float64 `json:"highestTimestampSec"`
	HighestSentTimestamp float64 `json:"highestSentTimestampSec"`

	// Pending data awaiting send.
	PendingSamples    int `json:"pendingSamples"`
	PendingExemplars  int `json:"pendingExemplars"`
	PendingHistograms int `json:"pendingHistograms"`

	// Sharding.
	CurrentShards int `json:"currentShards"`
	DesiredShards int `json:"desiredShards"`
	MinShards     int `json:"minShards"`
	MaxShards     int `json:"maxShards"`
	ShardCapacity int `json:"shardCapacity"`

	// Sent totals.
	SamplesSent    int `json:"samplesSent"`
	ExemplarsSent  int `json:"exemplarsSent"`
	HistogramsSent int `json:"histogramsSent"`
	MetadataSent   int `json:"metadataSent"`
	BytesSent      int `json:"bytesSent"`

	// Failure totals.
	SamplesFailed    int `json:"samplesFailed"`
	ExemplarsFailed  int `json:"exemplarsFailed"`
	HistogramsFailed int `json:"histogramsFailed"`
	MetadataFailed   int `json:"metadataFailed"`

	// Retry totals.
	SamplesRetried    int `json:"samplesRetried"`
	ExemplarsRetried  int `json:"exemplarsRetried"`
	HistogramsRetried int `json:"histogramsRetried"`
	MetadataRetried   int `json:"metadataRetried"`
}

// RemoteWriteStatus contains status information for all remote write queues.
type RemoteWriteStatus struct {
	Queues []QueueStatus `json:"queues"`
}

// readGauge extracts the current value from a prometheus.Gauge.
func readGauge(g prometheus.Gauge) float64 {
	var m dto.Metric
	if err := g.Write(&m); err != nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

// readCounter extracts the current value from a prometheus.Counter.
func readCounter(c prometheus.Counter) float64 {
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

// Stats returns a snapshot of the queue's current status.
func (t *QueueManager) Stats() QueueStatus {
	return QueueStatus{
		Name:     t.storeClient.Name(),
		Endpoint: t.storeClient.Endpoint(),

		HighestTimestamp:     t.metrics.highestTimestamp.Get(),
		HighestSentTimestamp: t.metrics.highestSentTimestamp.Get(),

		PendingSamples:    int(readGauge(t.metrics.pendingSamples)),
		PendingExemplars:  int(readGauge(t.metrics.pendingExemplars)),
		PendingHistograms: int(readGauge(t.metrics.pendingHistograms)),

		CurrentShards: int(readGauge(t.metrics.numShards)),
		DesiredShards: int(readGauge(t.metrics.desiredNumShards)),
		MinShards:     int(readGauge(t.metrics.minNumShards)),
		MaxShards:     int(readGauge(t.metrics.maxNumShards)),
		ShardCapacity: int(readGauge(t.metrics.shardCapacity)),

		SamplesSent:    int(readCounter(t.metrics.samplesTotal)),
		ExemplarsSent:  int(readCounter(t.metrics.exemplarsTotal)),
		HistogramsSent: int(readCounter(t.metrics.histogramsTotal)),
		MetadataSent:   int(readCounter(t.metrics.metadataTotal)),
		BytesSent:      int(readCounter(t.metrics.sentBytesTotal)),

		SamplesFailed:    int(readCounter(t.metrics.failedSamplesTotal)),
		ExemplarsFailed:  int(readCounter(t.metrics.failedExemplarsTotal)),
		HistogramsFailed: int(readCounter(t.metrics.failedHistogramsTotal)),
		MetadataFailed:   int(readCounter(t.metrics.failedMetadataTotal)),

		SamplesRetried:    int(readCounter(t.metrics.retriedSamplesTotal)),
		ExemplarsRetried:  int(readCounter(t.metrics.retriedExemplarsTotal)),
		HistogramsRetried: int(readCounter(t.metrics.retriedHistogramsTotal)),
		MetadataRetried:   int(readCounter(t.metrics.retriedMetadataTotal)),
	}
}

// RemoteWriteStatus returns status information for all remote write queues.
func (rws *WriteStorage) RemoteWriteStatus() RemoteWriteStatus {
	rws.mtx.Lock()
	defer rws.mtx.Unlock()

	queues := make([]QueueStatus, 0, len(rws.queues))
	for _, q := range rws.queues {
		q.clientMtx.RLock()
		qs := q.Stats()
		q.clientMtx.RUnlock()
		queues = append(queues, qs)
	}

	return RemoteWriteStatus{Queues: queues}
}
