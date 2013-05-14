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
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"net/http"
	"sync"
)

type DatabasesHandler struct {
	States []leveldb.DatabaseState

	Incoming chan []leveldb.DatabaseState

	mutex sync.RWMutex
}

func (h *DatabasesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	select {
	case states := <-h.Incoming:
		h.mutex.Lock()
		defer h.mutex.Unlock()
		h.States = states
	default:
		h.mutex.RLock()
		defer h.mutex.RUnlock()
	}

	executeTemplate(w, "databases", h)
}
