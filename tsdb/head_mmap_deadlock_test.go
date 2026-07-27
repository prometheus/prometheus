package tsdb

import (
    "context"
    "os"
    "testing"
    "time"

    promslog "github.com/prometheus/common/promslog"
    "github.com/prometheus/prometheus/model/labels"
    "github.com/stretchr/testify/require"
)

// Test that a panic during mmapHeadChunks (e.g., due to write errors)
// does not leak the stripe or series locks and therefore does not deadlock
// Head.Close(). This is a regression test for issue #19021.
func TestMmapHeadChunks_PanicDoesNotLeakLocks(t *testing.T) {
    t.Parallel()

    opts := DefaultHeadOptions()
    opts.ChunkRange = 1000 // Small range to force chunk cuts with few samples.
    opts.ChunkDirRoot = t.TempDir()

    h, err := NewHead(nil, promslog.NewNopLogger(), nil, nil, opts, NewHeadStats())
    require.NoError(t, err)
    require.NoError(t, h.Init(0))

    // Append enough samples to a single series so that the head contains
    // multiple head chunks eligible for mmapping.
    app := h.Appender(context.Background())
    lset := labels.FromStrings("__name__", "deadlock_regression", "job", "test")

    var ref uint64
    // First sample at ts=0 creates the first chunk.
    r, err := app.Append(0, lset, 0, 1)
    require.NoError(t, err)
    ref = uint64(r)

    // Next samples cross chunk range boundaries to cut new chunks.
    for i := 1; i <= 3; i++ {
        ts := int64(i)*opts.ChunkRange + 1
        _, err = app.Append(uint64(ref), labels.EmptyLabels(), ts, float64(i))
        require.NoError(t, err)
    }
    require.NoError(t, app.Commit())

    // Remove the mmapped chunk directory to provoke a write error when mmapping.
    // This triggers handleChunkWriteError to panic inside mmap, exercising the
    // panic path while locks are held.
    require.NoError(t, os.RemoveAll(mmappedChunksDir(h.opts.ChunkDirRoot)))

    // Run mmapHeadChunks and recover the expected panic.
    mmapDone := make(chan struct{})
    go func() {
        defer func() { _ = recover(); close(mmapDone) }()
        h.mmapHeadChunks()
    }()

    select {
    case <-mmapDone:
        // OK: panic recovered and goroutine unwound.
    case <-time.After(5 * time.Second):
        t.Fatal("mmapHeadChunks did not return (panic) in time")
    }

    // Restore the directory so that Close() doesn't encounter further write errors.
    require.NoError(t, os.MkdirAll(mmappedChunksDir(h.opts.ChunkDirRoot), 0o755))

    // Now ensure Close() completes and does not deadlock due to leaked locks.
    closeCh := make(chan error, 1)
    go func() { closeCh <- h.Close() }()

    select {
    case err := <-closeCh:
        require.NoError(t, err)
    case <-time.After(5 * time.Second):
        t.Fatal("Head.Close() deadlocked after mmap panic; locks may have leaked")
    }
}
