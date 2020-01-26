// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build psdebug

package pubsub

import (
	"sync"
	"time"
)

var (
	dmu          sync.Mutex
	msgTraces    = map[string][]Event{}
	ackIDToMsgID = map[string]string{}
)

type Event struct {
	Desc string
	At   time.Time
}

func MessageEvents(msgID string) []Event {
	dmu.Lock()
	defer dmu.Unlock()
	return msgTraces[msgID]
}

func addRecv(msgID, ackID string, t time.Time) {
	dmu.Lock()
	defer dmu.Unlock()
	ackIDToMsgID[ackID] = msgID
	addEvent(msgID, "recv", t)
}

func addAcks(ackIDs []string) {
	dmu.Lock()
	defer dmu.Unlock()
	now := time.Now()
	for _, id := range ackIDs {
		addEvent(ackIDToMsgID[id], "ack", now)
	}
}

func addModAcks(ackIDs []string, deadlineSecs int32) {
	dmu.Lock()
	defer dmu.Unlock()
	desc := "modack"
	if deadlineSecs == 0 {
		desc = "nack"
	}
	now := time.Now()
	for _, id := range ackIDs {
		addEvent(ackIDToMsgID[id], desc, now)
	}
}

func addEvent(msgID, desc string, t time.Time) {
	msgTraces[msgID] = append(msgTraces[msgID], Event{desc, t})
}
