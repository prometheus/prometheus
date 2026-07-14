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

package index

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestMemPostingsReadWaiterBlocksNewAdds(t *testing.T) {
	var gate postingsPhaseGate
	gate.init()

	gate.beginAdd()
	var endFirstAdd sync.Once
	endFirst := func() { endFirstAdd.Do(gate.endAdd) }
	t.Cleanup(endFirst)

	readerEntered := make(chan struct{})
	releaseReader := make(chan struct{})
	readerDone := make(chan struct{})
	var releaseReaderOnce sync.Once
	release := func() { releaseReaderOnce.Do(func() { close(releaseReader) }) }
	t.Cleanup(release)

	go func() {
		gate.beginRead()
		close(readerEntered)
		<-releaseReader
		gate.endRead()
		close(readerDone)
	}()

	waitForGateState(t, &gate, func(g *postingsPhaseGate) bool {
		return g.readWaiters == 1
	}, "reader did not register as waiting")

	secondAddStarted := make(chan struct{})
	secondAddEntered := make(chan struct{})
	secondAddDone := make(chan struct{})

	// Start the second Add before releasing the first one.
	gate.mtx.Lock()
	go func() {
		close(secondAddStarted)
		gate.beginAdd()
		close(secondAddEntered)
		gate.endAdd()
		close(secondAddDone)
	}()
	<-secondAddStarted

	firstAddExitStarted := make(chan struct{})
	go func() {
		close(firstAddExitStarted)
		endFirst()
	}()
	<-firstAddExitStarted
	gate.mtx.Unlock()

	select {
	case <-readerEntered:
	case <-secondAddEntered:
		t.Fatal("later Add entered before the waiting reader")
	case <-time.After(time.Second):
		t.Fatal("waiting reader did not enter")
	}

	select {
	case <-secondAddEntered:
		t.Fatal("later Add entered while the reader was active")
	default:
	}

	release()
	select {
	case <-readerDone:
	case <-time.After(time.Second):
		t.Fatal("reader did not exit")
	}

	select {
	case <-secondAddEntered:
	case <-time.After(time.Second):
		t.Fatal("later Add did not enter after the reader exited")
	}

	select {
	case <-secondAddDone:
	case <-time.After(time.Second):
		t.Fatal("later Add did not exit")
	}
}

func waitForGateState(t *testing.T, gate *postingsPhaseGate, condition func(*postingsPhaseGate) bool, failure string) {
	t.Helper()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		gate.mtx.Lock()
		ok := condition(gate)
		gate.mtx.Unlock()
		if ok {
			return
		}

		select {
		case <-timer.C:
			t.Fatal(failure)
		default:
			runtime.Gosched()
		}
	}
}
