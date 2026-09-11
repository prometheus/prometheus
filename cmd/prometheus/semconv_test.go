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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecideSemconvAction(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		featureEnabled, agentMode, registryConfigured bool
		want                                          semconvAction
	}{
		{"feature off, nothing configured", false, false, false, semconvNoop},
		{"feature off, registry configured", false, false, true, semconvDisabledWarn},
		{"feature off in agent mode, registry configured", false, true, true, semconvDisabledWarn},
		{"feature off in agent mode, nothing configured", false, true, false, semconvNoop},
		{"feature on, nothing configured", true, false, false, semconvEmbeddedRegistry},
		{"feature on, registry configured", true, false, true, semconvOperatorRegistry},
		{"feature on in agent mode, nothing configured", true, true, false, semconvAgentSkip},
		{"feature on in agent mode, registry configured", true, true, true, semconvAgentSkip},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, decideSemconvAction(tc.featureEnabled, tc.agentMode, tc.registryConfigured))
		})
	}
}
