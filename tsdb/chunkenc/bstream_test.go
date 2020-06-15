// Copyright 2017 The Prometheus Authors
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

// The code in this file was largely written by Damian Gryski as part of
// https://github.com/dgryski/go-tsz and published under the license below.
// It received minor modifications to suit Prometheus's needs.

// Copyright (c) 2015,2016 Damian Gryski <damian@gryski.com>
// All rights reserved.

// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:

// * Redistributions of source code must retain the above copyright notice,
// this list of conditions and the following disclaimer.
//
// * Redistributions in binary form must reproduce the above copyright notice,
// this list of conditions and the following disclaimer in the documentation
// and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package chunkenc

import (
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestBstreamReader(t *testing.T) {
	// Write to the bit stream.
	w := bstream{}
	for _, bit := range []bit{true, false} {
		w.writeBit(bit)
	}
	for nbits := 1; nbits <= 64; nbits++ {
		w.writeBits(uint64(nbits), nbits)
	}
	for v := 1; v < 10000; v += 123 {
		w.writeBits(uint64(v), 29)
	}

	// Read back.
	r := newBReader(w.bytes())
	for _, bit := range []bit{true, false} {
		v, err := r.readBitFast()
		if err != nil {
			v, err = r.readBit()
		}
		testutil.Ok(t, err)
		testutil.Equals(t, bit, v)
	}
	for nbits := uint8(1); nbits <= 64; nbits++ {
		v, err := r.readBitsFast(nbits)
		if err != nil {
			v, err = r.readBits(nbits)
		}
		testutil.Ok(t, err)
		testutil.Equals(t, uint64(nbits), v, "nbits=%d", nbits)
	}
	for v := 1; v < 10000; v += 123 {
		actual, err := r.readBitsFast(29)
		if err != nil {
			actual, err = r.readBits(29)
		}
		testutil.Ok(t, err)
		testutil.Equals(t, uint64(v), actual, "v=%d", v)
	}
}
