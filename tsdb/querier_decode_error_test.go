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

package tsdb

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
)

func TestBlockSeriesIteratorStopsOnChunkDecodeError(t *testing.T) {
	results := runBlockSeriesIteratorDecodeErrorScenarios()
	require.NoError(t, results.setupErr)
	require.True(t, results.complete)
	requireIteratorObservation(t, results.beforeFirstRaw, nil, io.EOF)
	requireIteratorObservation(t, results.afterOneRaw, []iteratorSample{{timestamp: 0, value: 1}}, io.EOF)
	requireIteratorObservation(t, results.finalRaw, []iteratorSample{{timestamp: 100, value: 2}}, io.EOF)

	for _, tc := range []struct {
		name        string
		result      iteratorScenarioResult
		nextSamples []iteratorSample
		wantError   error
		seekSamples []iteratorSample
	}{
		{
			name:        "before first sample",
			result:      results.beforeFirst,
			nextSamples: nil,
			wantError:   io.EOF,
			seekSamples: nil,
		},
		{
			name:        "after one sample",
			result:      results.afterOne,
			nextSamples: []iteratorSample{{timestamp: 0, value: 1}},
			wantError:   io.EOF,
			seekSamples: []iteratorSample{{timestamp: 0, value: 1}},
		},
		{
			name:        "tombstone wrapped",
			result:      results.tombstoneWrapped,
			nextSamples: nil,
			wantError:   io.EOF,
			seekSamples: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireIteratorObservation(t, tc.result.next, tc.nextSamples, tc.wantError)
			require.Equal(t, chunkenc.ValNone, tc.result.seek.seekResult)
			requireIteratorObservation(t, tc.result.seek, tc.seekSamples, tc.wantError)
			requireIteratorObservation(t, tc.result.reused, []iteratorSample{{timestamp: 300, value: 4}}, nil)
		})
	}

	t.Run("clean transition", func(t *testing.T) {
		requireIteratorObservation(t, results.clean.next, []iteratorSample{{timestamp: 0, value: 1}, {timestamp: 100, value: 2}}, nil)
		requireIteratorObservation(t, results.clean.reused, []iteratorSample{{timestamp: 300, value: 4}}, nil)
	})

	t.Run("final chunk error", func(t *testing.T) {
		requireIteratorObservation(t, results.final.next, []iteratorSample{{timestamp: 0, value: 1}, {timestamp: 100, value: 2}}, io.EOF)
		requireIteratorObservation(t, results.final.reused, []iteratorSample{{timestamp: 300, value: 4}}, nil)
	})
}

func requireIteratorObservation(t *testing.T, actual iteratorObservation, expected []iteratorSample, wantError error) {
	t.Helper()
	require.Equal(t, expected, actual.samples)
	require.False(t, actual.unexpectedType)
	require.Equal(t, chunkenc.ValNone, actual.repeatedNext)
	if wantError == nil {
		require.NoError(t, actual.err)
		require.NoError(t, actual.repeatedError)
		return
	}
	require.ErrorIs(t, actual.err, wantError)
	require.ErrorIs(t, actual.repeatedError, wantError)
}

type iteratorSample struct {
	timestamp int64
	value     float64
}

type iteratorObservation struct {
	samples        []iteratorSample
	unexpectedType bool
	err            error
	seekResult     chunkenc.ValueType
	repeatedNext   chunkenc.ValueType
	repeatedError  error
}

type iteratorScenarioResult struct {
	next   iteratorObservation
	seek   iteratorObservation
	reused iteratorObservation
}

type iteratorDecodeErrorScenarios struct {
	setupErr         error
	complete         bool
	beforeFirstRaw   iteratorObservation
	afterOneRaw      iteratorObservation
	finalRaw         iteratorObservation
	beforeFirst      iteratorScenarioResult
	afterOne         iteratorScenarioResult
	tombstoneWrapped iteratorScenarioResult
	clean            iteratorScenarioResult
	final            iteratorScenarioResult
}

type iteratorSeriesInput struct {
	name      string
	chunks    []chunks.Meta
	intervals tombstones.Intervals
}

func observeIterator(it chunkenc.Iterator) iteratorObservation {
	var result iteratorObservation
	for valueType := it.Next(); valueType != chunkenc.ValNone; valueType = it.Next() {
		recordIteratorValue(it, valueType, &result)
	}
	result.err = it.Err()
	result.repeatedNext = it.Next()
	result.repeatedError = it.Err()
	return result
}

func observeIteratorSeek(it chunkenc.Iterator, prime bool) iteratorObservation {
	var result iteratorObservation
	if prime {
		if valueType := it.Next(); valueType != chunkenc.ValNone {
			recordIteratorValue(it, valueType, &result)
		}
	}
	result.seekResult = it.Seek(100)
	if result.seekResult != chunkenc.ValNone {
		recordIteratorValue(it, result.seekResult, &result)
	}
	result.err = it.Err()
	result.repeatedNext = it.Next()
	result.repeatedError = it.Err()
	return result
}

func recordIteratorValue(it chunkenc.Iterator, valueType chunkenc.ValueType, result *iteratorObservation) {
	if valueType != chunkenc.ValFloat {
		result.unexpectedType = true
		return
	}
	timestamp, value := it.At()
	result.samples = append(result.samples, iteratorSample{timestamp: timestamp, value: value})
}

func malformedXORChunkBeforeFirstSample() (chunkenc.Chunk, error) {
	return chunkenc.FromData(chunkenc.EncXOR, []byte{0, 1})
}

func malformedXORChunkAfterOneSample(timestamp int64, value float64) (chunkenc.Chunk, error) {
	chunk, err := newXORChunk(timestamp, value)
	if err != nil {
		return nil, err
	}
	data := append([]byte(nil), chunk.Bytes()...)
	binary.BigEndian.PutUint16(data[:2], 2)
	return chunkenc.FromData(chunkenc.EncXOR, data)
}

func newXORChunk(timestamp int64, value float64) (chunkenc.Chunk, error) {
	chunk := chunkenc.NewXORChunk()
	appender, err := chunk.Appender()
	if err != nil {
		return nil, err
	}
	appender.Append(0, timestamp, value)
	return chunk, nil
}

func newXORMeta(timestamp, minTime, maxTime int64, value float64) (chunks.Meta, error) {
	chunk, err := newXORChunk(timestamp, value)
	if err != nil {
		return chunks.Meta{}, err
	}
	return chunks.Meta{Chunk: chunk, MinTime: minTime, MaxTime: maxTime}, nil
}

func runBlockSeriesIteratorDecodeErrorScenarios() (results iteratorDecodeErrorScenarios) {
	beforeFirst, err := malformedXORChunkBeforeFirstSample()
	if err != nil {
		results.setupErr = err
		return results
	}
	afterOne, err := malformedXORChunkAfterOneSample(0, 1)
	if err != nil {
		results.setupErr = err
		return results
	}
	finalAfterOne, err := malformedXORChunkAfterOneSample(100, 2)
	if err != nil {
		results.setupErr = err
		return results
	}
	results.beforeFirstRaw = observeIterator(beforeFirst.Iterator(nil))
	results.afterOneRaw = observeIterator(afterOne.Iterator(nil))
	results.finalRaw = observeIterator(finalAfterOne.Iterator(nil))

	validAt0, err := newXORMeta(0, 0, 0, 1)
	if err != nil {
		results.setupErr = err
		return results
	}
	validAt100, err := newXORMeta(100, 100, 100, 2)
	if err != nil {
		results.setupErr = err
		return results
	}
	goodAt200, err := newXORMeta(200, 200, 201, 3)
	if err != nil {
		results.setupErr = err
		return results
	}
	goodAt300, err := newXORMeta(300, 300, 300, 4)
	if err != nil {
		results.setupErr = err
		return results
	}

	inputs := []iteratorSeriesInput{
		{name: "after", chunks: []chunks.Meta{{Chunk: afterOne, MinTime: 0, MaxTime: 1}, validAt100}},
		{name: "before", chunks: []chunks.Meta{{Chunk: beforeFirst, MinTime: 0, MaxTime: 1}, validAt100}},
		{name: "clean", chunks: []chunks.Meta{validAt0, validAt100}},
		{name: "final", chunks: []chunks.Meta{validAt0, {Chunk: finalAfterOne, MinTime: 100, MaxTime: 101}}},
		{name: "reuse", chunks: []chunks.Meta{goodAt200, goodAt300}, intervals: tombstones.Intervals{{Mint: 200, Maxt: 200}}},
		{name: "tombstone", chunks: []chunks.Meta{{Chunk: afterOne, MinTime: 0, MaxTime: 1}, validAt100}, intervals: tombstones.Intervals{{Mint: 0, Maxt: 0}}},
	}

	dir, err := os.MkdirTemp("", "prometheus-tsdb-iterator-error-")
	if err != nil {
		results.setupErr = err
		return results
	}
	defer func() { results.setupErr = errors.Join(results.setupErr, os.RemoveAll(dir)) }()

	blockReader, err := newIteratorErrorBlockReader(filepath.Join(dir, "block"), inputs)
	if err != nil {
		results.setupErr = err
		return results
	}
	querier, err := NewBlockQuerier(blockReader, math.MinInt64, math.MaxInt64)
	if err != nil {
		results.setupErr = errors.Join(err, blockReader.chunks.Close(), blockReader.tombstones.Close(), blockReader.index.Close())
		return results
	}
	defer func() { results.setupErr = errors.Join(results.setupErr, querier.Close()) }()

	set := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchRegexp, "a", ".+"))
	seriesByName := make(map[string]storage.Series, len(inputs))
	for set.Next() {
		series := set.At()
		seriesByName[series.Labels().Get("a")] = series
	}
	if err := set.Err(); err != nil {
		results.setupErr = err
		return results
	}
	if len(seriesByName) != len(inputs) {
		results.setupErr = fmt.Errorf("selected %d series, expected %d", len(seriesByName), len(inputs))
		return results
	}
	for _, input := range inputs {
		if seriesByName[input.name] == nil {
			results.setupErr = fmt.Errorf("selected series %q is missing", input.name)
			return results
		}
	}

	reuseSeries := seriesByName["reuse"]
	results.beforeFirst = observeSelectedSeries(seriesByName["before"], reuseSeries, true, false)
	results.afterOne = observeSelectedSeries(seriesByName["after"], reuseSeries, true, true)
	results.tombstoneWrapped = observeSelectedSeries(seriesByName["tombstone"], reuseSeries, true, false)
	results.clean = observeSelectedSeries(seriesByName["clean"], reuseSeries, false, false)
	results.final = observeSelectedSeries(seriesByName["final"], reuseSeries, false, false)
	results.complete = true
	return results
}

func observeSelectedSeries(series, reuseSeries storage.Series, seek, primeSeek bool) iteratorScenarioResult {
	iterator := series.Iterator(nil)
	result := iteratorScenarioResult{next: observeIterator(iterator)}
	if seek {
		result.seek = observeIteratorSeek(series.Iterator(nil), primeSeek)
	}
	result.reused = observeIterator(reuseSeries.Iterator(iterator))
	return result
}

type iteratorErrorBlockReader struct {
	index      IndexReader
	chunks     ChunkReader
	tombstones tombstones.Reader
}

func (b *iteratorErrorBlockReader) Index() (IndexReader, error)  { return b.index, nil }
func (b *iteratorErrorBlockReader) Chunks() (ChunkReader, error) { return b.chunks, nil }
func (b *iteratorErrorBlockReader) Tombstones() (tombstones.Reader, error) {
	return b.tombstones, nil
}
func (*iteratorErrorBlockReader) Meta() BlockMeta { return BlockMeta{} }
func (*iteratorErrorBlockReader) Size() int64     { return 0 }

func newIteratorErrorBlockReader(dir string, inputs []iteratorSeriesInput) (*iteratorErrorBlockReader, error) {
	var allChunks []chunks.Meta
	starts := make(map[string]int, len(inputs))
	for _, input := range inputs {
		starts[input.name] = len(allChunks)
		allChunks = append(allChunks, input.chunks...)
	}

	chunkWriter, err := chunks.NewWriter(filepath.Join(dir, "chunks"))
	if err != nil {
		return nil, err
	}
	if err := chunkWriter.WriteChunks(allChunks...); err != nil {
		return nil, errors.Join(err, chunkWriter.Close())
	}
	if err := chunkWriter.Close(); err != nil {
		return nil, err
	}

	ix := newMockIndex()
	stones := tombstones.NewMemTombstones()
	for i, input := range inputs {
		start := starts[input.name]
		metas := allChunks[start : start+len(input.chunks)]
		ref := storage.SeriesRef(i + 1)
		if err := ix.AddSeries(ref, labels.FromStrings("a", input.name), metas...); err != nil {
			return nil, err
		}
		if err := ix.WritePostings("a", input.name, index.NewListPostings([]storage.SeriesRef{ref})); err != nil {
			return nil, err
		}
		if len(input.intervals) > 0 {
			stones.AddInterval(ref, input.intervals...)
		}
	}

	chunkReader, err := chunks.NewDirReader(filepath.Join(dir, "chunks"), nil)
	if err != nil {
		return nil, err
	}
	return &iteratorErrorBlockReader{index: ix, chunks: chunkReader, tombstones: stones}, nil
}
