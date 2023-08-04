package tsdb

import (
	"context"

	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"golang.org/x/sync/semaphore"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

var errAsyncBlockWriterNotRunning = errors.New("asyncBlockWriter doesn't run anymore")

// asyncBlockWriter runs a background goroutine that writes series and chunks to the block asynchronously.
// All calls on asyncBlockWriter must be done from single goroutine, it is not safe for concurrent usage from multiple goroutines.
type asyncBlockWriter struct {
	chunkPool chunkenc.Pool // Where to return chunks after writing.

	chunkw ChunkWriter
	indexw IndexWriter

	closeSemaphore *semaphore.Weighted

	seriesChan chan seriesToWrite
	finishedCh chan asyncBlockWriterResult

	closed bool
	result asyncBlockWriterResult
}

type asyncBlockWriterResult struct {
	stats BlockStats
	err   error
}

type seriesToWrite struct {
	lbls labels.Labels
	chks []chunks.Meta
}

func newAsyncBlockWriter(chunkPool chunkenc.Pool, chunkw ChunkWriter, indexw IndexWriter, closeSema *semaphore.Weighted) *asyncBlockWriter {
	bw := &asyncBlockWriter{
		chunkPool:      chunkPool,
		chunkw:         chunkw,
		indexw:         indexw,
		seriesChan:     make(chan seriesToWrite, 64),
		finishedCh:     make(chan asyncBlockWriterResult, 1),
		closeSemaphore: closeSema,
	}

	go bw.loop()
	return bw
}

// loop doing the writes. Return value is only used by defer statement, and is sent to the channel,
// before closing it.
func (bw *asyncBlockWriter) loop() (res asyncBlockWriterResult) {
	defer func() {
		bw.finishedCh <- res
		close(bw.finishedCh)
	}()

	stats := BlockStats{}
	ref := storage.SeriesRef(0)
	for sw := range bw.seriesChan {
		if err := bw.chunkw.WriteChunks(sw.chks...); err != nil {
			return asyncBlockWriterResult{err: errors.Wrap(err, "write chunks")}
		}
		if err := bw.indexw.AddSeries(ref, sw.lbls, sw.chks...); err != nil {
			return asyncBlockWriterResult{err: errors.Wrap(err, "add series")}
		}

		stats.NumChunks += uint64(len(sw.chks))
		stats.NumSeries++
		for _, chk := range sw.chks {
			stats.NumSamples += uint64(chk.Chunk.NumSamples())
		}

		for _, chk := range sw.chks {
			if err := bw.chunkPool.Put(chk.Chunk); err != nil {
				return asyncBlockWriterResult{err: errors.Wrap(err, "put chunk")}
			}
		}
		ref++
	}

	err := bw.closeSemaphore.Acquire(context.Background(), 1)
	if err != nil {
		return asyncBlockWriterResult{err: errors.Wrap(err, "failed to acquire semaphore before closing writers")}
	}
	defer bw.closeSemaphore.Release(1)

	// If everything went fine with writing so far, close writers.
	if err := bw.chunkw.Close(); err != nil {
		return asyncBlockWriterResult{err: errors.Wrap(err, "closing chunk writer")}
	}
	if err := bw.indexw.Close(); err != nil {
		return asyncBlockWriterResult{err: errors.Wrap(err, "closing index writer")}
	}

	return asyncBlockWriterResult{stats: stats}
}

func (bw *asyncBlockWriter) addSeries(lbls labels.Labels, chks []chunks.Meta) error {
	select {
	case bw.seriesChan <- seriesToWrite{lbls: lbls, chks: chks}:
		return nil
	case result, ok := <-bw.finishedCh:
		if ok {
			bw.result = result
		}

		// If the writer isn't running anymore because of an error occurred in loop()
		// then we should return that error too, otherwise it may be never reported
		// and we'll never know the actual root cause.
		if bw.result.err != nil {
			return errors.Wrap(bw.result.err, errAsyncBlockWriterNotRunning.Error())
		}
		return errAsyncBlockWriterNotRunning
	}
}

func (bw *asyncBlockWriter) closeAsync() {
	if !bw.closed {
		bw.closed = true

		close(bw.seriesChan)
	}
}

func (bw *asyncBlockWriter) waitFinished() (BlockStats, error) {
	// Wait for flusher to finish.
	result, ok := <-bw.finishedCh
	if ok {
		bw.result = result
	}

	return bw.result.stats, bw.result.err
}

type preventDoubleCloseIndexWriter struct {
	IndexWriter
	closed atomic.Bool
}

func newPreventDoubleCloseIndexWriter(iw IndexWriter) *preventDoubleCloseIndexWriter {
	return &preventDoubleCloseIndexWriter{IndexWriter: iw}
}

func (p *preventDoubleCloseIndexWriter) Close() error {
	if p.closed.CompareAndSwap(false, true) {
		return p.IndexWriter.Close()
	}
	return nil
}

type preventDoubleCloseChunkWriter struct {
	ChunkWriter
	closed atomic.Bool
}

func newPreventDoubleCloseChunkWriter(cw ChunkWriter) *preventDoubleCloseChunkWriter {
	return &preventDoubleCloseChunkWriter{ChunkWriter: cw}
}

func (p *preventDoubleCloseChunkWriter) Close() error {
	if p.closed.CompareAndSwap(false, true) {
		return p.ChunkWriter.Close()
	}
	return nil
}
