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

package tsdb

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wal"
)

// In Prometheus 2.1.0 we had a bug where the meta.json version was falsely bumped
// to 2. We had a migration in place resetting it to 1 but we should move immediately to
// version 3 next time to avoid confusion and issues.
func TestBlockMetaMustNeverBeVersion2(t *testing.T) {
	dir, err := ioutil.TempDir("", "metaversion")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(dir))
	}()

	_, err = writeMetaFile(log.NewNopLogger(), dir, &BlockMeta{})
	require.NoError(t, err)

	meta, _, err := readMetaFile(dir)
	require.NoError(t, err)
	require.NotEqual(t, 2, meta.Version, "meta.json version must never be 2")
}

func TestSetCompactionFailed(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()

	blockDir := createBlock(t, tmpdir, genSeries(1, 1, 0, 1))
	b, err := OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)
	require.Equal(t, false, b.meta.Compaction.Failed)
	require.NoError(t, b.setCompactionFailed())
	require.Equal(t, true, b.meta.Compaction.Failed)
	require.NoError(t, b.Close())

	b, err = OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)
	require.Equal(t, true, b.meta.Compaction.Failed)
	require.NoError(t, b.Close())
}

func TestCreateBlock(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()
	b, err := OpenBlock(nil, createBlock(t, tmpdir, genSeries(1, 1, 0, 10)), nil)
	if err == nil {
		require.NoError(t, b.Close())
	}
	require.NoError(t, err)
}

func TestCorruptedChunk(t *testing.T) {
	for _, tc := range []struct {
		name     string
		corrFunc func(f *os.File) // Func that applies the corruption.
		openErr  error
		iterErr  error
	}{
		{
			name: "invalid header size",
			corrFunc: func(f *os.File) {
				require.NoError(t, f.Truncate(1))
			},
			openErr: errors.New("invalid segment header in segment 0: invalid size"),
		},
		{
			name: "invalid magic number",
			corrFunc: func(f *os.File) {
				magicChunksOffset := int64(0)
				_, err := f.Seek(magicChunksOffset, 0)
				require.NoError(t, err)

				// Set invalid magic number.
				b := make([]byte, chunks.MagicChunksSize)
				binary.BigEndian.PutUint32(b[:chunks.MagicChunksSize], 0x00000000)
				n, err := f.Write(b)
				require.NoError(t, err)
				require.Equal(t, chunks.MagicChunksSize, n)
			},
			openErr: errors.New("invalid magic number 0"),
		},
		{
			name: "invalid chunk format version",
			corrFunc: func(f *os.File) {
				chunksFormatVersionOffset := int64(4)
				_, err := f.Seek(chunksFormatVersionOffset, 0)
				require.NoError(t, err)

				// Set invalid chunk format version.
				b := make([]byte, chunks.ChunksFormatVersionSize)
				b[0] = 0
				n, err := f.Write(b)
				require.NoError(t, err)
				require.Equal(t, chunks.ChunksFormatVersionSize, n)
			},
			openErr: errors.New("invalid chunk format version 0"),
		},
		{
			name: "chunk not enough bytes to read the chunk length",
			corrFunc: func(f *os.File) {
				// Truncate one byte after the segment header.
				require.NoError(t, f.Truncate(chunks.SegmentHeaderSize+1))
			},
			iterErr: errors.New("cannot populate chunk 8: segment doesn't include enough bytes to read the chunk size data field - required:13, available:9"),
		},
		{
			name: "chunk not enough bytes to read the data",
			corrFunc: func(f *os.File) {
				fi, err := f.Stat()
				require.NoError(t, err)
				require.NoError(t, f.Truncate(fi.Size()-1))
			},
			iterErr: errors.New("cannot populate chunk 8: segment doesn't include enough bytes to read the chunk - required:26, available:25"),
		},
		{
			name: "checksum mismatch",
			corrFunc: func(f *os.File) {
				fi, err := f.Stat()
				require.NoError(t, err)

				// Get the chunk data end offset.
				chkEndOffset := int(fi.Size()) - crc32.Size

				// Seek to the last byte of chunk data and modify it.
				_, err = f.Seek(int64(chkEndOffset-1), 0)
				require.NoError(t, err)
				n, err := f.Write([]byte("x"))
				require.NoError(t, err)
				require.Equal(t, n, 1)
			},
			iterErr: errors.New("cannot populate chunk 8: checksum mismatch expected:cfc0526c, actual:34815eae"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpdir, err := ioutil.TempDir("", "test_open_block_chunk_corrupted")
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.RemoveAll(tmpdir))
			}()

			series := storage.NewListSeries(labels.FromStrings("a", "b"), []tsdbutil.Sample{sample{1, 1}})
			blockDir := createBlock(t, tmpdir, []storage.Series{series})
			files, err := sequenceFiles(chunkDir(blockDir))
			require.NoError(t, err)
			require.Greater(t, len(files), 0, "No chunk created.")

			f, err := os.OpenFile(files[0], os.O_RDWR, 0666)
			require.NoError(t, err)

			// Apply corruption function.
			tc.corrFunc(f)
			require.NoError(t, f.Close())

			// Check open err.
			b, err := OpenBlock(nil, blockDir, nil)
			if tc.openErr != nil {
				require.Equal(t, tc.openErr.Error(), err.Error())
				return
			}
			defer func() { require.NoError(t, b.Close()) }()

			querier, err := NewBlockQuerier(b, 0, 1)
			require.NoError(t, err)
			defer func() { require.NoError(t, querier.Close()) }()
			set := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

			// Check chunk errors during iter time.
			require.True(t, set.Next())
			it := set.At().Iterator()
			require.Equal(t, false, it.Next())
			require.Equal(t, tc.iterErr.Error(), it.Err().Error())
		})
	}
}

func TestLabelValuesWithMatchers(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_block_label_values_with_matchers")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()

	var seriesEntries []storage.Series
	for i := 0; i < 100; i++ {
		seriesEntries = append(seriesEntries, storage.NewListSeries(labels.Labels{
			{Name: "unique", Value: fmt.Sprintf("value%d", i)},
			{Name: "tens", Value: fmt.Sprintf("value%d", i/10)},
		}, []tsdbutil.Sample{sample{100, 0}}))
	}

	blockDir := createBlock(t, tmpdir, seriesEntries)
	files, err := sequenceFiles(chunkDir(blockDir))
	require.NoError(t, err)
	require.Greater(t, len(files), 0, "No chunk created.")

	// Check open err.
	block, err := OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, block.Close()) }()

	indexReader, err := block.Index()
	require.NoError(t, err)
	defer func() { require.NoError(t, indexReader.Close()) }()

	testCases := []struct {
		name           string
		labelName      string
		matchers       []*labels.Matcher
		expectedValues []string
	}{
		{
			name:           "get tens based on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "unique", "value35")},
			expectedValues: []string{"value3"},
		}, {
			name:           "get unique ids based on a ten",
			labelName:      "unique",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "tens", "value1")},
			expectedValues: []string{"value10", "value11", "value12", "value13", "value14", "value15", "value16", "value17", "value18", "value19"},
		}, {
			name:           "get tens by pattern matching on unique id",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "unique", "value[5-7]5")},
			expectedValues: []string{"value5", "value6", "value7"},
		}, {
			name:           "get tens by matching for absence of unique label",
			labelName:      "tens",
			matchers:       []*labels.Matcher{labels.MustNewMatcher(labels.MatchNotEqual, "unique", "")},
			expectedValues: []string{"value0", "value1", "value2", "value3", "value4", "value5", "value6", "value7", "value8", "value9"},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			actualValues, err := indexReader.SortedLabelValues(tt.labelName, tt.matchers...)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)

			actualValues, err = indexReader.LabelValues(tt.labelName, tt.matchers...)
			sort.Strings(actualValues)
			require.NoError(t, err)
			require.Equal(t, tt.expectedValues, actualValues)
		})
	}
}

// TestBlockSize ensures that the block size is calculated correctly.
func TestBlockSize(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_blockSize")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.RemoveAll(tmpdir))
	}()

	var (
		blockInit    *Block
		expSizeInit  int64
		blockDirInit string
	)

	// Create a block and compare the reported size vs actual disk size.
	{
		blockDirInit = createBlock(t, tmpdir, genSeries(10, 1, 1, 100))
		blockInit, err = OpenBlock(nil, blockDirInit, nil)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, blockInit.Close())
		}()
		expSizeInit = blockInit.Size()
		actSizeInit, err := fileutil.DirSize(blockInit.Dir())
		require.NoError(t, err)
		require.Equal(t, expSizeInit, actSizeInit)
	}

	// Delete some series and check the sizes again.
	{
		require.NoError(t, blockInit.Delete(1, 10, labels.MustNewMatcher(labels.MatchRegexp, "", ".*")))
		expAfterDelete := blockInit.Size()
		require.Greater(t, expAfterDelete, expSizeInit, "after a delete the block size should be bigger as the tombstone file should grow %v > %v", expAfterDelete, expSizeInit)
		actAfterDelete, err := fileutil.DirSize(blockDirInit)
		require.NoError(t, err)
		require.Equal(t, expAfterDelete, actAfterDelete, "after a delete reported block size doesn't match actual disk size")

		c, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{0}, nil, nil)
		require.NoError(t, err)
		blockDirAfterCompact, err := c.Compact(tmpdir, []string{blockInit.Dir()}, nil)
		require.NoError(t, err)
		blockAfterCompact, err := OpenBlock(nil, filepath.Join(tmpdir, blockDirAfterCompact.String()), nil)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, blockAfterCompact.Close())
		}()
		expAfterCompact := blockAfterCompact.Size()
		actAfterCompact, err := fileutil.DirSize(blockAfterCompact.Dir())
		require.NoError(t, err)
		require.Greater(t, actAfterDelete, actAfterCompact, "after a delete and compaction the block size should be smaller %v,%v", actAfterDelete, actAfterCompact)
		require.Equal(t, expAfterCompact, actAfterCompact, "after a delete and compaction reported block size doesn't match actual disk size")
	}
}

func TestReadIndexFormatV1(t *testing.T) {
	/* The block here was produced at the commit
	    706602daed1487f7849990678b4ece4599745905 used in 2.0.0 with:
	   db, _ := Open("v1db", nil, nil, nil)
	   app := db.Appender()
	   app.Add(labels.FromStrings("foo", "bar"), 1, 2)
	   app.Add(labels.FromStrings("foo", "baz"), 3, 4)
	   app.Add(labels.FromStrings("foo", "meh"), 1000*3600*4, 4) // Not in the block.
	   // Make sure we've enough values for the lack of sorting of postings offsets to show up.
	   for i := 0; i < 100; i++ {
	     app.Add(labels.FromStrings("bar", strconv.FormatInt(int64(i), 10)), 0, 0)
	   }
	   app.Commit()
	   db.compact()
	   db.Close()
	*/

	blockDir := filepath.Join("testdata", "index_format_v1")
	block, err := OpenBlock(nil, blockDir, nil)
	require.NoError(t, err)

	q, err := NewBlockQuerier(block, 0, 1000)
	require.NoError(t, err)
	require.Equal(t, query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")),
		map[string][]tsdbutil.Sample{`{foo="bar"}`: {sample{t: 1, v: 2}}})

	q, err = NewBlockQuerier(block, 0, 1000)
	require.NoError(t, err)
	require.Equal(t, query(t, q, labels.MustNewMatcher(labels.MatchNotRegexp, "foo", "^.?$")),
		map[string][]tsdbutil.Sample{
			`{foo="bar"}`: {sample{t: 1, v: 2}},
			`{foo="baz"}`: {sample{t: 3, v: 4}},
		})
}

func BenchmarkLabelValuesWithMatchers(b *testing.B) {
	tmpdir, err := ioutil.TempDir("", "bench_block_label_values_with_matchers")
	require.NoError(b, err)
	defer func() {
		require.NoError(b, os.RemoveAll(tmpdir))
	}()

	var seriesEntries []storage.Series
	metricCount := 1000000
	for i := 0; i < metricCount; i++ {
		seriesEntries = append(seriesEntries, storage.NewListSeries(labels.Labels{
			{Name: "unique", Value: fmt.Sprintf("value%d", i)},
			{Name: "tens", Value: fmt.Sprintf("value%d", i/(metricCount/10))},
			{Name: "ninety", Value: fmt.Sprintf("value%d", i/(metricCount/10)/9)}, // "0" for the first 90%, then "1"
		}, []tsdbutil.Sample{sample{100, 0}}))
	}

	blockDir := createBlock(b, tmpdir, seriesEntries)
	files, err := sequenceFiles(chunkDir(blockDir))
	require.NoError(b, err)
	require.Greater(b, len(files), 0, "No chunk created.")

	// Check open err.
	block, err := OpenBlock(nil, blockDir, nil)
	require.NoError(b, err)
	defer func() { require.NoError(b, block.Close()) }()

	indexReader, err := block.Index()
	require.NoError(b, err)
	defer func() { require.NoError(b, indexReader.Close()) }()

	matchers := []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "ninety", "value0")}

	b.ResetTimer()
	b.ReportAllocs()

	for benchIdx := 0; benchIdx < b.N; benchIdx++ {
		actualValues, err := indexReader.LabelValues("tens", matchers...)
		require.NoError(b, err)
		require.Equal(b, 9, len(actualValues))
	}
}

// createBlock creates a block with given set of series and returns its dir.
func createBlock(tb testing.TB, dir string, series []storage.Series) string {
	blockDir, err := CreateBlock(series, dir, 0, log.NewNopLogger())
	require.NoError(tb, err)
	return blockDir
}

func createBlockFromHead(tb testing.TB, dir string, head *Head) string {
	compactor, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{1000000}, nil, nil)
	require.NoError(tb, err)

	require.NoError(tb, os.MkdirAll(dir, 0777))

	// Add +1 millisecond to block maxt because block intervals are half-open: [b.MinTime, b.MaxTime).
	// Because of this block intervals are always +1 than the total samples it includes.
	ulid, err := compactor.Write(dir, head, head.MinTime(), head.MaxTime()+1, nil)
	require.NoError(tb, err)
	return filepath.Join(dir, ulid.String())
}

func createHead(tb testing.TB, w *wal.WAL, series []storage.Series, chunkDir string) *Head {
	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = chunkDir
	head, err := NewHead(nil, nil, w, opts)
	require.NoError(tb, err)

	app := head.Appender(context.Background())
	for _, s := range series {
		ref := uint64(0)
		it := s.Iterator()
		lset := s.Labels()
		for it.Next() {
			t, v := it.At()
			ref, err = app.Append(ref, lset, t, v)
			require.NoError(tb, err)
		}
		require.NoError(tb, it.Err())
	}
	require.NoError(tb, app.Commit())
	return head
}

const (
	defaultLabelName  = "labelName"
	defaultLabelValue = "labelValue"
)

// genSeries generates series with a given number of labels and values.
func genSeries(totalSeries, labelCount int, mint, maxt int64) []storage.Series {
	if totalSeries == 0 || labelCount == 0 {
		return nil
	}

	series := make([]storage.Series, totalSeries)

	for i := 0; i < totalSeries; i++ {
		lbls := make(map[string]string, labelCount)
		lbls[defaultLabelName] = strconv.Itoa(i)
		for j := 1; len(lbls) < labelCount; j++ {
			lbls[defaultLabelName+strconv.Itoa(j)] = defaultLabelValue + strconv.Itoa(j)
		}
		samples := make([]tsdbutil.Sample, 0, maxt-mint+1)
		for t := mint; t < maxt; t++ {
			samples = append(samples, sample{t: t, v: rand.Float64()})
		}
		series[i] = storage.NewListSeries(labels.FromMap(lbls), samples)
	}
	return series
}

// populateSeries generates series from given labels, mint and maxt.
func populateSeries(lbls []map[string]string, mint, maxt int64) []storage.Series {
	if len(lbls) == 0 {
		return nil
	}

	series := make([]storage.Series, 0, len(lbls))
	for _, lbl := range lbls {
		if len(lbl) == 0 {
			continue
		}
		samples := make([]tsdbutil.Sample, 0, maxt-mint+1)
		for t := mint; t <= maxt; t++ {
			samples = append(samples, sample{t: t, v: rand.Float64()})
		}
		series = append(series, storage.NewListSeries(labels.FromMap(lbl), samples))
	}
	return series
}
