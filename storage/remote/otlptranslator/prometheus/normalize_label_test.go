// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package normalize

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeDropSanitization(t *testing.T) {
	require.Equal(t, "", NormalizeLabel(""))
	require.Equal(t, "_test", NormalizeLabel("_test"))
	require.Equal(t, "key_0test", NormalizeLabel("0test"))
	require.Equal(t, "test", NormalizeLabel("test"))
	require.Equal(t, "test__", NormalizeLabel("test_/"))
	require.Equal(t, "__test", NormalizeLabel("__test"))
}
