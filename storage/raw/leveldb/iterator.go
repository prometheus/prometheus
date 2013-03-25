// Copyright 2013 Prometheus Team
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

package leveldb

// TODO: Evaluate whether to use coding.Encoder for the key and values instead
//       raw bytes for consistency reasons.

// Iterator wraps Levigo and LevelDB's iterator behaviors in a manner that is
// conducive to IO-free testing.
//
// It borrows some of the operational assumptions from goskiplist, which
// functions very similarly, in that it uses no separate Valid method to
// determine health.  All methods that have a return signature of (ok bool)
// assume in the real LevelDB case that if ok == false that the iterator
// must be disposed of at this given instance and recreated if future
// work is desired.  This is a quirk of LevelDB itself!
type Iterator interface {
	// GetError reports low-level errors, if available.  This should not indicate
	// that the iterator is necessarily unhealthy but maybe that the underlying
	// table is corrupted itself.  See the notes above for (ok bool) return
	// signatures to determine iterator health.
	GetError() error
	Key() []byte
	Next() (ok bool)
	Previous() (ok bool)
	Seek(key []byte) (ok bool)
	SeekToFirst() (ok bool)
	SeekToLast() (ok bool)
	Value() []byte
}
