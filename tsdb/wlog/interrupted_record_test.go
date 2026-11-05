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

package wlog_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

const (
	interruptedWritePageSize    = 32 * 1024
	interruptedWriteSegmentSize = 8 * interruptedWritePageSize
	interruptedWriteSampleCount = 16384
)

func TestHeadAppenderInterruptedWrite(t *testing.T) {
	for _, compress := range compression.Types() {
		for _, bytesBeforeErr := range []int{1, 13} {
			t.Run(fmt.Sprintf("%s/bytesBeforeErr=%d", compress, bytesBeforeErr), func(t *testing.T) {
				dir := t.TempDir()
				head, wal := newInterruptedWriteHead(t, dir, compress)
				seriesLabels := labels.FromStrings("__name__", "prefix")

				prefixAppender := head.AppenderV2(context.Background())
				ref, err := prefixAppender.Append(0, seriesLabels, 0, 0, 1, nil, nil, storage.AOptions{})
				if err != nil {
					t.Fatalf("append prefix sample: %v", err)
				}
				if err := prefixAppender.Commit(); err != nil {
					t.Fatalf("commit prefix sample: %v", err)
				}

				samples := interruptedWriteSamples(chunks.HeadSeriesRef(ref))
				encoder := record.Encoder{}
				rawRecord := encoder.Samples(samples, nil)
				encoded := rawRecord
				if compress != compression.None {
					compressed, err := compression.Encode(compress, rawRecord, compression.NewSyncEncodeBuffer())
					if err != nil {
						t.Fatalf("encode representative sample batch: %v", err)
					}
					if len(rawRecord)-len(compressed) > 0 {
						encoded = compressed
					}
				}
				if len(encoded) <= interruptedWritePageSize {
					t.Fatalf("sample batch does not span pages: %d bytes", len(encoded))
				}
				if len(encoded) >= interruptedWriteSegmentSize-interruptedWritePageSize {
					t.Fatalf("encoded sample batch does not fit in the active segment: %d bytes", len(encoded))
				}

				faultErr := errors.New("injected segment write failure")
				wlog.InjectWriteFailureForTest(wal, bytesBeforeErr, faultErr)
				failingAppender := head.AppenderV2(context.Background())
				for _, sample := range samples {
					if _, err := failingAppender.Append(ref, seriesLabels, 0, sample.T, sample.V, nil, nil, storage.AOptions{}); err != nil {
						t.Fatalf("append sample batch: %v", err)
					}
				}
				if err := failingAppender.Commit(); !errors.Is(err, faultErr) {
					t.Fatalf("setup did not deliver the injected WAL failure through Commit: %v", err)
				}

				laterAppender := head.AppenderV2(context.Background())
				if _, err := laterAppender.Append(ref, seriesLabels, 0, interruptedWriteSampleCount+1, 2, nil, nil, storage.AOptions{}); err != nil {
					t.Fatalf("append later sample: %v", err)
				}
				if err := laterAppender.Commit(); !errors.Is(err, faultErr) {
					t.Errorf("a later TSDB Commit did not return the interrupted WAL error: %v", err)
				}

				existingSeriesRollback := head.AppenderV2(context.Background())
				if _, err := existingSeriesRollback.Append(ref, seriesLabels, 0, interruptedWriteSampleCount+2, 2, nil, nil, storage.AOptions{}); err != nil {
					t.Fatalf("append existing series for rollback: %v", err)
				}
				if err := existingSeriesRollback.Rollback(); err != nil {
					t.Fatalf("rollback with no series record to log: %v", err)
				}
				newSeriesRollback := head.AppenderV2(context.Background())
				if _, err := newSeriesRollback.Append(0, labels.FromStrings("__name__", "new_series"), 0, 0, 1, nil, nil, storage.AOptions{}); err != nil {
					t.Fatalf("append new series for rollback: %v", err)
				}
				if err := newSeriesRollback.Rollback(); !errors.Is(err, faultErr) {
					t.Errorf("a new-series Rollback did not return the interrupted WAL error: %v", err)
				}
				if err := head.Close(); !errors.Is(err, faultErr) {
					t.Errorf("Close did not report the interrupted WAL error: %v", err)
				}
				if _, _, _, err := tsdb.LastChunkSnapshot(dir); !errors.Is(err, record.ErrNotFound) {
					t.Errorf("interrupted Head shutdown created a chunk snapshot: %v", err)
				}
				reopenedWAL, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), interruptedWriteSegmentSize, compress)
				if err != nil {
					t.Fatalf("open WAL for repair: %v", err)
				}
				reopenedHead, err := newInterruptedWriteHeadWithWAL(t, dir, reopenedWAL)
				if err != nil {
					t.Fatalf("create reopened Head: %v", err)
				}
				initErr := reopenedHead.Init(math.MinInt64)
				if initErr == nil {
					t.Fatal("expected replay to report the interrupted WAL record")
				}
				if _, ok := errors.AsType[*wlog.CorruptionErr](initErr); !ok {
					t.Fatalf("replay returned an unexpected error instead of WAL corruption: %v", initErr)
				}
				if err := reopenedWAL.Repair(initErr); err != nil {
					t.Fatalf("repair interrupted WAL: %v", err)
				}
				verifyReplayedPrefix(t, reopenedHead)

				postRepairAppender := reopenedHead.AppenderV2(context.Background())
				if _, err := postRepairAppender.Append(ref, seriesLabels, 0, interruptedWriteSampleCount+2, 3, nil, nil, storage.AOptions{}); err != nil {
					t.Fatalf("append after repaired reopen: %v", err)
				}
				if err := postRepairAppender.Commit(); err != nil {
					t.Fatalf("commit after repaired reopen: %v", err)
				}
				if err := reopenedHead.Close(); err != nil {
					t.Fatalf("close repaired Head: %v", err)
				}
				recovered, _ := newInterruptedWriteHead(t, dir, compress)
				verifyReplayedPrefix(t, recovered)
			})
		}
	}
}

func verifyReplayedPrefix(t *testing.T, head *tsdb.Head) {
	t.Helper()
	querier, err := tsdb.NewBlockQuerier(head, 0, 1)
	if err != nil {
		t.Fatalf("create prefix querier: %v", err)
	}
	defer func() {
		if err := querier.Close(); err != nil {
			t.Fatalf("close prefix querier: %v", err)
		}
	}()

	seriesSet := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "__name__", "prefix"))
	if !seriesSet.Next() {
		if err := seriesSet.Err(); err != nil {
			t.Fatalf("query committed prefix: %v", err)
		}
		t.Error("replay did not restore the committed prefix series")
		return
	}

	iterator := seriesSet.At().Iterator(nil)
	valueType := iterator.Next()
	if err := iterator.Err(); err != nil {
		t.Fatalf("iterate committed prefix: %v", err)
	}
	if valueType != chunkenc.ValFloat {
		t.Error("replay did not restore the prefix float sample")
		return
	}
	timestamp, value := iterator.At()
	if timestamp != 0 || value != 1 {
		t.Errorf("replayed prefix sample changed: got (%d, %v), want (0, 1)", timestamp, value)
	}
	if iterator.Next() != chunkenc.ValNone {
		if err := iterator.Err(); err != nil {
			t.Fatalf("finish prefix iteration: %v", err)
		}
		t.Error("replay returned an unexpected extra prefix sample")
	}
	if err := iterator.Err(); err != nil {
		t.Fatalf("finish prefix iteration: %v", err)
	}
	if seriesSet.Next() {
		t.Error("replay returned a duplicate prefix series")
	}
	if err := seriesSet.Err(); err != nil {
		t.Fatalf("finish prefix query: %v", err)
	}
}

func newInterruptedWriteHead(t *testing.T, dir string, compress compression.Type) (*tsdb.Head, *wlog.WL) {
	t.Helper()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), interruptedWriteSegmentSize, compress)
	if err != nil {
		t.Fatalf("create WAL: %v", err)
	}
	head, err := newInterruptedWriteHeadWithWAL(t, dir, wal)
	if err != nil {
		t.Fatalf("create Head: %v", err)
	}
	if err := head.Init(math.MinInt64); err != nil {
		t.Fatalf("initialize Head: %v", err)
	}
	return head, wal
}

func newInterruptedWriteHeadWithWAL(t *testing.T, dir string, wal *wlog.WL) (*tsdb.Head, error) {
	t.Helper()
	opts := tsdb.DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.EnableMemorySnapshotOnShutdown = true
	head, err := tsdb.NewHead(nil, nil, wal, nil, opts, nil)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = head.Close() })
	return head, nil
}

func interruptedWriteSamples(ref chunks.HeadSeriesRef) []record.RefSample {
	samples := make([]record.RefSample, interruptedWriteSampleCount)
	state := uint64(0x243f6a8885a308d3)
	for i := range samples {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		samples[i] = record.RefSample{
			Ref: ref,
			T:   int64(i + 1),
			V:   math.Float64frombits(0x3ff0000000000000 | (state & 0x000fffffffffffff)),
		}
	}
	return samples
}
