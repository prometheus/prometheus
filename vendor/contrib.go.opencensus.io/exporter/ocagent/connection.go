// Copyright 2018, OpenCensus Authors
//
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

package ocagent

import (
	"math/rand"
	"sync/atomic"
	"time"
	"unsafe"
)

func (ae *Exporter) lastConnectError() error {
	errPtr := (*error)(atomic.LoadPointer(&ae.lastConnectErrPtr))
	if errPtr == nil {
		return nil
	}
	return *errPtr
}

func (ae *Exporter) saveLastConnectError(err error) {
	var errPtr *error
	if err != nil {
		errPtr = &err
	}
	atomic.StorePointer(&ae.lastConnectErrPtr, unsafe.Pointer(errPtr))
}

func (ae *Exporter) setStateDisconnected(err error) {
	ae.saveLastConnectError(err)
	select {
	case ae.disconnectedCh <- true:
	default:
	}
}

func (ae *Exporter) setStateConnected() {
	ae.saveLastConnectError(nil)
}

func (ae *Exporter) connected() bool {
	return ae.lastConnectError() == nil
}

const defaultConnReattemptPeriod = 10 * time.Second

func (ae *Exporter) indefiniteBackgroundConnection() error {
	defer func() {
		ae.backgroundConnectionDoneCh <- true
	}()

	connReattemptPeriod := ae.reconnectionPeriod
	if connReattemptPeriod <= 0 {
		connReattemptPeriod = defaultConnReattemptPeriod
	}

	// No strong seeding required, nano time can
	// already help with pseudo uniqueness.
	rng := rand.New(rand.NewSource(time.Now().UnixNano() + rand.Int63n(1024)))

	// maxJitter: 1 + (70% of the connectionReattemptPeriod)
	maxJitter := int64(1 + 0.7*float64(connReattemptPeriod))

	for {
		// Otherwise these will be the normal scenarios to enable
		// reconnections if we trip out.
		// 1. If we've stopped, return entirely
		// 2. Otherwise block until we are disconnected, and
		//    then retry connecting
		select {
		case <-ae.stopCh:
			return errStopped

		case <-ae.disconnectedCh:
			// Normal scenario that we'll wait for
		}

		if err := ae.connect(); err == nil {
			ae.setStateConnected()
		} else {
			ae.setStateDisconnected(err)
		}

		// Apply some jitter to avoid lockstep retrials of other
		// agent-exporters. Lockstep retrials could result in an
		// innocent DDOS, by clogging the machine's resources and network.
		jitter := time.Duration(rng.Int63n(maxJitter))
		select {
		case <-ae.stopCh:
			return errStopped
		case <-time.After(connReattemptPeriod + jitter):
		}
	}
}

func (ae *Exporter) connect() error {
	cc, err := ae.dialToAgent()
	if err != nil {
		return err
	}
	return ae.enableConnectionStreams(cc)
}
