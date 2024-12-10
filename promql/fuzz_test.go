// Copyright 2022 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Only build when go-fuzz is in use
//go:build gofuzz

package promql

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestfuzzParseMetricWithContentTypePanicOnInvalid(t *testing.T) {
	defer func() {
		if p := recover(); p == nil {
			t.Error("invalid content type should panic")
		} else {
			err, ok := p.(error)
			require.True(t, ok)
			require.ErrorContains(t, err, "duplicate parameter name")
		}
	}()

	const invalidContentType = "application/openmetrics-text; charset=UTF-8; charset=utf-8"
	fuzzParseMetricWithContentType([]byte{}, invalidContentType)
}
