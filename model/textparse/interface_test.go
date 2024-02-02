// Copyright 2022 The Prometheus Authors
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

package textparse

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewParser(t *testing.T) {
	t.Parallel()

	requirePromParser := func(t *testing.T, p Parser) {
		require.NotNil(t, p)
		_, ok := p.(*PromParser)
		require.True(t, ok)
	}

	requireOpenMetricsParser := func(t *testing.T, p Parser) {
		require.NotNil(t, p)
		_, ok := p.(*OpenMetricsParser)
		require.True(t, ok)
	}

	for name, tt := range map[string]*struct {
		contentType    string
		validateParser func(*testing.T, Parser)
		err            string
	}{
		"empty-string": {
			validateParser: requirePromParser,
		},
		"invalid-content-type-1": {
			contentType:    "invalid/",
			validateParser: requirePromParser,
			err:            "expected token after slash",
		},
		"invalid-content-type-2": {
			contentType:    "invalid/invalid/invalid",
			validateParser: requirePromParser,
			err:            "unexpected content after media subtype",
		},
		"invalid-content-type-3": {
			contentType:    "/",
			validateParser: requirePromParser,
			err:            "no media type",
		},
		"invalid-content-type-4": {
			contentType:    "application/openmetrics-text; charset=UTF-8; charset=utf-8",
			validateParser: requirePromParser,
			err:            "duplicate parameter name",
		},
		"openmetrics": {
			contentType:    "application/openmetrics-text",
			validateParser: requireOpenMetricsParser,
		},
		"openmetrics-with-charset": {
			contentType:    "application/openmetrics-text; charset=utf-8",
			validateParser: requireOpenMetricsParser,
		},
		"openmetrics-with-charset-and-version": {
			contentType:    "application/openmetrics-text; version=1.0.0; charset=utf-8",
			validateParser: requireOpenMetricsParser,
		},
		"plain-text": {
			contentType:    "text/plain",
			validateParser: requirePromParser,
		},
		"plain-text-with-version": {
			contentType:    "text/plain; version=0.0.4",
			validateParser: requirePromParser,
		},
		"some-other-valid-content-type": {
			contentType:    "text/html",
			validateParser: requirePromParser,
		},
	} {
		t.Run(name, func(t *testing.T) {
			tt := tt // Copy to local variable before going parallel.
			t.Parallel()

			p, err := New([]byte{}, tt.contentType, false)
			tt.validateParser(t, p)
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.err)
			}
		})
	}
}
