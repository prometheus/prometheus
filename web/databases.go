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

package web

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/prometheus/storage/raw"
)

type DatabaseStatesProvider interface {
	States() raw.DatabaseStates
}

type DatabasesHandler struct {
	RefreshInterval time.Duration
	NextRefresh     time.Time

	Current raw.DatabaseStates

	Provider DatabaseStatesProvider

	mutex sync.RWMutex
}

func (h *DatabasesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Refresh()

	h.mutex.RLock()
	defer h.mutex.RUnlock()
	executeTemplate(w, "databases", h)
}

func (h *DatabasesHandler) Refresh() {
	h.mutex.RLock()
	if !time.Now().After(h.NextRefresh) {
		h.mutex.RUnlock()
		return
	}
	h.mutex.RUnlock()

	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.Current = h.Provider.States()
	h.NextRefresh = time.Now().Add(h.RefreshInterval)
}
