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
				wg.Add(1)

				go func() {
					defer wg.Done()
					<-start

					for b.Loop() {
						appendID, _ := iso.newAppendID(0)

						iso.closeAppend(appendID)
					}
				}()
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
				wg.Add(1)

				go func() {
					defer wg.Done()
					<-start

					for b.Loop() {
						appendID, _ := iso.newAppendID(0)

						iso.closeAppend(appendID)
					}
				}()
			}

			readers := goroutines / 100
			if readers == 0 {
				readers++
			}

			for g := 0; g < readers; g++ {
				wg.Add(1)

				go func() {
					defer wg.Done()
					<-start

					for b.Loop() {
						s := iso.State(math.MinInt64, math.MaxInt64)
						s.Close()
					}
				}()
			}

			b.ResetTimer()
			close(start)
			wg.Wait()
		})
	}
}
