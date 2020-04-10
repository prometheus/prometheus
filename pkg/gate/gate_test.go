// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gate

import (
	"context"
	"github.com/prometheus/prometheus/util/testutil"
	"sync"
	"testing"
	"time"
)

func TestLabels_IsOverload(t *testing.T) {

	maxGoRoutine := 3
	gate := New(maxGoRoutine)
	callCount := 10
	overloadChan := make(chan int, callCount-maxGoRoutine)
	var wg sync.WaitGroup

	for i := 0; i < callCount; i++ {
		go func() {
			wg.Add(1)
			defer wg.Done()
			if gate.IsOverload() {
				overloadChan <- 1
				return
			}
			if err := gate.Start(context.Background()); err != nil {

			}
			time.Sleep(2 * time.Second)
			gate.Done()
		}()
	}

	wg.Wait()
	testutil.Equals(t, len(overloadChan), callCount-maxGoRoutine)

}
