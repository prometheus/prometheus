// Copyright 2017 The Prometheus Authors
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

package storage

import (
	"sync"
	"time"

	"github.com/prometheus/common/model"
)

// Maximum timeout of the remote storage call
var (
	MaximumTimeout     model.Duration
	RemoteStorageCount int
)

type AsyncStatus struct {
	mtx       sync.RWMutex
	waitGroup map[string]*sync.WaitGroup
}

var asyncStatus = &AsyncStatus{
	waitGroup: map[string]*sync.WaitGroup{},
}

// IncAsyncStatus increase the wait group
func IncAsyncStatus(ruleName string) {
	if len(ruleName) >= 0 {
		asyncStatus.mtx.Lock()
		defer asyncStatus.mtx.Unlock()
		wg, ok := asyncStatus.waitGroup[ruleName]
		if !ok {
			wg = &sync.WaitGroup{}
		}
		wg.Add(RemoteStorageCount)
		asyncStatus.waitGroup[ruleName] = wg
	}
}

// DescAsyncStatus decrease the wait group
func DescAsyncStatus(ruleName string) {
	if len(ruleName) >= 0 {
		asyncStatus.mtx.Lock()
		defer asyncStatus.mtx.Unlock()
		if wg, ok := asyncStatus.waitGroup[ruleName]; ok {
			wg.Done()
		}
	}
}

// CommitAsyncStatus wait async remote write completed
func CommitAsyncStatus(ruleName string) {
	if len(ruleName) >= 0 {
		wg := GetAsyncStatusWaitGroup(ruleName)
		if wg != nil {
			if mt, err := time.ParseDuration(MaximumTimeout.String()); err == nil {
				defer delete(asyncStatus.waitGroup, ruleName)
				WaitWithTimeout(wg, mt)
			}
		}
	}
}

// GetAsyncStatusWaitGroup get async status wait group from cache for specific rule
func GetAsyncStatusWaitGroup(ruleName string) *sync.WaitGroup {
	asyncStatus.mtx.RLock()
	defer asyncStatus.mtx.RUnlock()
	if wg, ok := asyncStatus.waitGroup[ruleName]; ok {
		return wg
	}
	return nil
}

// WaitWithTimeout add timeout for wait group
func WaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false
	case <-time.After(timeout):
		return true
	}
}
