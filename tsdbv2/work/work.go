// Copyright 2024 The Prometheus Authors
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

package work

import (
	"context"
	"runtime"
	"sync"
)

// Work breakdown slice into chunks that are processed concurrently across available CPU.
// Blocks until all tasks are completed.
func Work[T any](ctx context.Context, slice []T, do func(ctx context.Context, value []T)) {
	N := runtime.NumCPU()
	var wg sync.WaitGroup
	size := len(slice) / N
	remainder := len(slice) % N
	var start, end int
	for range N {
		start, end = end, end+size
		if remainder > 0 {
			remainder--
			end++
		}
		chunk := slice[start:end:end]
		if len(chunk) == 0 {
			// avoid sending empty slices to workes. It is good practice for
			// workers  to check this on site. But it is  a waste to spin a goroutine
			// here.
			continue
		}
		wg.Add(1)

		go func(ls []T) {
			defer wg.Done()
			do(ctx, ls)
		}(chunk)
	}
	wg.Wait()
}
