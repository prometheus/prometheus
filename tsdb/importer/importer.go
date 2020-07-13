// Copyright 2020 The Prometheus Authors
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

package importer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

type Parser interface {
	// IsParsable returns true if given parser can be used for given input.
	// It's up to parser how much bytes it will fetch from reader to detect format.
	IsParsable(io.Reader) error
	// Parse parses the input.
	Parse(io.Reader) error
}

var (
	// This error is thrown when the current entry does not have a timestamp associated.
	NoTimestampError = errors.New("expected timestamp with metric")
	// This error is thrown when the sample being parsed currently has a corresponding block
	// in the database, but has no metadata collected for it, curiously enough.
	NoBlockMetaFoundError = errors.New("no metadata found for current samples' block")
)

type blockTimestampPair struct {
	start, end int64
}

type newBlockMeta struct {
	index      int
	count      int
	mint, maxt int64
	isAligned  bool
	dir        string
	children   []*newBlockMeta
}

// Content Type for the Open Metrics Parser.
// Needed to init the text parser.
const contentType = "application/openmetrics-text;"

// ImportFromFile imports data from a file formatted according to the Open Metrics format,
// converts it into block(s), and places the newly created block(s) in the
// TSDB DB directory, where it is treated like any other block.
func ImportFromFile(
	r io.Reader,
	outputPath string,
	maxSamplesInMemory int,
	maxBlockChildren int,
) error {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	level.Debug(logger).Log("msg", "creating temp directory")
	tmpDbDir, err := ioutil.TempDir("", "importer")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDbDir)

	level.Debug(logger).Log("msg", "reading existing block metadata to infer time ranges")
	blockMetas, err := constructBlockMetadata(dbPath)
	if err != nil {
		return err
	}

	level.Info(logger).Log("msg", "importing input data and aligning with existing DB")
	if err = writeSamples(r, tmpDbDir, blockMetas, maxSamplesInMemory, maxBlockChildren, logger); err != nil {
		return err
	}

	level.Info(logger).Log("msg", "merging fully overlapping blocks")
	if err = mergeBlocks(tmpDbDir, blockMetas, logger); err != nil {
		return err
	}

	level.Debug(logger).Log("msg", "copying newly created blocks from temp location to actual DB location")
	if err = fileutil.CopyDirs(tmpDbDir, dbPath); err != nil {
		return err
	}

	return nil
}

// writeSamples parses each metric sample, and writes the samples to the correspondingly aligned block
// to the given directory.
func writeSamples(
	r io.Reader,
	dir string,
	metas []*newBlockMeta,
	maxSamplesInMemory int,
	maxBlockChildren int,
	logger log.Logger,
) error {
	// First block represents everything before DB start.
	// Last block represents everything after DB end.
	dbMint, dbMaxt := metas[0].maxt, metas[len(metas)-1].mint

	blocks := make([][]*tsdb.MetricSample, len(metas))
	currentPassCount := 0

	// TODO: Maybe use bufio.Reader.Readline instead?
	// https://stackoverflow.com/questions/21124327/how-to-read-a-text-file-line-by-line-in-go-when-some-lines-are-long-enough-to-ca
	// Use a streaming approach to avoid loading too much data at once.
	scanner := bufio.NewScanner(r)
	buf := new(bytes.Buffer)
	scanner.Split(sampleStreamer(buf))

	for scanner.Scan() {
		if currentPassCount == 0 {
			blocks = make([][]*tsdb.MetricSample, len(metas))
		}

		encSample := scanner.Bytes()
		decBuf := bytes.NewBuffer(encSample)
		sample := tsdb.MetricSample{}
		if err := gob.NewDecoder(decBuf).Decode(&sample); err != nil {
			level.Error(logger).Log("msg", "failed to decode current entry returned by file scanner", "err", err)
			return err
		}

		// Find what block this sample belongs to.
		var blockIndex int
		if len(metas) == 1 || sample.Timestamp < dbMint {
			blockIndex = 0
		} else if sample.Timestamp >= dbMaxt {
			blockIndex = len(metas) - 1
		} else {
			blockIndex = getBlockIndex(sample.Timestamp, metas)
			if blockIndex == -1 {
				return NoBlockMetaFoundError
			}
		}

		meta := metas[blockIndex]
		meta.mint = value.MinInt64(meta.mint, sample.Timestamp)
		meta.maxt = value.MaxInt64(meta.maxt, sample.Timestamp)
		meta.count++

		blocks[blockIndex] = append(blocks[blockIndex], &sample)

		currentPassCount++
		// Have enough samples to write to disk.
		if currentPassCount == maxSamplesInMemory {
			if err := flushBlocks(dir, blocks, metas, maxBlockChildren, logger); err != nil {
				return err
			}
			// Reset current pass count.
			currentPassCount = 0
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Flush any remaining samples.
	if err := flushBlocks(dir, blocks, metas, maxBlockChildren, logger); err != nil {
		return err
	}

	return nil
}

// flushBlocks writes the given blocks of samples to disk, and updates block metadata information.
func flushBlocks(
	dir string,
	toFlush [][]*tsdb.MetricSample,
	metas []*newBlockMeta,
	maxBlockChildren int,
	logger log.Logger,
) error {
	for blockIdx, block := range toFlush {
		// If current block is empty, nothing to write.
		if len(block) == 0 {
			continue
		}
		meta := metas[blockIdx]

		// Put each sample into the appropriate block.
		bins, binTimes := binSamples(block, meta, tsdb.DefaultBlockDuration)
		for binIdx, bin := range bins {
			if len(bin) == 0 {
				continue
			}
			start, end := binTimes[binIdx].start, binTimes[binIdx].end
			path, err := tsdb.CreateBlock(bin, dir, start, end, logger)
			if err != nil {
				return err
			}
			child := &newBlockMeta{
				index:     meta.index,
				count:     0,
				mint:      start,
				maxt:      end,
				isAligned: meta.isAligned,
				dir:       path,
			}
			meta.children = append(meta.children, child)
		}
		if maxBlockChildren > 0 && len(meta.children) >= maxBlockChildren {
			if err := compactMeta(dir, meta, logger); err != nil {
				return err
			}
		}
	}
	return nil
}

// sampleStreamer returns a function that can be used to parse an OpenMetrics compliant
// byte array, and return each token in the user-provided byte buffer.
func sampleStreamer(buf *bytes.Buffer) func([]byte, bool) (int, []byte, error) {
	currentEntryInvalid := false
	return func(data []byte, atEOF bool) (int, []byte, error) {
		var err error
		advance := 0
		lineIndex := 0
		lines := strings.Split(string(data), "\n")
		parser := textparse.New(data, contentType)
		for {
			var et textparse.Entry
			if et, err = parser.Next(); err != nil {
				if !atEOF && et == textparse.EntryInvalid && !currentEntryInvalid {
					currentEntryInvalid = true
					// Fetch more data for the scanner to see if the entry is really invalid.
					return 0, nil, nil
				}
				if err == io.EOF {
					err = nil
				}
				return 0, nil, err
			}

			// Reset invalid entry flag as it meant that we just had to get more data
			// into the scanner.
			if currentEntryInvalid {
				currentEntryInvalid = false
			}

			// Add 1 to account for newline character.
			lineLength := len(lines[lineIndex]) + 1
			lineIndex++

			if et != textparse.EntrySeries {
				advance = advance + lineLength
				continue
			}

			_, ctime, cvalue := parser.Series()
			if ctime == nil {
				return 0, nil, NoTimestampError
			}
			// OpenMetrics parser multiples times by 1000 - undoing that.
			ctimeCorrected := *ctime / 1000

			var clabels labels.Labels
			_ = parser.Metric(&clabels)

			sample := tsdb.MetricSample{
				Timestamp: ctimeCorrected,
				Value:     cvalue,
				Labels:    labels.FromMap(clabels.Map()),
			}

			buf.Reset()
			enc := gob.NewEncoder(buf)
			if err = enc.Encode(sample); err != nil {
				return 0, nil, err
			}

			advance += lineLength
			return advance, buf.Bytes(), nil
		}
	}
}

// makeRange returns a series of block times between start and stop,
// with step divisions.
func makeRange(start, stop, step int64) []blockTimestampPair {
	if step <= 0 || stop < start {
		return []blockTimestampPair{}
	}
	r := make([]blockTimestampPair, 0)
	// In case we only have samples with the same timestamp.
	if start == stop {
		r = append(r, blockTimestampPair{
			start: start,
			end:   stop + 1,
		})
		return r
	}
	for s := start; s < stop; s += step {
		pair := blockTimestampPair{
			start: s,
		}
		if (s + step) >= stop {
			pair.end = stop + 1
		} else {
			pair.end = s + step
		}
		r = append(r, pair)
	}
	return r
}

// mergeBlocks looks for blocks that have overlapping time intervals, and compacts them.
func mergeBlocks(dest string, metas []*newBlockMeta, logger log.Logger) error {
	for _, m := range metas {
		// If no children, there's nothing to merge.
		if len(m.children) == 0 {
			continue
		}
		if err := compactMeta(dest, m, logger); err != nil {
			return err
		}
	}
	return nil
}

// binBlocks groups blocks that have fully intersecting intervals of the given durations.
// It iterates through the given durations, creating progressive blocks, and once it reaches
// the last given duration, it divvies all the remaining samples into blocks of that duration.
// Input blocks are assumed sorted by time.
func binBlocks(blocks []*newBlockMeta, durations []int64) [][]*newBlockMeta {
	bins := make([][]*newBlockMeta, 0)
	if len(blocks) == 0 || len(durations) == 0 {
		return bins
	}

	bin := make([]*newBlockMeta, 0)

	start := blocks[0].mint
	blockIdx := 0
Outer:
	for dIdx, duration := range durations {
		for bIdx, block := range blocks[blockIdx:] {
			blockIdx = bIdx
			if block.maxt-block.mint >= duration {
				if block.mint <= start {
					bin = append(bin, block)
					bins = append(bins, bin)
				} else {
					bins = append(bins, bin)
					bins = append(bins, []*newBlockMeta{block})
				}
				bin = []*newBlockMeta{}
				start = block.maxt
				if dIdx == len(durations)-1 {
					continue
				} else {
					blockIdx++
					continue Outer
				}
			}
			if block.mint-start < duration && block.maxt-start < duration {
				// If current block is within the duration window, place it in the current bin.
				bin = append(bin, block)
				blockIdx++
			} else {
				// Else create a new block, starting with this block.
				bins = append(bins, bin)
				bin = []*newBlockMeta{block}
				start = block.mint
				if dIdx < len(durations)-1 {
					blockIdx++
					continue Outer
				}
			}
		}
	}
	bins = append(bins, bin)
	return bins
}

// binSamples divvies the samples into bins corresponding to the given block.
// If an aligned block is given to it, then a single bin is created,
// else, it divides the samples into blocks of duration.
// It returns the binned samples, and the time limits for each bin.
func binSamples(samples []*tsdb.MetricSample, meta *newBlockMeta, duration int64) ([][]*tsdb.MetricSample, []blockTimestampPair) {
	findBlock := func(ts int64, blocks []blockTimestampPair) int {
		for idx, block := range blocks {
			if block.start <= ts && ts < block.end {
				return idx
			}
		}
		return -1
	}
	bins := make([][]*tsdb.MetricSample, 0)
	times := make([]blockTimestampPair, 0)
	if len(samples) == 0 {
		return bins, times
	}
	if meta.isAligned {
		bins = append(bins, samples)
		times = []blockTimestampPair{{start: meta.mint, end: meta.maxt}}
	} else {
		timeTranches := makeRange(meta.mint, meta.maxt, duration)
		bins = make([][]*tsdb.MetricSample, len(timeTranches))
		binIdx := -1
		for _, sample := range samples {
			ts := sample.Timestamp
			if binIdx == -1 {
				binIdx = findBlock(ts, timeTranches)
			}
			block := timeTranches[binIdx]
			if block.start <= ts && ts < block.end {
				bins[binIdx] = append(bins[binIdx], sample)
			} else {
				binIdx = findBlock(ts, timeTranches)
				bins[binIdx] = append(bins[binIdx], sample)
			}
		}
		times = timeTranches
	}
	return bins, times
}

// constructBlockMetadata gives us the new blocks' metadatas.
func constructBlockMetadata(path string) ([]*newBlockMeta, error) {
	dbMint, dbMaxt := int64(math.MinInt64), int64(math.MaxInt64)

	// If we try to open a regular RW handle on an active TSDB instance,
	// it will fail. Hence, we open a RO handle.
	db, err := tsdb.OpenDBReadOnly(path, nil)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	blocks, err := db.Blocks()
	if err != nil {
		return nil, err
	}

	metas := []*newBlockMeta{
		&newBlockMeta{
			index:     0,
			count:     0,
			mint:      math.MaxInt64,
			maxt:      dbMint,
			isAligned: false,
		},
	}

	for idx, block := range blocks {
		start, end := block.Meta().MinTime, block.Meta().MaxTime
		metas = append(metas, &newBlockMeta{
			index:     idx + 1,
			count:     0,
			mint:      start,
			maxt:      end,
			isAligned: true,
		})
		if idx == 0 {
			dbMint, dbMaxt = start, end
		} else {
			dbMint = value.MinInt64(dbMint, start)
			dbMaxt = value.MaxInt64(dbMaxt, end)
		}
	}
	// If there are existing blocks in the import location,
	// we need to add a block for samples that fall over DB maxt.
	if len(metas) > 1 {
		// Update first block with the right dbMint as it's maxt.
		metas[0].maxt = dbMint
		metas = append(metas, &newBlockMeta{
			index:     len(metas),
			count:     0,
			mint:      dbMaxt,
			maxt:      math.MinInt64,
			isAligned: false,
		})
	}
	return metas, nil
}

// getBlockIndex returns the index of the block the current timestamp corresponds to.
// User-provided times are assumed sorted in ascending order.
func getBlockIndex(current int64, metas []*newBlockMeta) int {
	for idx, m := range metas {
		if current < m.maxt {
			return idx
		}
	}
	return -1
}

// compactBlocks compacts all the given blocks and places them in the destination directory.
func compactBlocks(dest string, blocks []*newBlockMeta, logger log.Logger) (string, error) {
	dirs := make([]string, 0)
	for _, meta := range blocks {
		dirs = append(dirs, meta.dir)
	}
	path, err := compactDirs(dest, dirs, logger)
	if err != nil {
		return "", err
	}
	for _, meta := range blocks {
		// Remove existing blocks directory as it is no longer needed.
		if err = os.RemoveAll(meta.dir); err != nil {
			return "", err
		}
		// Update child with new blocks path.
		meta.dir = path
	}
	return path, err
}

// compactMeta compacts the children of a given block, dividing them into the Prometheus recommended
// block sizes, if it has to.
func compactMeta(dest string, m *newBlockMeta, logger log.Logger) error {
	children := make([]*newBlockMeta, 0)
	if m.isAligned {
		path, err := compactBlocks(dest, m.children, logger)
		if err != nil {
			return err
		}
		m.dir = path
	} else {
		// Sort blocks to ensure we bin properly.
		sortBlocks(m.children)
		binned := binBlocks(m.children, []int64{tsdb.DefaultBlockDuration})
		for _, bin := range binned {
			if len(bin) <= 1 {
				children = append(children, bin...)
				continue
			}
			path, err := compactBlocks(dest, bin, logger)
			if err != nil {
				return err
			}
			children = append(children, &newBlockMeta{
				index:     m.index,
				count:     0,
				mint:      bin[0].mint,
				maxt:      bin[len(bin)-1].maxt,
				isAligned: m.isAligned,
				dir:       path,
			})
		}
	}
	m.children = children
	return nil
}

// compactDirs compacts the block dirs and places them in destination directory.
func compactDirs(dest string, dirs []string, logger log.Logger) (string, error) {
	path := ""
	compactor, err := tsdb.NewLeveledCompactor(context.Background(), nil, logger, []int64{tsdb.DefaultBlockDuration}, nil)
	if err != nil {
		return path, err
	}
	ulid, err := compactor.Compact(dest, dirs, nil)
	if err != nil {
		return path, err
	}
	path = filepath.Join(dest, ulid.String())
	return path, nil
}

// sortBlocks sorts block metadata according to the time limits.
func sortBlocks(blocks []*newBlockMeta) {
	sort.Slice(blocks, func(x, y int) bool {
		bx, by := blocks[x], blocks[y]
		if bx.mint != by.mint {
			return bx.mint < by.mint
		}
		return bx.maxt < by.maxt
	})
}
