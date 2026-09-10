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

package main

import (
	"context"
	"sync"

	"github.com/grafana/regexp"
	"k8s.io/client-go/rest"
)

// deprecationRegex matches the format of Kubernetes API deprecation warnings:
// See https://github.com/kubernetes/kubernetes/blob/da663405beb487d66c27a0220ea4073305ae9077/staging/src/k8s.io/apiserver/pkg/endpoints/deprecation/deprecation.go#L117.
var deprecationRegex = regexp.MustCompile(`\S+ \S+ is deprecated in v\d+\.\d+\+`)

// Even though deprecation warnings should be bounded in number, this safeguard should help prevent leaks.
const maxDeprecationWarnings = 32

// dedupDeprecationWarningLogger deduplicates Kube API deprecation warnings by message before logging them.
// Inspired by https://github.com/kubernetes/kubernetes/blob/3edae6c1c49958fd10a708d9cc8c4c9e7f5fb6e8/staging/src/k8s.io/client-go/rest/warnings.go#L113
type dedupDeprecationWarningLogger struct {
	logger rest.WarningHandlerWithContext
	lock   sync.Mutex
	logged map[string]struct{}
}

func newDedupDeprecationWarningLogger() *dedupDeprecationWarningLogger {
	return &dedupDeprecationWarningLogger{
		logger: rest.WarningLogger{},
		logged: make(map[string]struct{}),
	}
}

func (w *dedupDeprecationWarningLogger) HandleWarningHeaderWithContext(ctx context.Context, code int, agent, message string) {
	if code != 299 || message == "" {
		return
	}

	w.lock.Lock()
	defer w.lock.Unlock()

	if _, seen := w.logged[message]; seen {
		return
	}

	if deprecationRegex.MatchString(message) && len(w.logged) < maxDeprecationWarnings {
		w.logged[message] = struct{}{}
	}

	w.logger.HandleWarningHeaderWithContext(ctx, code, agent, message)
}
