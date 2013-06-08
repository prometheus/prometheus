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

// Persistence models a key-value store for bytes that supports various
// additional operations.
type Persistence interface {
	// Close reaps all of the underlying system resources associated with this
	// persistence.
	Close()
	// Has informs the user whether a given key exists in the database.
	Has(key proto.Message) (bool, error)
	// Get retrieves the key from the database if it exists or returns nil if
	// it is absent.
	Get(key, value proto.Message) (present bool, err error)
	// Drop removes the key from the database.
	Drop(key proto.Message) error
	// Put sets the key to a given value.
	Put(key, value proto.Message) error
	// ForEach is responsible for iterating through all records in the database
	// until one of the following conditions are met:
	//
	// 1.) A system anomaly in the database scan.
	// 2.) The last record in the database is reached.
	// 3.) A FilterResult of STOP is emitted by the Filter.
	//
	// Decoding errors for an entity cause that entity to be skipped.
	ForEach(storage.RecordDecoder, storage.RecordFilter, storage.RecordOperator) (scannedEntireCorpus bool, err error)
	// Commit applies the Batch operations to the database.
	Commit(Batch) error
}

// Batch models a pool of mutations for the database that can be committed
// en masse.  The interface implies no protocol around the atomicity of
// effectuation.
type Batch interface {
	// Close reaps all of the underlying system resources associated with this
	// batch mutation.
	Close()
	// Put follows the same protocol as Persistence.Put.
	Put(key, value proto.Message)
	// Drop follows the same protocol as Persistence.Drop.
	Drop(key proto.Message)
}
