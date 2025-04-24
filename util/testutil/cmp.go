// Copyright 2023 The Prometheus Authors
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

package testutil

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

// RequireEqual is a replacement for require.Equal using go-cmp adapted for
// Prometheus data structures, instead of DeepEqual.
func RequireEqual(t testing.TB, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	RequireEqualWithOptions(t, expected, actual, nil, msgAndArgs...)
}

// RequireEqualWithOptions works like RequireEqual but allows extra cmp.Options.
func RequireEqualWithOptions(t testing.TB, expected, actual interface{}, extra []cmp.Option, msgAndArgs ...interface{}) {
	t.Helper()
	options := append([]cmp.Option{cmp.Comparer(labels.Equal)}, extra...)
	if cmp.Equal(expected, actual, options...) {
		return
	}
	diff := cmp.Diff(expected, actual, options...)
	require.Fail(t, fmt.Sprintf("Not equal: \n"+
		"expected: %s\n"+
		"actual  : %s%s", expected, actual, diff), msgAndArgs...)
}
