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
	"hash/crc32"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"

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
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(dir))
	}()

	_, err = writeMetaFile(log.NewNopLogger(), dir, &BlockMeta{})
	assert.NoError(t, err)

	meta, _, err := readMetaFile(dir)
	assert.NoError(t, err)
	assert.True(t, meta.Version != 2, "meta.json version must never be 2")
}

func TestSetCompactionFailed(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpdir))
	}()

	blockDir := createBlock(t, tmpdir, genSeries(1, 1, 0, 1))
	b, err := OpenBlock(nil, blockDir, nil)
	assert.NoError(t, err)
	assert.Equal(t, false, b.meta.Compaction.Failed)
	assert.NoError(t, b.setCompactionFailed())
	assert.Equal(t, true, b.meta.Compaction.Failed)
	assert.NoError(t, b.Close())

	b, err = OpenBlock(nil, blockDir, nil)
	assert.NoError(t, err)
	assert.Equal(t, true, b.meta.Compaction.Failed)
	assert.NoError(t, b.Close())
}

func TestCreateBlock(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpdir))
	}()
	b, err := OpenBlock(nil, createBlock(t, tmpdir, genSeries(1, 1, 0, 10)), nil)
	if err == nil {
		assert.NoError(t, b.Close())
	}
	assert.NoError(t, err)
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
				assert.NoError(t, f.Truncate(1))
			},
			openErr: errors.New("invalid segment header in segment 0: invalid size"),
		},
		{
			name: "invalid magic number",
			corrFunc: func(f *os.File) {
				magicChunksOffset := int64(0)
				_, err := f.Seek(magicChunksOffset, 0)
				assert.NoError(t, err)

				// Set invalid magic number.
				b := make([]byte, chunks.MagicChunksSize)
				binary.BigEndian.PutUint32(b[:chunks.MagicChunksSize], 0x00000000)
				n, err := f.Write(b)
				assert.NoError(t, err)
				assert.Equal(t, chunks.MagicChunksSize, n)
			},
			openErr: errors.New("invalid magic number 0"),
		},
		{
			name: "invalid chunk format version",
			corrFunc: func(f *os.File) {
				chunksFormatVersionOffset := int64(4)
				_, err := f.Seek(chunksFormatVersionOffset, 0)
				assert.NoError(t, err)

				// Set invalid chunk format version.
				b := make([]byte, chunks.ChunksFormatVersionSize)
				b[0] = 0
				n, err := f.Write(b)
				assert.NoError(t, err)
				assert.Equal(t, chunks.ChunksFormatVersionSize, n)
			},
			openErr: errors.New("invalid chunk format version 0"),
		},
		{
			name: "chunk not enough bytes to read the chunk length",
			corrFunc: func(f *os.File) {
				// Truncate one byte after the segment header.
				assert.NoError(t, f.Truncate(chunks.SegmentHeaderSize+1))
			},
			iterErr: errors.New("cannot populate chunk 8: segment doesn't include enough bytes to read the chunk size data field - required:13, available:9"),
		},
		{
			name: "chunk not enough bytes to read the data",
			corrFunc: func(f *os.File) {
				fi, err := f.Stat()
				assert.NoError(t, err)
				assert.NoError(t, f.Truncate(fi.Size()-1))
			},
			iterErr: errors.New("cannot populate chunk 8: segment doesn't include enough bytes to read the chunk - required:26, available:25"),
		},
		{
			name: "checksum mismatch",
			corrFunc: func(f *os.File) {
				fi, err := f.Stat()
				assert.NoError(t, err)

				// Get the chunk data end offset.
				chkEndOffset := int(fi.Size()) - crc32.Size

				// Seek to the last byte of chunk data and modify it.
				_, err = f.Seek(int64(chkEndOffset-1), 0)
				assert.NoError(t, err)
				n, err := f.Write([]byte("x"))
				assert.NoError(t, err)
				assert.Equal(t, n, 1)
			},
			iterErr: errors.New("cannot populate chunk 8: checksum mismatch expected:cfc0526c, actual:34815eae"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpdir, err := ioutil.TempDir("", "test_open_block_chunk_corrupted")
			assert.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(tmpdir))
			}()

			series := storage.NewListSeries(labels.FromStrings("a", "b"), []tsdbutil.Sample{sample{1, 1}})
			blockDir := createBlock(t, tmpdir, []storage.Series{series})
			files, err := sequenceFiles(chunkDir(blockDir))
			assert.NoError(t, err)
			assert.True(t, len(files) > 0, "No chunk created.")

			f, err := os.OpenFile(files[0], os.O_RDWR, 0666)
			assert.NoError(t, err)

			// Apply corruption function.
			tc.corrFunc(f)
			assert.NoError(t, f.Close())

			// Check open err.
			b, err := OpenBlock(nil, blockDir, nil)
			if tc.openErr != nil {
				assert.Equal(t, tc.openErr.Error(), err.Error())
				return
			}
			defer func() { assert.NoError(t, b.Close()) }()

			querier, err := NewBlockQuerier(b, 0, 1)
			assert.NoError(t, err)
			defer func() { assert.NoError(t, querier.Close()) }()
			set := querier.Select(false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

			// Check chunk errors during iter time.
			assert.True(t, set.Next(), "")
			it := set.At().Iterator()
			assert.Equal(t, false, it.Next())
			assert.Equal(t, tc.iterErr.Error(), it.Err().Error())
		})
	}
}

// TestBlockSize ensures that the block size is calculated correctly.
func TestBlockSize(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "test_blockSize")
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpdir))
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
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, blockInit.Close())
		}()
		expSizeInit = blockInit.Size()
		actSizeInit, err := fileutil.DirSize(blockInit.Dir())
		assert.NoError(t, err)
		assert.Equal(t, expSizeInit, actSizeInit)
	}

	// Delete some series and check the sizes again.
	{
		assert.NoError(t, blockInit.Delete(1, 10, labels.MustNewMatcher(labels.MatchRegexp, "", ".*")))
		expAfterDelete := blockInit.Size()
		assert.True(t, expAfterDelete > expSizeInit, "after a delete the block size should be bigger as the tombstone file should grow %v > %v", expAfterDelete, expSizeInit)
		actAfterDelete, err := fileutil.DirSize(blockDirInit)
		assert.NoError(t, err)
		assert.Equal(t, expAfterDelete, actAfterDelete, "after a delete reported block size doesn't match actual disk size")

		c, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{0}, nil)
		assert.NoError(t, err)
		blockDirAfterCompact, err := c.Compact(tmpdir, []string{blockInit.Dir()}, nil)
		assert.NoError(t, err)
		blockAfterCompact, err := OpenBlock(nil, filepath.Join(tmpdir, blockDirAfterCompact.String()), nil)
		assert.NoError(t, err)
		defer func() {
			assert.NoError(t, blockAfterCompact.Close())
		}()
		expAfterCompact := blockAfterCompact.Size()
		actAfterCompact, err := fileutil.DirSize(blockAfterCompact.Dir())
		assert.NoError(t, err)
		assert.True(t, actAfterDelete > actAfterCompact, "after a delete and compaction the block size should be smaller %v,%v", actAfterDelete, actAfterCompact)
		assert.Equal(t, expAfterCompact, actAfterCompact, "after a delete and compaction reported block size doesn't match actual disk size")
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
	assert.NoError(t, err)

	q, err := NewBlockQuerier(block, 0, 1000)
	assert.NoError(t, err)
	assert.Equal(t, query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar")),
		map[string][]tsdbutil.Sample{`{foo="bar"}`: {sample{t: 1, v: 2}}})

	q, err = NewBlockQuerier(block, 0, 1000)
	assert.NoError(t, err)
	assert.Equal(t, query(t, q, labels.MustNewMatcher(labels.MatchNotRegexp, "foo", "^.?$")),
		map[string][]tsdbutil.Sample{
			`{foo="bar"}`: {sample{t: 1, v: 2}},
			`{foo="baz"}`: {sample{t: 3, v: 4}},
		})
}

// createBlock creates a block with given set of series and returns its dir.
func createBlock(tb testing.TB, dir string, series []storage.Series) string {
	blockDir, err := CreateBlock(series, dir, 0, log.NewNopLogger())
	assert.NoError(tb, err)
	return blockDir
}

func createBlockFromHead(tb testing.TB, dir string, head *Head) string {
	compactor, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{1000000}, nil)
	assert.NoError(tb, err)

	assert.NoError(tb, os.MkdirAll(dir, 0777))

	// Add +1 millisecond to block maxt because block intervals are half-open: [b.MinTime, b.MaxTime).
	// Because of this block intervals are always +1 than the total samples it includes.
	ulid, err := compactor.Write(dir, head, head.MinTime(), head.MaxTime()+1, nil)
	assert.NoError(tb, err)
	return filepath.Join(dir, ulid.String())
}

func createHead(tb testing.TB, w *wal.WAL, series []storage.Series, chunkDir string) *Head {
	head, err := NewHead(nil, nil, w, DefaultBlockDuration, chunkDir, nil, DefaultStripeSize, nil)
	assert.NoError(tb, err)

	app := head.Appender(context.Background())
	for _, s := range series {
		ref := uint64(0)
		it := s.Iterator()
		for it.Next() {
			t, v := it.At()
			if ref != 0 {
				err := app.AddFast(ref, t, v)
				if err == nil {
					continue
				}
			}
			ref, err = app.Add(s.Labels(), t, v)
			assert.NoError(tb, err)
		}
		assert.NoError(tb, it.Err())
	}
	assert.NoError(tb, app.Commit())
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
