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

package chunk

import "testing"

func TestCountBits(t *testing.T) {
	for i := byte(0); i < 56; i++ {
		for j := byte(0); j <= 8; j++ {
			for k := byte(0); k < 8; k++ {
				p := uint64(bitMask[j][k]) << i
				gotLeading, gotSignificant := countBits(p)
				wantLeading := 56 - i + k
				wantSignificant := j
				if j+k > 8 {
					wantSignificant -= j + k - 8
				}
				if wantLeading > 31 {
					wantSignificant += wantLeading - 31
					wantLeading = 31
				}
				if p == 0 {
					wantLeading = 0
					wantSignificant = 0
				}
				if wantLeading != gotLeading {
					t.Errorf(
						"unexpected leading bit count for i=%d, j=%d, k=%d; want %d, got %d",
						i, j, k, wantLeading, gotLeading,
					)
				}
				if wantSignificant != gotSignificant {
					t.Errorf(
						"unexpected significant bit count for i=%d, j=%d, k=%d; want %d, got %d",
						i, j, k, wantSignificant, gotSignificant,
					)
				}
			}
		}
	}
}
