// Copyright 2025 The Prometheus Authors
//
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

package wlog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/compression"
)

func BenchmarkLiveReader_DecodeLargeRecord(b *testing.B) {
	for _, readers := range []int{1, 16, 128} {
		b.Run(fmt.Sprintf("readers=%d", readers), func(b *testing.B) {
			benchmarkLiveReaderDecodeLargeRecord(b, readers)
		})
	}
}

func benchmarkLiveReaderDecodeLargeRecord(b *testing.B, readers int) {
	dir := b.TempDir()

	w, err := New(nil, nil, dir, compression.Snappy)
	require.NoError(b, err)

	// Make the decoded record large, but highly compressible.
	// This isolates the cost this benchmark is intended to expose:
	// repeated snappy.Decode destination-buffer allocation when creating
	// multiple LiveReader instances.
	rec := bytes.Repeat([]byte{42}, 256*1024)

	const records = 32
	for range records {
		require.NoError(b, w.Log(rec))
	}
	require.NoError(b, w.Close())

	segData, err := os.ReadFile(SegmentName(dir, 0))
	require.NoError(b, err)

	logger := promslog.NewNopLogger()
	metrics := NewLiveReaderMetrics(nil)

	b.ReportAllocs()
	b.SetBytes(int64(readers * records * len(rec)))
	b.ResetTimer()

	var sink int
	for b.Loop() {
		total := 0
		readRecords := 0

		for range readers {
			r := NewLiveReader(logger, metrics, bytes.NewReader(segData))
			for r.Next() {
				got := r.Record()
				readRecords++
				total += len(got)
			}
			r.Close()
			if err := r.Err(); err != nil && !errors.Is(err, io.EOF) {
				b.Fatal(err)
			}
		}

		wantRecords := readers * records
		if readRecords != wantRecords {
			b.Fatalf("read %d records, want %d", readRecords, wantRecords)
		}

		wantBytes := readers * records * len(rec)
		if total != wantBytes {
			b.Fatalf("read %d bytes, want %d", total, wantBytes)
		}

		sink = total
	}
	_ = sink
}
