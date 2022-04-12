// Copyright 2021 The Prometheus Authors
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

package chunks

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestChunkWriteQueue_GettingChunkFromQueue(t *testing.T) {
	var blockWriterWg sync.WaitGroup
	blockWriterWg.Add(1)

	// blockingChunkWriter blocks until blockWriterWg is done.
	blockingChunkWriter := func(_ HeadSeriesRef, _, _ int64, _ chunkenc.Chunk, _ ChunkDiskMapperRef, _ bool) error {
		blockWriterWg.Wait()
		return nil
	}

	q := newChunkWriteQueue(nil, 1000, blockingChunkWriter)

	defer q.stop()
	defer blockWriterWg.Done()

	testChunk := chunkenc.NewXORChunk()
	var ref ChunkDiskMapperRef
	job := chunkWriteJob{
		chk: testChunk,
		ref: ref,
	}
	require.NoError(t, q.addJob(job))

	// Retrieve chunk from the queue.
	gotChunk := q.get(ref)
	require.Equal(t, testChunk, gotChunk)
}

func TestChunkWriteQueue_WritingThroughQueue(t *testing.T) {
	var (
		gotSeriesRef     HeadSeriesRef
		gotMint, gotMaxt int64
		gotChunk         chunkenc.Chunk
		gotRef           ChunkDiskMapperRef
		gotCutFile       bool
	)

	blockingChunkWriter := func(seriesRef HeadSeriesRef, mint, maxt int64, chunk chunkenc.Chunk, ref ChunkDiskMapperRef, cutFile bool) error {
		gotSeriesRef = seriesRef
		gotMint = mint
		gotMaxt = maxt
		gotChunk = chunk
		gotRef = ref
		gotCutFile = cutFile
		return nil
	}

	q := newChunkWriteQueue(nil, 1000, blockingChunkWriter)
	defer q.stop()

	seriesRef := HeadSeriesRef(1)
	var mint, maxt int64 = 2, 3
	chunk := chunkenc.NewXORChunk()
	ref := newChunkDiskMapperRef(321, 123)
	cutFile := true
	awaitCb := make(chan struct{})
	require.NoError(t, q.addJob(chunkWriteJob{seriesRef: seriesRef, mint: mint, maxt: maxt, chk: chunk, ref: ref, cutFile: cutFile, callback: func(err error) {
		close(awaitCb)
	}}))
	<-awaitCb

	// Compare whether the write function has received all job attributes correctly.
	require.Equal(t, seriesRef, gotSeriesRef)
	require.Equal(t, mint, gotMint)
	require.Equal(t, maxt, gotMaxt)
	require.Equal(t, chunk, gotChunk)
	require.Equal(t, ref, gotRef)
	require.Equal(t, cutFile, gotCutFile)
}

func TestChunkWriteQueue_WrappingAroundSizeLimit(t *testing.T) {
	sizeLimit := 100
	unblockChunkWriterCh := make(chan struct{}, sizeLimit)

	// blockingChunkWriter blocks until the unblockChunkWriterCh channel returns a value.
	blockingChunkWriter := func(seriesRef HeadSeriesRef, mint, maxt int64, chunk chunkenc.Chunk, ref ChunkDiskMapperRef, cutFile bool) error {
		<-unblockChunkWriterCh
		return nil
	}

	q := newChunkWriteQueue(nil, sizeLimit, blockingChunkWriter)
	defer q.stop()
	// Unblock writers when shutting down.
	defer close(unblockChunkWriterCh)

	var chunkRef ChunkDiskMapperRef
	var callbackWg sync.WaitGroup
	addChunk := func() {
		callbackWg.Add(1)
		require.NoError(t, q.addJob(chunkWriteJob{
			ref: chunkRef,
			callback: func(err error) {
				callbackWg.Done()
			},
		}))
		chunkRef++
	}

	unblockChunkWriter := func() {
		unblockChunkWriterCh <- struct{}{}
	}

	// Fill the queue to the middle of the size limit.
	for job := 0; job < sizeLimit/2; job++ {
		addChunk()
	}

	// Consume the jobs.
	for job := 0; job < sizeLimit/2; job++ {
		unblockChunkWriter()
	}

	// Add jobs until the queue is full.
	// Note that one more job than <sizeLimit> can be added because one will be processed by the worker already
	// and it will block on the chunk write function.
	for job := 0; job < sizeLimit+1; job++ {
		addChunk()
	}

	// The queue should be full.
	require.True(t, q.queueIsFull())

	// Adding another job should block as long as no job from the queue gets consumed.
	addedJob := atomic.NewBool(false)
	go func() {
		addChunk()
		addedJob.Store(true)
	}()

	// Wait for 10ms while the adding of a new job is blocked.
	time.Sleep(time.Millisecond * 10)
	require.False(t, addedJob.Load())

	// Consume one job from the queue.
	unblockChunkWriter()

	// Wait until the job has been added to the queue.
	require.Eventually(t, func() bool { return addedJob.Load() }, time.Second, time.Millisecond*10)

	// The queue should be full again.
	require.True(t, q.queueIsFull())

	// Consume <sizeLimit>+1 jobs from the queue.
	// To drain the queue we need to consume <sizeLimit>+1 jobs because 1 job
	// is already in the state of being processed.
	for job := 0; job < sizeLimit+1; job++ {
		require.False(t, q.queueIsEmpty())
		unblockChunkWriter()
	}

	// Wait until all jobs have been processed.
	callbackWg.Wait()
	require.Eventually(t, q.queueIsEmpty, 500*time.Millisecond, 50*time.Millisecond)
}

func TestChunkWriteQueue_HandlerErrorViaCallback(t *testing.T) {
	testError := errors.New("test error")
	chunkWriter := func(_ HeadSeriesRef, _, _ int64, _ chunkenc.Chunk, _ ChunkDiskMapperRef, _ bool) error {
		return testError
	}

	awaitCb := make(chan struct{})
	var gotError error
	callback := func(err error) {
		gotError = err
		close(awaitCb)
	}

	q := newChunkWriteQueue(nil, 1, chunkWriter)
	defer q.stop()

	job := chunkWriteJob{callback: callback}
	require.NoError(t, q.addJob(job))

	<-awaitCb

	require.Equal(t, testError, gotError)
}

func BenchmarkChunkWriteQueue_addJob(b *testing.B) {
	for _, withReads := range []bool{false, true} {
		b.Run(fmt.Sprintf("with reads %t", withReads), func(b *testing.B) {
			for _, concurrentWrites := range []int{1, 10, 100, 1000} {
				b.Run(fmt.Sprintf("%d concurrent writes", concurrentWrites), func(b *testing.B) {
					issueReadSignal := make(chan struct{})
					q := newChunkWriteQueue(nil, 1000, func(ref HeadSeriesRef, i, i2 int64, chunk chunkenc.Chunk, ref2 ChunkDiskMapperRef, b bool) error {
						if withReads {
							select {
							case issueReadSignal <- struct{}{}:
							default:
								// Can't write to issueReadSignal, don't block but omit read instead.
							}
						}
						return nil
					})
					b.Cleanup(func() {
						// Stopped already, so no more writes will happen.
						close(issueReadSignal)
					})
					b.Cleanup(q.stop)

					start := sync.WaitGroup{}
					start.Add(1)

					jobs := make(chan chunkWriteJob, b.N)
					for i := 0; i < b.N; i++ {
						jobs <- chunkWriteJob{
							seriesRef: HeadSeriesRef(i),
							ref:       ChunkDiskMapperRef(i),
						}
					}
					close(jobs)

					go func() {
						for range issueReadSignal {
							// We don't care about the ID we're getting, we just want to grab the lock.
							_ = q.get(ChunkDiskMapperRef(0))
						}
					}()

					done := sync.WaitGroup{}
					done.Add(concurrentWrites)
					for w := 0; w < concurrentWrites; w++ {
						go func() {
							start.Wait()
							for j := range jobs {
								_ = q.addJob(j)
							}
							done.Done()
						}()
					}

					b.ResetTimer()
					start.Done()
					done.Wait()
				})
			}
		})
	}
}
