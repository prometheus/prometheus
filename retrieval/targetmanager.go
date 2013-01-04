// Copyright 2013 Prometheus Team
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

package retrieval

import (
	"container/heap"
	"log"
	"time"
)

type TargetManager interface {
	acquire()
	release()
	Add(t *Target)
	Remove(t *Target)
}

type targetManager struct {
	requestAllowance chan bool
	pools            map[time.Duration]TargetPool
	results          chan Result
}

func NewTargetManager(results chan Result, requestAllowance int) TargetManager {
	return targetManager{
		requestAllowance: make(chan bool, requestAllowance),
		results:          results,
		pools:            make(map[time.Duration]TargetPool),
	}
}

func (m targetManager) acquire() {
	m.requestAllowance <- true
}

func (m targetManager) release() {
	<-m.requestAllowance
}

func (m targetManager) Add(t *Target) {
	targetPool, ok := m.pools[t.Interval]

	if !ok {
		targetPool.manager = m
		log.Printf("Pool %s does not exist; creating and starting...", t.Interval)
		go targetPool.Run(m.results, t.Interval)
	}

	heap.Push(&targetPool, t)
	m.pools[t.Interval] = targetPool
}

func (m targetManager) Remove(t *Target) {
	panic("not implemented")
}
