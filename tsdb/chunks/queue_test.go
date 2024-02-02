// Copyright 2022 The Prometheus Authors
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
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func (q *writeJobQueue) assertInvariants(t *testing.T) {
	q.mtx.Lock()
	defer q.mtx.Unlock()

	totalSize := 0
	for s := q.first; s != nil; s = s.nextSegment {
		require.NotNil(t, s.segment)

		// Next read index is lower or equal than next write index (we cannot past written jobs)
		require.LessOrEqual(t, s.nextRead, s.nextWrite)

		// Number of unread elements in this segment.
		totalSize += s.nextWrite - s.nextRead

		// First segment can be partially read, other segments were not read yet.
		if s == q.first {
			require.GreaterOrEqual(t, s.nextRead, 0)
		} else {
			require.Equal(t, 0, s.nextRead)
		}

		// If first shard is empty (everything was read from it already), it must have extra capacity for
		// additional elements, otherwise it would have been removed.
		if s == q.first && s.nextRead == s.nextWrite {
			require.Less(t, s.nextWrite, len(s.segment))
		}

		// Segments in the middle are full.
		if s != q.first && s != q.last {
			require.Len(t, s.segment, s.nextWrite)
		}
		// Last segment must have at least one element, or we wouldn't have created it.
		require.Greater(t, s.nextWrite, 0)
	}

	require.Equal(t, q.size, totalSize)
}

func TestQueuePushPopSingleGoroutine(t *testing.T) {
	seed := time.Now().UnixNano()
	t.Log("seed:", seed)
	r := rand.New(rand.NewSource(seed))

	const maxSize = 500
	const maxIters = 50

	for max := 1; max < maxSize; max++ {
		queue := newWriteJobQueue(max, 1+(r.Int()%max))

		elements := 0 // total elements in the queue
		lastWriteID := 0
		lastReadID := 0

		for iter := 0; iter < maxIters; iter++ {
			if elements < max {
				toWrite := r.Int() % (max - elements)
				if toWrite == 0 {
					toWrite = 1
				}

				for i := 0; i < toWrite; i++ {
					lastWriteID++
					require.True(t, queue.push(chunkWriteJob{seriesRef: HeadSeriesRef(lastWriteID)}))

					elements++
				}
			}

			if elements > 0 {
				toRead := r.Int() % elements
				if toRead == 0 {
					toRead = 1
				}

				for i := 0; i < toRead; i++ {
					lastReadID++

					j, b := queue.pop()
					require.True(t, b)
					require.Equal(t, HeadSeriesRef(lastReadID), j.seriesRef)

					elements--
				}
			}

			require.Equal(t, elements, queue.length())
			queue.assertInvariants(t)
		}
	}
}

func TestQueuePushBlocksOnFullQueue(t *testing.T) {
	queue := newWriteJobQueue(5, 5)

	pushTime := make(chan time.Time)
	go func() {
		require.True(t, queue.push(chunkWriteJob{seriesRef: 1}))
		require.True(t, queue.push(chunkWriteJob{seriesRef: 2}))
		require.True(t, queue.push(chunkWriteJob{seriesRef: 3}))
		require.True(t, queue.push(chunkWriteJob{seriesRef: 4}))
		require.True(t, queue.push(chunkWriteJob{seriesRef: 5}))
		pushTime <- time.Now()
		// This will block
		require.True(t, queue.push(chunkWriteJob{seriesRef: 6}))
		pushTime <- time.Now()
	}()

	timeBeforePush := <-pushTime

	delay := 100 * time.Millisecond
	select {
	case <-time.After(delay):
		// ok
	case <-pushTime:
		require.Fail(t, "didn't expect another push to proceed")
	}

	popTime := time.Now()
	j, b := queue.pop()
	require.True(t, b)
	require.Equal(t, HeadSeriesRef(1), j.seriesRef)

	timeAfterPush := <-pushTime

	require.GreaterOrEqual(t, timeAfterPush.Sub(popTime), time.Duration(0))
	require.GreaterOrEqual(t, timeAfterPush.Sub(timeBeforePush), delay)
}

func TestQueuePopBlocksOnEmptyQueue(t *testing.T) {
	queue := newWriteJobQueue(5, 5)

	popTime := make(chan time.Time)
	go func() {
		j, b := queue.pop()
		require.True(t, b)
		require.Equal(t, HeadSeriesRef(1), j.seriesRef)

		popTime <- time.Now()

		// This will block
		j, b = queue.pop()
		require.True(t, b)
		require.Equal(t, HeadSeriesRef(2), j.seriesRef)

		popTime <- time.Now()
	}()

	queue.push(chunkWriteJob{seriesRef: 1})

	timeBeforePop := <-popTime

	delay := 100 * time.Millisecond
	select {
	case <-time.After(delay):
		// ok
	case <-popTime:
		require.Fail(t, "didn't expect another pop to proceed")
	}

	pushTime := time.Now()
	require.True(t, queue.push(chunkWriteJob{seriesRef: 2}))

	timeAfterPop := <-popTime

	require.GreaterOrEqual(t, timeAfterPop.Sub(pushTime), time.Duration(0))
	require.Greater(t, timeAfterPop.Sub(timeBeforePop), delay)
}

func TestQueuePopUnblocksOnClose(t *testing.T) {
	queue := newWriteJobQueue(5, 5)

	popTime := make(chan time.Time)
	go func() {
		j, b := queue.pop()
		require.True(t, b)
		require.Equal(t, HeadSeriesRef(1), j.seriesRef)

		popTime <- time.Now()

		// This will block until queue is closed.
		j, b = queue.pop()
		require.False(t, b)

		popTime <- time.Now()
	}()

	queue.push(chunkWriteJob{seriesRef: 1})

	timeBeforePop := <-popTime

	delay := 100 * time.Millisecond
	select {
	case <-time.After(delay):
		// ok
	case <-popTime:
		require.Fail(t, "didn't expect another pop to proceed")
	}

	closeTime := time.Now()
	queue.close()

	timeAfterPop := <-popTime

	require.GreaterOrEqual(t, timeAfterPop.Sub(closeTime), time.Duration(0))
	require.GreaterOrEqual(t, timeAfterPop.Sub(timeBeforePop), delay)
}

func TestQueuePopAfterCloseReturnsAllElements(t *testing.T) {
	const count = 10

	queue := newWriteJobQueue(count, count)

	for i := 0; i < count; i++ {
		require.True(t, queue.push(chunkWriteJob{seriesRef: HeadSeriesRef(i)}))
	}

	// close the queue before popping all elements.
	queue.close()

	// No more pushing allowed after close.
	require.False(t, queue.push(chunkWriteJob{seriesRef: HeadSeriesRef(11111)}))

	// Verify that we can still read all pushed elements.
	for i := 0; i < count; i++ {
		j, b := queue.pop()
		require.True(t, b)
		require.Equal(t, HeadSeriesRef(i), j.seriesRef)
	}

	_, b := queue.pop()
	require.False(t, b)
}

func TestQueuePushPopManyGoroutines(t *testing.T) {
	const readGoroutines = 5
	const writeGoroutines = 10
	const writes = 500

	queue := newWriteJobQueue(1024, 64)

	// Reading goroutine
	refsMx := sync.Mutex{}
	refs := map[HeadSeriesRef]bool{}

	readersWG := sync.WaitGroup{}
	for i := 0; i < readGoroutines; i++ {
		readersWG.Add(1)

		go func() {
			defer readersWG.Done()

			for j, ok := queue.pop(); ok; j, ok = queue.pop() {
				refsMx.Lock()
				refs[j.seriesRef] = true
				refsMx.Unlock()
			}
		}()
	}

	id := atomic.Uint64{}

	writersWG := sync.WaitGroup{}
	for i := 0; i < writeGoroutines; i++ {
		writersWG.Add(1)

		go func() {
			defer writersWG.Done()

			for i := 0; i < writes; i++ {
				ref := id.Inc()

				require.True(t, queue.push(chunkWriteJob{seriesRef: HeadSeriesRef(ref)}))
			}
		}()
	}

	// Wait until all writes are done.
	writersWG.Wait()

	// Close the queue and wait for reading to be done.
	queue.close()
	readersWG.Wait()

	// Check if we have all expected values
	require.Len(t, refs, writeGoroutines*writes)
}

func TestQueueSegmentIsKeptEvenIfEmpty(t *testing.T) {
	queue := newWriteJobQueue(1024, 64)

	require.True(t, queue.push(chunkWriteJob{seriesRef: 1}))
	_, b := queue.pop()
	require.True(t, b)

	require.NotNil(t, queue.first)
	require.Equal(t, 1, queue.first.nextRead)
	require.Equal(t, 1, queue.first.nextWrite)
}
