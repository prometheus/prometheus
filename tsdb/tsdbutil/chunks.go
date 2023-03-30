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
	"fmt"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

type Samples interface {
	Get(i int) Sample
	Len() int
}

type Sample interface {
	T() int64
	F() float64
	H() *histogram.Histogram
	FH() *histogram.FloatHistogram
	Type() chunkenc.ValueType
}

type SampleSlice []Sample

func (s SampleSlice) Get(i int) Sample { return s[i] }
func (s SampleSlice) Len() int         { return len(s) }

// ChunkFromSamples requires all samples to have the same type.
func ChunkFromSamples(s []Sample) chunks.Meta {
	return ChunkFromSamplesGeneric(SampleSlice(s))
}

// ChunkFromSamplesGeneric requires all samples to have the same type.
func ChunkFromSamplesGeneric(s Samples) chunks.Meta {
	mint, maxt := int64(0), int64(0)

	if s.Len() > 0 {
		mint, maxt = s.Get(0).T(), s.Get(s.Len()-1).T()
	}

	if s.Len() == 0 {
		return chunks.Meta{
			Chunk: chunkenc.NewXORChunk(),
		}
	}

	sampleType := s.Get(0).Type()
	c, err := chunkenc.NewEmptyChunk(sampleType.ChunkEncoding())
	if err != nil {
		panic(err) // TODO(codesome): dont panic.
	}

	ca, _ := c.Appender()

	for i := 0; i < s.Len(); i++ {
		switch sampleType {
		case chunkenc.ValFloat:
			ca.Append(s.Get(i).T(), s.Get(i).F())
		case chunkenc.ValHistogram:
			ca.AppendHistogram(s.Get(i).T(), s.Get(i).H())
		case chunkenc.ValFloatHistogram:
			ca.AppendFloatHistogram(s.Get(i).T(), s.Get(i).FH())
		default:
			panic(fmt.Sprintf("unknown sample type %s", sampleType.String()))
		}
	}
	return chunks.Meta{
		MinTime: mint,
		MaxTime: maxt,
		Chunk:   c,
	}
}

type sample struct {
	t  int64
	f  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

func (s sample) T() int64 {
	return s.t
}

func (s sample) F() float64 {
	return s.f
}

func (s sample) H() *histogram.Histogram {
	return s.h
}

func (s sample) FH() *histogram.FloatHistogram {
	return s.fh
}

func (s sample) Type() chunkenc.ValueType {
	switch {
	case s.h != nil:
		return chunkenc.ValHistogram
	case s.fh != nil:
		return chunkenc.ValFloatHistogram
	default:
		return chunkenc.ValFloat
	}
}

// PopulatedChunk creates a chunk populated with samples every second starting at minTime
func PopulatedChunk(numSamples int, minTime int64) chunks.Meta {
	samples := make([]Sample, numSamples)
	for i := 0; i < numSamples; i++ {
		samples[i] = sample{t: minTime + int64(i*1000), f: 1.0}
	}
	return ChunkFromSamples(samples)
}

// GenerateSamples starting at start and counting up numSamples.
func GenerateSamples(start, numSamples int) []Sample {
	return generateSamples(start, numSamples, func(i int) Sample {
		return sample{
			t: int64(i),
			f: float64(i),
		}
	})
}

func generateSamples(start, numSamples int, gen func(int) Sample) []Sample {
	samples := make([]Sample, 0, numSamples)
	for i := start; i < start+numSamples; i++ {
		samples = append(samples, gen(i))
	}
	return samples
}
