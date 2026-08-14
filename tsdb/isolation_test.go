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

package tsdb

import (
	"math"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsolation(t *testing.T) {
	type result struct {
		id           uint64
		lowWatermark uint64
	}
	var appendA, appendB result
	iso := newIsolation(false)

	// Low watermark starts at 1.
	require.Equal(t, uint64(0), iso.lowWatermark())
	require.Equal(t, int64(math.MaxInt64), iso.lowestAppendTime())

	// Pretend we are starting to append.
	appendA.id, appendA.lowWatermark = iso.newAppendID(10)
	require.Equal(t, result{1, 1}, appendA)
	require.Equal(t, uint64(1), iso.lowWatermark())

	require.Equal(t, 0, countOpenReads(iso))
	require.Equal(t, int64(10), iso.lowestAppendTime())

	// Now we start a read.
	stateA := iso.State(10, 20)
	require.Equal(t, 1, countOpenReads(iso))

	// Second appender.
	appendB.id, appendB.lowWatermark = iso.newAppendID(20)
	require.Equal(t, result{2, 1}, appendB)
	require.Equal(t, uint64(1), iso.lowWatermark())
	require.Equal(t, int64(10), iso.lowestAppendTime())

	iso.closeAppend(appendA.id)
	// Low watermark remains at 1 because stateA is still open
	require.Equal(t, uint64(1), iso.lowWatermark())

	require.Equal(t, 1, countOpenReads(iso))
	require.Equal(t, int64(20), iso.lowestAppendTime())

	// Finish the read and low watermark should rise.
	stateA.Close()
	require.Equal(t, uint64(2), iso.lowWatermark())

	require.Equal(t, 0, countOpenReads(iso))

	iso.closeAppend(appendB.id)
	require.Equal(t, uint64(2), iso.lowWatermark())
	require.Equal(t, int64(math.MaxInt64), iso.lowestAppendTime())
}

// TestCommittedAppendID pins the contract of (*isolation).committedAppendID:
// it returns the highest appendID that is guaranteed to be committed, i.e. the
// highest ID a block writer cannot see in incompleteAppends. Callers in the
// compaction path use it to decide whether a series can be safely evicted, so
// regressions here can cause silent data loss.
func TestCommittedAppendID(t *testing.T) {
	t.Run("disabled isolation always returns 0", func(t *testing.T) {
		iso := newIsolation(true)
		require.Equal(t, uint64(0), iso.committedAppendID())

		// Even after issuing an ID (which is a no-op when disabled), the result
		// must stay 0 because no per-sample append-IDs are tracked.
		_, _ = iso.newAppendID(0)
		require.Equal(t, uint64(0), iso.committedAppendID())
	})

	t.Run("no appenders ever opened", func(t *testing.T) {
		iso := newIsolation(false)
		require.Equal(t, uint64(0), iso.committedAppendID(),
			"with no appenders the sentinel's appendID is 0, which is also the last issued ID")
	})

	t.Run("all issued IDs closed returns lastAppendID", func(t *testing.T) {
		iso := newIsolation(false)
		idA, _ := iso.newAppendID(0)
		idB, _ := iso.newAppendID(0)
		iso.closeAppend(idA)
		iso.closeAppend(idB)
		require.Equal(t, idB, iso.committedAppendID(),
			"with no open appenders, committed must equal lastAppendID")
		require.Equal(t, iso.lastAppendID(), iso.committedAppendID())
	})

	t.Run("single open appender caps committed at id-1", func(t *testing.T) {
		iso := newIsolation(false)
		id, _ := iso.newAppendID(0)
		require.Equal(t, id-1, iso.committedAppendID(),
			"the only open appender is in flight, so committed is one below its id")
	})

	t.Run("multiple open appenders track the lowest", func(t *testing.T) {
		iso := newIsolation(false)
		idA, _ := iso.newAppendID(0) // 1
		idB, _ := iso.newAppendID(0) // 2
		idC, _ := iso.newAppendID(0) // 3
		require.Equal(t, idA-1, iso.committedAppendID(),
			"with A,B,C open the lowest open is A, so committed is A-1")

		// Closing the highest open ID must not change committed: A is still in
		// flight, so anything >= A is still uncovered by the block.
		iso.closeAppend(idC)
		require.Equal(t, idA-1, iso.committedAppendID(),
			"closing the highest open appender does not advance committed")

		// Closing the lowest open ID raises committed to (next lowest)-1.
		iso.closeAppend(idA)
		require.Equal(t, idB-1, iso.committedAppendID(),
			"closing the lowest open appender raises committed to (new lowest)-1")

		// Closing the last open appender returns committed to lastAppendID.
		iso.closeAppend(idB)
		require.Equal(t, idC, iso.committedAppendID(),
			"once every issued ID is closed, committed equals lastAppendID")
	})

	t.Run("open reads do not influence committed", func(t *testing.T) {
		// committedAppendID is independent of readsOpen: a stale read snapshot
		// captured at a lower watermark must not pull committed back, otherwise
		// long-running queries would unnecessarily block eviction.
		iso := newIsolation(false)
		idA, _ := iso.newAppendID(0)
		state := iso.State(0, math.MaxInt64)
		iso.closeAppend(idA)

		require.Equal(t, idA, iso.committedAppendID(),
			"with no open appenders, committed must equal lastAppendID even while a read is open")

		state.Close()
		require.Equal(t, idA, iso.committedAppendID())
	})
}

func countOpenReads(iso *isolation) int {
	count := 0
	iso.TraverseOpenReads(func(*isolationState) bool {
		count++
		return true
	})
	return count
}

func BenchmarkIsolation(b *testing.B) {
	for _, goroutines := range []int{10, 100, 1000, 10000} {
		b.Run(strconv.Itoa(goroutines), func(b *testing.B) {
			iso := newIsolation(false)

			wg := sync.WaitGroup{}
			start := make(chan struct{})

			for range goroutines {
				wg.Go(func() {
					<-start

					for b.Loop() {
						appendID, _ := iso.newAppendID(0)

						iso.closeAppend(appendID)
					}
				})
			}

			b.ResetTimer()
			close(start)
			wg.Wait()
		})
	}
}

func BenchmarkIsolationWithState(b *testing.B) {
	for _, goroutines := range []int{10, 100, 1000, 10000} {
		b.Run(strconv.Itoa(goroutines), func(b *testing.B) {
			iso := newIsolation(false)

			wg := sync.WaitGroup{}
			start := make(chan struct{})

			for range goroutines {
				wg.Go(func() {
					<-start

					for b.Loop() {
						appendID, _ := iso.newAppendID(0)

						iso.closeAppend(appendID)
					}
				})
			}

			readers := goroutines / 100
			if readers == 0 {
				readers++
			}

			for g := 0; g < readers; g++ {
				wg.Go(func() {
					<-start

					for b.Loop() {
						s := iso.State(math.MinInt64, math.MaxInt64)
						s.Close()
					}
				})
			}

			b.ResetTimer()
			close(start)
			wg.Wait()
		})
	}
}
