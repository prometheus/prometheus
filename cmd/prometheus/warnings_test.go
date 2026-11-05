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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeWarningLogger struct {
	logs []string
}

func (fl *fakeWarningLogger) HandleWarningHeaderWithContext(_ context.Context, _ int, _, message string) {
	fl.logs = append(fl.logs, message)
}

func TestDedupeDeprecationWarningLogger(t *testing.T) {
	wl := dedupDeprecationWarningLogger{
		logger: &fakeWarningLogger{},
		logged: make(map[string]struct{}),
	}

	deprecationMessage := "v1 Endpoints is deprecated in v1.33+; use [discovery.k8s.io/v1](http://discovery.k8s.io/v1) EndpointSlice"
	for range 10 {
		wl.HandleWarningHeaderWithContext(context.Background(), 299, "", deprecationMessage)
	}
	require.Len(t, wl.logger.(*fakeWarningLogger).logs, 1)
	require.Len(t, wl.logged, 1)
	require.Equal(t, wl.logger.(*fakeWarningLogger).logs[0], deprecationMessage)

	for i := range 10 {
		wl.HandleWarningHeaderWithContext(context.Background(), 299, "", fmt.Sprintf("some other warning %d", i+1))
	}
	require.Len(t, wl.logger.(*fakeWarningLogger).logs, 11)
	require.Len(t, wl.logged, 1)
	require.Equal(t, "some other warning 10", wl.logger.(*fakeWarningLogger).logs[10])
}
