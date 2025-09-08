// Copyright 2024 Prometheus Team
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

package writev2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestSymbolsTable(t *testing.T) {
	s := NewSymbolTable()
	require.Equal(t, []string{""}, s.Symbols(), "required empty reference does not exist")
	require.Equal(t, uint32(0), s.Symbolize(""))
	require.Equal(t, []string{""}, s.Symbols())

	require.Equal(t, uint32(1), s.Symbolize("abc"))
	require.Equal(t, []string{"", "abc"}, s.Symbols())

	require.Equal(t, uint32(2), s.Symbolize("__name__"))
	require.Equal(t, []string{"", "abc", "__name__"}, s.Symbols())

	require.Equal(t, uint32(3), s.Symbolize("foo"))
	require.Equal(t, []string{"", "abc", "__name__", "foo"}, s.Symbols())

	s.Reset()
	require.Equal(t, []string{""}, s.Symbols(), "required empty reference does not exist")
	require.Equal(t, uint32(0), s.Symbolize(""))

	require.Equal(t, uint32(1), s.Symbolize("__name__"))
	require.Equal(t, []string{"", "__name__"}, s.Symbols())

	require.Equal(t, uint32(2), s.Symbolize("abc"))
	require.Equal(t, []string{"", "__name__", "abc"}, s.Symbols())

	ls := labels.FromStrings("__name__", "qwer", "zxcv", "1234")
	encoded := s.SymbolizeLabels(ls, nil)
	require.Equal(t, []uint32{1, 3, 4, 5}, encoded)
	b := labels.NewScratchBuilder(len(encoded))
	decoded, err := desymbolizeLabels(&b, encoded, s.Symbols())
	require.NoError(t, err)
	require.Equal(t, ls, decoded)

	// Different buf.
	ls = labels.FromStrings("__name__", "qwer", "zxcv2222", "1234")
	encoded = s.SymbolizeLabels(ls, []uint32{1, 3, 4, 5})
	require.Equal(t, []uint32{1, 3, 6, 5}, encoded)
}
