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
	"errors"

	"github.com/prometheus/prometheus/tsdb/record"
)

// MemoryWriteBatch contains committed data and the labels for every referenced
// series. WriteMemory copies data retained by the queues, so callers may reuse
// the batch, including histogram and exemplar data, after WriteMemory returns.
type MemoryWriteBatch struct {
	Series          []record.RefSeries
	Samples         []record.RefSample
	Histograms      []record.RefHistogramSample
	FloatHistograms []record.RefFloatHistogramSample
	Exemplars       []record.RefExemplar
}

// EnableMemoryWrite disables WAL readers and enables dropping new data when a
// destination's queues are full or resharding. Call it before applying config.
func (s *Storage) EnableMemoryWrite() error {
	s.rws.mtx.Lock()
	defer s.rws.mtx.Unlock()
	s.rws.memoryMtx.Lock()
	defer s.rws.memoryMtx.Unlock()
	if len(s.rws.queues) != 0 {
		return errors.New("memory write must be enabled before configuring remote write")
	}
	s.rws.memoryWrite = true
	return nil
}

// WriteMemory offers a committed batch to every destination without waiting for
// network I/O or queue space. Dropped data is counted per destination and type.
// Series must contain labels for all references in the batch, including refs
// used in previous batches. Callers must not mutate the batch during this call.
// Calls are serialized to preserve commit order across destinations.
func (s *Storage) WriteMemory(b MemoryWriteBatch) error {
	s.rws.memoryMtx.Lock()
	defer s.rws.memoryMtx.Unlock()
	if s.rws.memoryClosed {
		return errors.New("remote write storage is closed")
	}
	if !s.rws.memoryWrite {
		return errors.New("memory write is not enabled")
	}
	for _, q := range s.rws.memoryQueues {
		if q.memoryStopped {
			q.metrics.droppedSamplesTotal.WithLabelValues(reasonQueueUnavailable).Add(float64(len(b.Samples)))
			if q.sendNativeHistograms {
				q.metrics.droppedHistogramsTotal.WithLabelValues(reasonQueueUnavailable).Add(float64(len(b.Histograms) + len(b.FloatHistograms)))
			}
			if q.sendExemplars {
				q.metrics.droppedExemplarsTotal.WithLabelValues(reasonQueueUnavailable).Add(float64(len(b.Exemplars)))
			}
			continue
		}
		q.StoreSeries(b.Series, 0)
		q.Append(b.Samples)
		q.AppendHistograms(b.Histograms)
		q.AppendFloatHistograms(b.FloatHistograms)
		q.AppendExemplars(b.Exemplars)
		// Queued data owns its labels. Keeping only this batch's lookup state
		// avoids requiring WAL checkpoints or replay after configuration reloads.
		q.SeriesReset(1)
	}
	return nil
}

func (t *QueueManager) dropMemory(data timeSeries, reason string) {
	t.dataDropped.incr(1)
	switch data.sType {
	case tSample:
		t.metrics.droppedSamplesTotal.WithLabelValues(reason).Inc()
	case tExemplar:
		t.metrics.droppedExemplarsTotal.WithLabelValues(reason).Inc()
	case tHistogram, tFloatHistogram:
		t.metrics.droppedHistogramsTotal.WithLabelValues(reason).Inc()
	}
}
