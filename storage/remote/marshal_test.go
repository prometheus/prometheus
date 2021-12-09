// Copyright 2013 The Prometheus Authors
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

package remote

import (
	"fmt"
	math_bits "math/bits"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sizeVarint(t *testing.T) {
	for i := 0; i < 63; i++ {
		x := uint64(1) << i
		t.Run(fmt.Sprint(x), func(t *testing.T) {
			want := (math_bits.Len64(x|1) + 6) / 7 // Implementation used by protoc.
			got := sizeVarint(x)
			assert.Equal(t, want, got)
		})
	}
}
