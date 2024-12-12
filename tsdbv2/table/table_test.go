// Copyright 2024 The Prometheus Authors
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

package table

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func ExampleFormat() {
	source := [][]string{
		{"base", "name", "value", "bit"},
		{"fields.Labels", "foo", "bar", "1"},
		{"fields.Labels", "foo", "baz", "2"},
		{"fields.Labels", "alpha", "baz", "1"},
	}
	fmt.Println(Format(source...))
	// Output:
	// base          name  value bit
	// fields.Labels foo   bar   1
	// fields.Labels foo   baz   2
	// fields.Labels alpha baz   1
}

func TestFormatTable(t *testing.T) {
	source := [][]string{
		{"base", "name", "value", "bit"},
		{"fields.Labels", "foo", "bar", "1"},
		{"fields.Labels", "foo", "baz", "2"},
		{"fields.Labels", "alpha", "baz", "1"},
	}
	check(t, "testdata/format.txt", Format(source...))
	check(t, "testdata/format_single.txt", SingleLineComment(source...))
	check(t, "testdata/format_multi.txt", MultiLineComment(source...))
	check(t, "testdata/format_example.txt", Example(source...))
}

func check(t *testing.T, name, got string) {
	t.Helper()
	want, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
