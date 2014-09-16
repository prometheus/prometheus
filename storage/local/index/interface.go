package index

import "encoding"

// KeyValueStore persists key/value pairs. Implementations must be fundamentally
// goroutine-safe. However, it is the caller's responsibility that keys and
// values can be safely marshaled and unmarshaled (via the MarshalBinary and
// UnmarshalBinary methods of the keys and values). For example, if you call the
// Put method of a KeyValueStore implementation, but the key or the value are
// modified concurrently while being marshaled into its binary representation,
// you obviously have a problem. Methods of KeyValueStore only return after
// (un)marshaling is complete.
type KeyValueStore interface {
	Put(key, value encoding.BinaryMarshaler) error
	// Get unmarshals the result into value. It returns false if no entry
	// could be found for key. If value is nil, Get behaves like Has.
	Get(key encoding.BinaryMarshaler, value encoding.BinaryUnmarshaler) (bool, error)
	Has(key encoding.BinaryMarshaler) (bool, error)
	// Delete returns an error if key does not exist.
	Delete(key encoding.BinaryMarshaler) error

	NewBatch() Batch
	Commit(b Batch) error

	Close() error
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
