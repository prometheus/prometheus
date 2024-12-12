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

package hash

import (
	"encoding/binary"

	"github.com/cespare/xxhash/v2"
)

func String(s string) uint64 {
	return xxhash.Sum64String(s)
}

func Bytes(s []byte) uint64 {
	return xxhash.Sum64(s)
}

type U64 struct {
	xxhash.Digest
	o [8]byte
}

func New(base uint64) *U64 {
	h := new(U64)
	binary.BigEndian.PutUint64(h.o[:], base)
	_, _ = h.Write(h.o[:])
	return h
}

func (u *U64) Text(s string) uint64 {
	_, _ = u.Write(u.o[:])
	_, _ = u.WriteString(s)
	return u.Sum64()
}
