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

package chunkenc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkXorRead(b *testing.B) {
	c := NewXORChunk()
	app, err := c.Appender()
	require.NoError(b, err)
	for i := int64(0); i < 120*1000; i += 1000 {
		app.Append(0, i, float64(i)+float64(i)/10+float64(i)/100+float64(i)/1000)
	}

	b.ReportAllocs()

	var it Iterator
	for b.Loop() {
		var ts int64
		var v float64
		it = c.Iterator(it)
		for it.Next() != ValNone {
			ts, v = it.At()
		}
		_, _ = ts, v
	}
}
