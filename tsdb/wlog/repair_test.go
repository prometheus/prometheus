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

package wlog

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/util/compression"
)

// createCorruptedWAL writes segs full segments to dir and overwrites bytes in
// the middle of segment 0. It returns the records written to segment 0 before
// the corrupted page.
func createCorruptedWAL(t *testing.T, dir string, segs, segmentSize int, compress compression.Type) (intact [][]byte) {
	t.Helper()
	recordSize := pageSize/4 - recordHeaderSize

	w, err := NewSize(promslog.NewNopLogger(), nil, dir, segmentSize, compress)
	require.NoError(t, err)
	for w.segment.Index() < segs {
		buf := make([]byte, recordSize)
		_, err := rand.Read(buf)
		require.NoError(t, err)
		require.NoError(t, w.Log(buf))
	}
	require.NoError(t, w.Close())
	// The last Log rotated into an extra, partly filled segment. Drop it and
	// its records so that exactly segs segments remain.
	require.NoError(t, os.Remove(SegmentName(dir, segs)))

	f, err := os.OpenFile(SegmentName(dir, 0), os.O_RDWR, 0o666)
	require.NoError(t, err)
	fi, err := f.Stat()
	require.NoError(t, err)
	off := (fi.Size() / 2) / pageSize * pageSize // Start of a page in the middle.
	_, err = f.WriteAt(bytes.Repeat([]byte{0xFF}, 64), off+recordHeaderSize)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	return readAllRecords(t, dir, true)
}

// readAllRecords returns the records the segments in dir yield before the first error.
func readAllRecords(t *testing.T, dir string, expectCorruption bool) [][]byte {
	t.Helper()
	sr, err := NewSegmentsReader(dir)
	require.NoError(t, err)
	defer sr.Close()
	r := NewReader(sr)
	var recs [][]byte
	for r.Next() {
		recs = append(recs, append([]byte(nil), r.Record()...))
	}
	if expectCorruption {
		require.Error(t, r.Err())
	} else {
		require.NoError(t, r.Err())
	}
	return recs
}

// openAndFindCorruption opens the WAL in dir the way startup does and returns it with
// the corruption error replay hits.
func openAndFindCorruption(t *testing.T, dir string, segmentSize int, compress compression.Type) (*WL, error) {
	t.Helper()
	w, err := NewSize(promslog.NewNopLogger(), nil, dir, segmentSize, compress)
	require.NoError(t, err)
	sr, err := NewSegmentsReader(dir)
	require.NoError(t, err)
	r := NewReader(sr)
	for r.Next() {
	}
	require.NoError(t, sr.Close())
	var cerr *CorruptionErr
	require.ErrorAs(t, r.Err(), &cerr)
	require.Equal(t, 0, cerr.Segment)
	return w, r.Err()
}

// TestRepair_StoppedWhileDeletingKeepsWALOpenable checks that a Repair that
// stops partway through deleting the segments after the corrupted one leaves
// segments that are still sequential, so the next start can open the WAL and
// repair again. Here a failed delete stops it; in production a crash or a
// kill at the same point leaves the same directory. If segments are deleted
// oldest first, it leaves a gap and the next start fails with "segments are
// not sequential" (https://github.com/prometheus/prometheus/issues/6080).
func TestRepair_StoppedWhileDeletingKeepsWALOpenable(t *testing.T) {
	const (
		segs        = 4 // 0 (corrupted) to 3; opening the WAL creates 4.
		segmentSize = 4 * pageSize
	)
	for blocked := 1; blocked < segs; blocked++ {
		t.Run(fmt.Sprintf("delete_of_segment_%d_fails", blocked), func(t *testing.T) {
			dir := t.TempDir()
			intact := createCorruptedWAL(t, dir, segs, segmentSize, compression.None)
			require.NotEmpty(t, intact)

			w, corruption := openAndFindCorruption(t, dir, segmentSize, compression.None)

			// Move the segment aside and put a non-empty directory of the
			// same name in its place, which os.Remove cannot delete.
			fn := SegmentName(dir, blocked)
			require.NoError(t, os.Rename(fn, fn+".aside"))
			require.NoError(t, os.MkdirAll(filepath.Join(fn, "blocker"), 0o777))

			require.Error(t, w.Repair(corruption))
			w.Close() // The repair may already have closed the active segment.

			// Remove the cause of the failure by putting the segment back. The
			// WAL must open again and the next repair must keep the records
			// before the corruption.
			require.NoError(t, os.RemoveAll(fn))
			require.NoError(t, os.Rename(fn+".aside", fn))
			_, err := listSegments(dir)
			require.NoError(t, err, "segments left after the failed repair")

			w, corruption = openAndFindCorruption(t, dir, segmentSize, compression.None)
			require.NoError(t, w.Repair(corruption))
			require.NoError(t, w.Close())
			require.Equal(t, intact, readAllRecords(t, dir, false))
		})
	}
}

// TestRepair_ConfigChange checks that Repair keeps every record before the
// corruption when the WAL was written with compression and larger segments
// than the ones it is opened with, so that the intact records no longer fit
// in one segment if they are encoded again.
func TestRepair_ConfigChange(t *testing.T) {
	dir := t.TempDir()
	w, err := NewSize(promslog.NewNopLogger(), nil, dir, 16*pageSize, compression.Snappy)
	require.NoError(t, err)
	for i := uint64(0); w.segment.Index() < 2; i++ {
		buf := bytes.Repeat([]byte("compressible WAL record"), pageSize/8)
		binary.LittleEndian.PutUint64(buf, i)
		require.NoError(t, w.Log(buf))
	}
	require.NoError(t, w.Close())
	require.NoError(t, os.Remove(SegmentName(dir, 2)))
	fn := SegmentName(dir, 0)
	encoded, err := os.ReadFile(fn)
	require.NoError(t, err)
	require.NotZero(t, encoded[0]&snappyMask, "fixture must contain actually compressed records")
	f, err := os.OpenFile(fn, os.O_RDWR, 0)
	require.NoError(t, err)
	_, err = f.WriteAt(bytes.Repeat([]byte{0xFF}, 64), int64(len(encoded)/2))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	intact := readAllRecords(t, dir, true)
	require.NotEmpty(t, intact)
	var expanded int
	for _, rec := range intact {
		expanded += len(rec) + recordHeaderSize
	}
	require.Greater(t, expanded, 2*pageSize, "intact records must overflow the new segment size when re-encoded")

	w, corruption := openAndFindCorruption(t, dir, 2*pageSize, compression.None)
	require.NoError(t, w.Repair(corruption))
	require.NoError(t, w.Close())

	require.Equal(t, intact, readAllRecords(t, dir, false))
	_, err = listSegments(dir)
	require.NoError(t, err)
}

// TestRepair_KeepsStaleRepairFile checks that Repair does not overwrite a
// <seg>.repair file left by an interrupted repair of an older version. Such a
// file can hold the only copy of the segment's intact records.
func TestRepair_KeepsStaleRepairFile(t *testing.T) {
	const segmentSize = 4 * pageSize
	dir := t.TempDir()
	createCorruptedWAL(t, dir, 2, segmentSize, compression.None)
	stale := SegmentName(dir, 0) + ".repair"
	content := []byte("left by an interrupted repair")
	require.NoError(t, os.WriteFile(stale, content, 0o666))

	w, corruption := openAndFindCorruption(t, dir, segmentSize, compression.None)
	require.NoError(t, w.Repair(corruption))
	require.NoError(t, w.Close())

	got, err := os.ReadFile(stale)
	require.NoError(t, err)
	require.Equal(t, content, got)
}

// TestRepair_FailedReplacementKeepsOriginal checks that when creating, writing
// or syncing the repaired segment fails, the corrupted segment is left in
// place byte for byte, so that the next start can repair it again. The write
// and sync cases point the temporary file at Linux devices that fail those
// operations, so no filesystem hooks are needed in the WAL; other platforms
// skip them.
func TestRepair_FailedReplacementKeepsOriginal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		block     func(t *testing.T, tmp string) // Makes the repair fail.
		errno     syscall.Errno                  // Expected error, if checked.
		operation string                         // Expected error text, if checked.
	}{
		{
			name: "create",
			block: func(t *testing.T, tmp string) {
				// A non-empty directory where the repaired segment is created.
				require.NoError(t, os.MkdirAll(filepath.Join(tmp, "blocker"), 0o777))
			},
		},
		{
			name: "write",
			block: func(t *testing.T, tmp string) {
				linuxDevice(t, "/dev/full")
				require.NoError(t, os.Symlink("/dev/full", tmp))
			},
			errno:     syscall.ENOSPC,
			operation: "copy intact records",
		},
		{
			name: "sync",
			block: func(t *testing.T, tmp string) {
				linuxDevice(t, "/dev/null")
				require.NoError(t, os.Symlink("/dev/null", tmp))
			},
			errno:     syscall.EINVAL,
			operation: "sync repaired segment",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const segmentSize = 4 * pageSize
			dir := t.TempDir()
			intact := createCorruptedWAL(t, dir, 2, segmentSize, compression.None)
			require.NotEmpty(t, intact)
			fn := SegmentName(dir, 0)
			before, err := os.ReadFile(fn)
			require.NoError(t, err)
			tc.block(t, fn+".repair.tmp")

			w, corruption := openAndFindCorruption(t, dir, segmentSize, compression.None)
			err = w.Repair(corruption)
			require.Error(t, err)
			if tc.errno != 0 {
				require.ErrorIs(t, err, tc.errno)
				require.ErrorContains(t, err, tc.operation)
			}
			w.Close() // Repair has already closed the active segment.
			after, err := os.ReadFile(fn)
			require.NoError(t, err)
			require.Equal(t, before, after)
			if tc.errno != 0 {
				// Repair removes the temporary file it failed to write.
				_, err = os.Lstat(fn + ".repair.tmp")
				require.True(t, os.IsNotExist(err))
			}

			require.NoError(t, os.RemoveAll(fn+".repair.tmp"))
			w, corruption = openAndFindCorruption(t, dir, segmentSize, compression.None)
			require.NoError(t, w.Repair(corruption))
			require.NoError(t, w.Close())
			require.Equal(t, intact, readAllRecords(t, dir, false))
		})
	}
}

// linuxDevice skips the test unless it runs on Linux and path exists.
func linuxDevice(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("requires Linux %s semantics", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("device unavailable: %v", err)
	}
}

// TestRepair_EmptyPrefixWritesZeroedPage checks that a repair that keeps no
// records still writes one zeroed page, as Repair did before it copied bytes.
// A reader over several segments reports a record torn at the end of one
// segment as corruption at offset 0 of the next. Without the page, the next
// replay fails again.
func TestRepair_EmptyPrefixWritesZeroedPage(t *testing.T) {
	const segmentSize = 4 * pageSize
	dir := t.TempDir()
	w, err := NewSize(nil, nil, dir, segmentSize, compression.None)
	require.NoError(t, err)
	require.NoError(t, w.Log([]byte("kept"), make([]byte, pageSize+1)))
	require.NoError(t, w.Close())
	// Cut the second record after its first page.
	require.NoError(t, os.Truncate(SegmentName(dir, 0), pageSize))

	// Opening the WAL creates the empty segment 1.
	w, err = NewSize(nil, nil, dir, segmentSize, compression.None)
	require.NoError(t, err)
	sr, err := NewSegmentsReader(dir)
	require.NoError(t, err)
	r := NewReader(sr)
	for r.Next() {
	}
	require.NoError(t, sr.Close())
	var cerr *CorruptionErr
	require.ErrorAs(t, r.Err(), &cerr)
	require.Equal(t, 1, cerr.Segment)
	require.Zero(t, cerr.Offset)

	require.NoError(t, w.Repair(cerr))
	require.NoError(t, w.Close())
	fi, err := os.Stat(SegmentName(dir, 1))
	require.NoError(t, err)
	require.Equal(t, int64(pageSize), fi.Size())
	require.Equal(t, [][]byte{[]byte("kept")}, readAllRecords(t, dir, false))
}
