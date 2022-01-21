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

func TestNew(t *testing.T) {
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

	requireNil := func(t *testing.T, p Parser) {
		require.Nil(t, p)
	}

	for name, tt := range map[string]*struct {
		contentType    string
		validateParser func(*testing.T, Parser)
		err            string
	}{
		"empty-string": {
			contentType:    "",
			validateParser: requirePromParser,
			err:            "",
		},
		"invalid-content-type": {
			contentType:    "application/openmetrics-text; charset=UTF-8; charset=utf-8",
			validateParser: requireNil,
			err:            "duplicate parameter name",
		},
		"application/openmetrics-text": {
			contentType:    "application/openmetrics-text",
			validateParser: requireOpenMetricsParser,
			err:            "",
		},
		"text/plain": {
			contentType:    "text/plain",
			validateParser: requirePromParser,
			err:            "",
		},
		"some-other-valid-content-type": {
			contentType:    "text/html",
			validateParser: requirePromParser,
			err:            "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			tt := tt // copy to local var before going parallel
			t.Parallel()

			p, err := New([]byte{}, tt.contentType)
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
