package wlog

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/compression"
)

func BenchmarkLiveReader_ReadSegment(b *testing.B) {
	dir := b.TempDir()
	w, err := New(nil, nil, dir, compression.Snappy)
	require.NoError(b, err)

	// Create a dummy record roughly simulating a series or sample record.
	// We want enough data to trigger compression and decoding.
	rec := make([]byte, 1024)
	for i := range rec {
		rec[i] = byte(i % 256)
	}

	// Write enough records to fill a few pages.
	var recs [][]byte
	for range 1000 {
		recs = append(recs, rec)
	}
	require.NoError(b, w.Log(recs...))
	require.NoError(b, w.Close())

	// Read the first segment file into memory.
	segName := SegmentName(dir, 0)
	segData, err := os.ReadFile(segName)
	require.NoError(b, err)

	logger := promslog.NewNopLogger()
	// We reuse the metrics object as Watcher would (it's created once in NewWatcher)
	metrics := NewLiveReaderMetrics(nil)

	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		// Simulate the loop in Watcher.readCheckpoint or similar where a new LiveReader is created for the segment.
		r := NewLiveReader(logger, metrics, bytes.NewReader(segData))
		for r.Next() {
			_ = r.Record()
		}
		r.Close()
		if r.Err() != nil && r.Err() != io.EOF {
			b.Fatal(r.Err())
		}
	}
}
