// Copyright 2018 The Prometheus Authors
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

package tsdbutil

import (
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

type Sample interface {
	T() int64
	V() float64
}

func ChunkFromSamples(s []Sample) chunks.Meta {
	mint, maxt := int64(0), int64(0)

	if len(s) > 0 {
		mint, maxt = s[0].T(), s[len(s)-1].T()
	}

	c := chunkenc.NewXORChunk()
	ca, _ := c.Appender()

	for _, s := range s {
		ca.Append(s.T(), s.V())
	}
	return chunks.Meta{
		MinTime: mint,
		MaxTime: maxt,
		Chunk:   c,
	}
}

// PopulatedChunk creates a chunk populated with samples every second starting at minTime
func PopulatedChunk(numSamples int, minTime int64) chunks.Meta {
	samples := make([]Sample, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = sample{minTime + int64(i*1000), 1.0}
	}
	return ChunkFromSamples(samples)
}
