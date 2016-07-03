// Copyright 2014 The Prometheus Authors
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

package local

import (
	"time"

	"github.com/prometheus/common/model"
)

// memorySeriesPreloader is a Preloader for the MemorySeriesStorage.
type memorySeriesPreloader struct {
	storage          *MemorySeriesStorage
	pinnedChunkDescs []*chunkDesc
}

// PreloadRange implements Preloader.
func (p *memorySeriesPreloader) PreloadRange(
	fp model.Fingerprint,
	from model.Time, through model.Time,
) SeriesIterator {
	cds, iter := p.storage.preloadChunksForRange(fp, from, through)
	p.pinnedChunkDescs = append(p.pinnedChunkDescs, cds...)
	return iter
}

// PreloadInstant implements Preloader
func (p *memorySeriesPreloader) PreloadInstant(
	fp model.Fingerprint,
	timestamp model.Time, stalenessDelta time.Duration,
) SeriesIterator {
	cds, iter := p.storage.preloadChunksForInstant(fp, timestamp.Add(-stalenessDelta), timestamp)
	p.pinnedChunkDescs = append(p.pinnedChunkDescs, cds...)
	return iter
}

// Close implements Preloader.
func (p *memorySeriesPreloader) Close() {
	for _, cd := range p.pinnedChunkDescs {
		cd.unpin(p.storage.evictRequests)
	}
	chunkOps.WithLabelValues(unpin).Add(float64(len(p.pinnedChunkDescs)))

}
