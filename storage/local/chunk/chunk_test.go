// Copyright 2016 The Prometheus Authors
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

// Note: this file has tests for code in both delta.go and doubledelta.go --
// it may make sense to split those out later, but given that the tests are
// near-identical and share a helper, this feels simpler for now.

package chunk

import (
	"testing"

	"github.com/prometheus/common/model"
)

func TestLen(t *testing.T) {
	chunks := []Chunk{}
	for _, encoding := range []Encoding{Delta, DoubleDelta, Varbit} {
		c, err := NewForEncoding(encoding)
		if err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, c)
	}

	for _, c := range chunks {
		for i := 0; i <= 10; i++ {
			if c.Len() != i {
				t.Errorf("chunk type %s should have %d samples, had %d", c.Encoding(), i, c.Len())
			}

			cs, _ := c.Add(model.SamplePair{
				Timestamp: model.Time(i),
				Value:     model.SampleValue(i),
			})
			c = cs[0]
		}
	}
}
