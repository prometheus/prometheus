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
	"sync"
)

// isolationState holds the isolation information.
type isolationState struct {
	// We will ignore all appends above the max, or that are incomplete.
	maxAppendID       uint64
	incompleteAppends map[uint64]struct{}
	lowWatermark      uint64 // Lowest of incompleteAppends/maxAppendID.
	isolation         *isolation

	// Doubly linked list of active reads.
	next *isolationState
	prev *isolationState
}

// Close closes the state.
func (i *isolationState) Close() {
	i.isolation.readMtx.Lock()
	defer i.isolation.readMtx.Unlock()
	i.next.prev = i.prev
	i.prev.next = i.next
}

// isolation is the global isolation state.
type isolation struct {
	// Mutex for accessing lastAppendID and appendsOpen.
	appendMtx sync.Mutex
	// Each append is given an internal id.
	lastAppendID uint64
	// Which appends are currently in progress.
	appendsOpen map[uint64]struct{}
	// Mutex for accessing readsOpen.
	// If taking both appendMtx and readMtx, take appendMtx first.
	readMtx sync.Mutex
	// All current in use isolationStates. This is a doubly-linked list.
	readsOpen *isolationState
}

func newIsolation() *isolation {
	isoState := &isolationState{}
	isoState.next = isoState
	isoState.prev = isoState

	return &isolation{
		appendsOpen: map[uint64]struct{}{},
		readsOpen:   isoState,
	}
}

// lowWatermark returns the appendID below which we no longer need to track
// which appends were from which appendID.
func (i *isolation) lowWatermark() uint64 {
	i.appendMtx.Lock() // Take appendMtx first.
	defer i.appendMtx.Unlock()
	i.readMtx.Lock()
	defer i.readMtx.Unlock()
	if i.readsOpen.prev != i.readsOpen {
		return i.readsOpen.prev.lowWatermark
	}
	lw := i.lastAppendID
	for k := range i.appendsOpen {
		if k < lw {
			lw = k
		}
	}
	return lw
}

// State returns an object used to control isolation
// between a query and appends. Must be closed when complete.
func (i *isolation) State() *isolationState {
	i.appendMtx.Lock() // Take append mutex before read mutex.
	defer i.appendMtx.Unlock()
	isoState := &isolationState{
		maxAppendID:       i.lastAppendID,
		lowWatermark:      i.lastAppendID,
		incompleteAppends: make(map[uint64]struct{}, len(i.appendsOpen)),
		isolation:         i,
	}
	for k := range i.appendsOpen {
		isoState.incompleteAppends[k] = struct{}{}
		if k < isoState.lowWatermark {
			isoState.lowWatermark = k
		}
	}

	i.readMtx.Lock()
	defer i.readMtx.Unlock()
	isoState.prev = i.readsOpen
	isoState.next = i.readsOpen.next
	i.readsOpen.next.prev = isoState
	i.readsOpen.next = isoState
	return isoState
}

// newAppendID increments the transaction counter and returns a new transaction
// ID. The first ID returned is 1.
func (i *isolation) newAppendID() uint64 {
	i.appendMtx.Lock()
	defer i.appendMtx.Unlock()
	i.lastAppendID++
	i.appendsOpen[i.lastAppendID] = struct{}{}
	return i.lastAppendID
}

func (i *isolation) closeAppend(appendID uint64) {
	i.appendMtx.Lock()
	defer i.appendMtx.Unlock()
	delete(i.appendsOpen, appendID)
}

// The transactionID ring buffer.
type txRing struct {
	txIDs     []uint64
	txIDFirst int // Position of the first id in the ring.
	txIDCount int // How many ids in the ring.
}

func newTxRing(cap int) *txRing {
	return &txRing{
		txIDs: make([]uint64, cap),
	}
}

func (txr *txRing) add(appendID uint64) {
	if txr.txIDCount == len(txr.txIDs) {
		// Ring buffer is full, expand by doubling.
		newRing := make([]uint64, txr.txIDCount*2)
		idx := copy(newRing[:], txr.txIDs[txr.txIDFirst:])
		copy(newRing[idx:], txr.txIDs[:txr.txIDFirst])
		txr.txIDs = newRing
		txr.txIDFirst = 0
	}

	txr.txIDs[(txr.txIDFirst+txr.txIDCount)%len(txr.txIDs)] = appendID
	txr.txIDCount++
}

func (txr *txRing) cleanupAppendIDsBelow(bound uint64) {
	pos := txr.txIDFirst

	for txr.txIDCount > 0 {
		if txr.txIDs[pos] < bound {
			txr.txIDFirst++
			txr.txIDCount--
		} else {
			break
		}

		pos++
		if pos == len(txr.txIDs) {
			pos = 0
		}
	}

	txr.txIDFirst %= len(txr.txIDs)
}

func (txr *txRing) iterator() *txRingIterator {
	return &txRingIterator{
		pos: txr.txIDFirst,
		ids: txr.txIDs,
	}
}

// txRingIterator lets you iterate over the ring. It doesn't terminate,
// it DOESN'T terminate.
type txRingIterator struct {
	ids []uint64

	pos int
}

func (it *txRingIterator) At() uint64 {
	return it.ids[it.pos]
}

func (it *txRingIterator) Next() {
	it.pos++
	if it.pos == len(it.ids) {
		it.pos = 0
	}
}
