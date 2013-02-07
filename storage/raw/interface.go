// Copyright 2012 Prometheus Team
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
	"github.com/prometheus/prometheus/coding"
	"github.com/prometheus/prometheus/storage"
)

type Pair struct {
	Left  []byte
	Right []byte
}

type EachFunc func(pair *Pair)

type Persistence interface {
	Has(key coding.Encoder) (bool, error)
	Get(key coding.Encoder) ([]byte, error)
	Drop(key coding.Encoder) error
	Put(key, value coding.Encoder) error
	Close() error

	// ForEach is responsible for iterating through all records in the database
	// until one of the following conditions are met:
	//
	// 1.) A system anomaly in the database scan.
	// 2.) The last record in the database is reached.
	// 3.) A FilterResult of STOP is emitted by the Filter.
	//
	// Decoding errors for an entity cause that entity to be skipped.
	ForEach(decoder storage.RecordDecoder, filter storage.RecordFilter, operator storage.RecordOperator) (scannedEntireCorpus bool, err error)

	// Pending removal.
	GetAll() ([]Pair, error)
}
