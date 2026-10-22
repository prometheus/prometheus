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

package nativemetadata

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/metadata"
)

func TestInterner(t *testing.T) {
	i := NewInterner(2)
	a := i.Intern(*value("a"))
	require.Equal(t, *value("a"), *a)
	require.Same(t, a, i.Intern(*value("a")))
	b := i.Intern(*value("b"))
	require.Same(t, b, i.Intern(*value("b")))
	// Beyond the limit, new values are equal but not shared.
	c := i.Intern(*value("c"))
	require.Equal(t, *value("c"), *c)
	require.NotSame(t, c, i.Intern(*value("c")))
	require.Same(t, a, i.Intern(*value("a")))

	// Reducers call through a function value; hits must not allocate.
	intern := i.Intern
	m := *value("a")
	var got *metadata.Metadata
	require.Zero(t, testing.AllocsPerRun(100, func() { got = intern(m) }))
	require.Same(t, a, got)
}
