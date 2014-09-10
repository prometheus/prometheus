package index

import "encoding"

// KeyValueStore persists key/value pairs.
type KeyValueStore interface {
	Put(key, value encoding.BinaryMarshaler) error
	Get(k encoding.BinaryMarshaler, v encoding.BinaryUnmarshaler) (bool, error)
	Has(k encoding.BinaryMarshaler) (has bool, err error)
	Delete(k encoding.BinaryMarshaler) error

	NewBatch() Batch
	Commit(b Batch) error

	Close() error
}

// Batch allows KeyValueStore mutations to be pooled and committed together.
type Batch interface {
	Put(key, value encoding.BinaryMarshaler) error
	Delete(key encoding.BinaryMarshaler) error
	Reset()
}
