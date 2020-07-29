// Copyright 2020 The Prometheus Authors
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
	"strconv"
	"sync"
	"testing"
)

func BenchmarkIsolation(b *testing.B) {
	for _, goroutines := range []int{10, 100, 1000, 10000} {
		b.Run(strconv.Itoa(goroutines), func(b *testing.B) {
			iso := newIsolation()

			wg := sync.WaitGroup{}
			start := make(chan struct{})

			for g := 0; g < goroutines; g++ {
				wg.Add(1)

				go func() {
					defer wg.Done()
					<-start

					for i := 0; i < b.N; i++ {
						appendID := iso.newAppendID()
						_ = iso.lowWatermark()

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
			iso := newIsolation()

			wg := sync.WaitGroup{}
			start := make(chan struct{})

			for g := 0; g < goroutines; g++ {
				wg.Add(1)

				go func() {
					defer wg.Done()
					<-start

					for i := 0; i < b.N; i++ {
						appendID := iso.newAppendID()
						_ = iso.lowWatermark()

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

					for i := 0; i < b.N; i++ {
						s := iso.State()
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
