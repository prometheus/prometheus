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
//
// Provenance-includes-location: https://github.com/dolthub/swiss/blob/f4b2babd2bc1cf0a2d66bab4e579ca35b6202338/fastrand_1.22.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright 2023 Dolthub, Inc.

//go:build !go1.22

package tsdb

import (
	_ "unsafe"
)

//go:linkname fastrand runtime.fastrand
func fastrand() uint32

// randIntN returns a random number in the interval [0, n).
func randIntN(n int) uint32 {
	return fastModN(fastrand(), uint32(n))
}
