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

package raw

import (
	"code.google.com/p/goprotobuf/proto"

	"github.com/prometheus/prometheus/storage"
)

// Database provides a few very basic methods to manage a database and inquire
// its state.
type Database interface {
	// Close reaps all of the underlying system resources associated with
	// this database. For databases that don't need that kind of clean-up,
	// it is implemented as a no-op (so that clients don't need to reason
	// and always call Close 'just in case').
	Close() error
	// State reports the state of the database as a DatabaseState object.
	State() *DatabaseState
	// Size returns the total size of the database in bytes. The number may
	// be an approximation, depending on the underlying database type.
	Size() (uint64, error)
}

// ForEacher is implemented by databases that can be iterated through.
type ForEacher interface {
	// ForEach is responsible for iterating through all records in the
	// database until one of the following conditions are met:
	//
	// 1.) A system anomaly in the database scan.
	// 2.) The last record in the database is reached.
	// 3.) A FilterResult of STOP is emitted by the Filter.
	//
	// Decoding errors for an entity cause that entity to be skipped.
	ForEach(storage.RecordDecoder, storage.RecordFilter, storage.RecordOperator) (scannedEntireCorpus bool, err error)
}

// Pruner is implemented by a database that can be pruned in some way.
type Pruner interface {
	Prune()
}

// Persistence models a key-value store for bytes that supports various
// additional operations.
type Persistence interface {
	Database
	ForEacher

	// Has informs the user whether a given key exists in the database.
	Has(key proto.Message) (bool, error)
	// Get populates 'value' with the value of 'key', if present, in which
	// case 'present' is returned as true.
	Get(key, value proto.Message) (present bool, err error)
	// Drop removes the key from the database.
	Drop(key proto.Message) error
	// Put sets the key to a given value.
	Put(key, value proto.Message) error
	// PutRaw sets the key to a given raw bytes value.
	PutRaw(key proto.Message, value []byte) error
	// Commit applies the Batch operations to the database.
	Commit(Batch) error
}

// Batch models a pool of mutations for the database that can be committed
// en masse.  The interface implies no protocol around the atomicity of
// effectuation.
type Batch interface {
	// Close reaps all of the underlying system resources associated with
	// this batch mutation.
	Close()
	// Put follows the same protocol as Persistence.Put.
	Put(key, value proto.Message)
	// PutRaw follows the same protocol as Persistence.PutRaw.
	PutRaw(key proto.Message, value []byte)
	// Drop follows the same protocol as Persistence.Drop.
	Drop(key proto.Message)
}
