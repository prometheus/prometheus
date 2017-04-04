// Copyright 2016 The Prometheus Authors
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
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/storage/local/chunk"
	"github.com/prometheus/prometheus/storage/local/codable"
)

const (
	headsFileName            = "heads.db"
	headsTempFileName        = "heads.db.tmp"
	headsFormatVersion       = 2
	headsFormatLegacyVersion = 1 // Can read, but will never write.
	headsMagicString         = "PrometheusHeads"
)

// headsScanner is a scanner to read time series with their heads from a
// heads.db file. It follows a similar semantics as the bufio.Scanner.
// It is not safe to use a headsScanner concurrently.
type headsScanner struct {
	f                    *os.File
	r                    *bufio.Reader
	fp                   model.Fingerprint // Read after each scan() call that has returned true.
	series               *memorySeries     // Read after each scan() call that has returned true.
	version              int64             // Read after newHeadsScanner has returned.
	seriesTotal          uint64            // Read after newHeadsScanner has returned.
	seriesCurrent        uint64
	chunksToPersistTotal int64 // Read after scan() has returned false.
	err                  error // Read after scan() has returned false.
}

func newHeadsScanner(filename string) *headsScanner {
	hs := &headsScanner{}
	defer func() {
		if hs.f != nil && hs.err != nil {
			hs.f.Close()
		}
	}()

	if hs.f, hs.err = os.Open(filename); hs.err != nil {
		return hs
	}
	hs.r = bufio.NewReaderSize(hs.f, fileBufSize)

	buf := make([]byte, len(headsMagicString))
	if _, hs.err = io.ReadFull(hs.r, buf); hs.err != nil {
		return hs
	}
	magic := string(buf)
	if magic != headsMagicString {
		hs.err = fmt.Errorf(
			"unexpected magic string, want %q, got %q",
			headsMagicString, magic,
		)
		return hs
	}
	hs.version, hs.err = binary.ReadVarint(hs.r)
	if (hs.version != headsFormatVersion && hs.version != headsFormatLegacyVersion) || hs.err != nil {
		hs.err = fmt.Errorf(
			"unknown or unreadable heads format version, want %d, got %d, error: %s",
			headsFormatVersion, hs.version, hs.err,
		)
		return hs
	}
	if hs.seriesTotal, hs.err = codable.DecodeUint64(hs.r); hs.err != nil {
		return hs
	}
	return hs
}

// scan works like bufio.Scanner.Scan.
func (hs *headsScanner) scan() bool {
	if hs.seriesCurrent == hs.seriesTotal || hs.err != nil {
		return false
	}

	var (
		seriesFlags      byte
		fpAsInt          uint64
		metric           codable.Metric
		persistWatermark int64
		modTimeNano      int64
		modTime          time.Time
		chunkDescsOffset int64
		savedFirstTime   int64
		numChunkDescs    int64
		firstTime        int64
		lastTime         int64
		encoding         byte
		ch               chunk.Chunk
		lastTimeHead     model.Time
	)
	if seriesFlags, hs.err = hs.r.ReadByte(); hs.err != nil {
		return false
	}
	headChunkPersisted := seriesFlags&flagHeadChunkPersisted != 0
	if fpAsInt, hs.err = codable.DecodeUint64(hs.r); hs.err != nil {
		return false
	}
	hs.fp = model.Fingerprint(fpAsInt)

	if hs.err = metric.UnmarshalFromReader(hs.r); hs.err != nil {
		return false
	}
	if hs.version != headsFormatLegacyVersion {
		// persistWatermark only present in v2.
		persistWatermark, hs.err = binary.ReadVarint(hs.r)
		if persistWatermark < 0 {
			hs.err = fmt.Errorf("found negative persist watermark in checkpoint: %d", persistWatermark)
		}
		if hs.err != nil {
			return false
		}
		modTimeNano, hs.err = binary.ReadVarint(hs.r)
		if hs.err != nil {
			return false
		}
		if modTimeNano != -1 {
			modTime = time.Unix(0, modTimeNano)
		}
	}
	if chunkDescsOffset, hs.err = binary.ReadVarint(hs.r); hs.err != nil {
		return false
	}
	if savedFirstTime, hs.err = binary.ReadVarint(hs.r); hs.err != nil {
		return false
	}

	if numChunkDescs, hs.err = binary.ReadVarint(hs.r); hs.err != nil {
		return false
	}
	if numChunkDescs < 0 {
		hs.err = fmt.Errorf("found negative number of chunk descriptors in checkpoint: %d", numChunkDescs)
		return false
	}

	chunkDescs := make([]*chunk.Desc, numChunkDescs)
	if hs.version == headsFormatLegacyVersion {
		if headChunkPersisted {
			persistWatermark = numChunkDescs
		} else {
			persistWatermark = numChunkDescs - 1
		}
	}
	headChunkClosed := true // Initial assumption.
	for i := int64(0); i < numChunkDescs; i++ {
		if i < persistWatermark {
			if firstTime, hs.err = binary.ReadVarint(hs.r); hs.err != nil {
				return false
			}
			if lastTime, hs.err = binary.ReadVarint(hs.r); hs.err != nil {
				return false
			}
			chunkDescs[i] = &chunk.Desc{
				ChunkFirstTime: model.Time(firstTime),
				ChunkLastTime:  model.Time(lastTime),
			}
			chunk.NumMemDescs.Inc()
		} else {
			// Non-persisted chunk.
			// If there are non-persisted chunks at all, we consider
			// the head chunk not to be closed yet.
			headChunkClosed = false
			if encoding, hs.err = hs.r.ReadByte(); hs.err != nil {
				return false
			}
			if ch, hs.err = chunk.NewForEncoding(chunk.Encoding(encoding)); hs.err != nil {
				return false
			}
			if hs.err = ch.Unmarshal(hs.r); hs.err != nil {
				return false
			}
			cd := chunk.NewDesc(ch, ch.FirstTime())
			if i < numChunkDescs-1 {
				// This is NOT the head chunk. So it's a chunk
				// to be persisted, and we need to populate lastTime.
				hs.chunksToPersistTotal++
				if hs.err = cd.MaybePopulateLastTime(); hs.err != nil {
					return false
				}
			}
			chunkDescs[i] = cd
		}
	}

	if lastTimeHead, hs.err = chunkDescs[len(chunkDescs)-1].LastTime(); hs.err != nil {
		return false
	}

	hs.series = &memorySeries{
		metric:           model.Metric(metric),
		chunkDescs:       chunkDescs,
		persistWatermark: int(persistWatermark),
		modTime:          modTime,
		chunkDescsOffset: int(chunkDescsOffset),
		savedFirstTime:   model.Time(savedFirstTime),
		lastTime:         lastTimeHead,
		headChunkClosed:  headChunkClosed,
	}
	hs.seriesCurrent++
	return true
}

// close closes the underlying file if required.
func (hs *headsScanner) close() {
	if hs.f != nil {
		hs.f.Close()
	}
}

// DumpHeads writes the metadata of the provided heads file in a human-readable
// form.
func DumpHeads(filename string, out io.Writer) error {
	hs := newHeadsScanner(filename)
	defer hs.close()

	if hs.err == nil {
		fmt.Fprintf(
			out,
			">>> Dumping %d series from heads file %q with format version %d. <<<\n",
			hs.seriesTotal, filename, hs.version,
		)
	}
	for hs.scan() {
		s := hs.series
		fmt.Fprintf(
			out,
			"FP=%v\tMETRIC=%s\tlen(chunkDescs)=%d\tpersistWatermark=%d\tchunkDescOffset=%d\tsavedFirstTime=%v\tlastTime=%v\theadChunkClosed=%t\n",
			hs.fp, s.metric, len(s.chunkDescs), s.persistWatermark, s.chunkDescsOffset, s.savedFirstTime, s.lastTime, s.headChunkClosed,
		)
	}
	if hs.err == nil {
		fmt.Fprintf(
			out,
			">>> Dump complete. %d chunks to persist. <<<\n",
			hs.chunksToPersistTotal,
		)
	}
	return hs.err
}
