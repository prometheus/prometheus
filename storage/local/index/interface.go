// Copyright 2014 The Prometheus Authors
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

package index

import "encoding"

// KeyValueStore persists key/value pairs. Implementations must be fundamentally
// goroutine-safe. However, it is the caller's responsibility that keys and
// values can be safely marshaled and unmarshaled (via the MarshalBinary and
// UnmarshalBinary methods of the keys and values). For example, if you call the
// Put method of a KeyValueStore implementation, but the key or the value are
// modified concurrently while being marshaled into its binary representation,
// you obviously have a problem. Methods of KeyValueStore return only after
// (un)marshaling is complete.
type KeyValueStore interface {
	Put(key, value encoding.BinaryMarshaler) error
	// Get unmarshals the result into value. It returns false if no entry
	// could be found for key. If value is nil, Get behaves like Has.
	Get(key encoding.BinaryMarshaler, value encoding.BinaryUnmarshaler) (bool, error)
	Has(key encoding.BinaryMarshaler) (bool, error)
	// Delete returns (false, nil) if key does not exist.
	Delete(key encoding.BinaryMarshaler) (bool, error)

	NewBatch() Batch
	Commit(b Batch) error

	// ForEach iterates through the complete KeyValueStore and calls the
	// supplied function for each mapping.
	ForEach(func(kv KeyValueAccessor) error) error

	Close() error
}

// KeyValueAccessor allows access to the key and value of an entry in a
// KeyValueStore.
type KeyValueAccessor interface {
	Key(encoding.BinaryUnmarshaler) error
	Value(encoding.BinaryUnmarshaler) error
}

// Batch allows KeyValueStore mutations to be pooled and committed together. An
// implementation does not have to be goroutine-safe. Never modify a Batch
// concurrently or commit the same batch multiple times concurrently. Marshaling
// of keys and values is guaranteed to be complete when the Put or Delete methods
// have returned.
type Batch interface {
	Put(key, value encoding.BinaryMarshaler) error
	Delete(key encoding.BinaryMarshaler) error
	Reset()
}
