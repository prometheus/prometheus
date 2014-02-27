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

package storage

// RecordDecoder decodes each key-value pair in the database. The protocol
// around it makes the assumption that the underlying implementation is
// concurrency safe.
type RecordDecoder interface {
	DecodeKey(in interface{}) (out interface{}, err error)
	DecodeValue(in interface{}) (out interface{}, err error)
}

// FilterResult describes the record matching and scanning behavior for the
// database.
type FilterResult int

const (
	// Stop scanning the database.
	Stop FilterResult = iota
	// Skip this record but continue scanning.
	Skip
	// Accept this record for the Operator.
	Accept
)

func (f FilterResult) String() string {
	switch f {
	case Stop:
		return "STOP"
	case Skip:
		return "SKIP"
	case Accept:
		return "ACCEPT"
	}

	panic("unknown")
}

// OperatorError is used for storage operations upon errors that may or may not
// be continuable.
type OperatorError struct {
	Error       error
	Continuable bool
}

// RecordFilter is responsible for controlling the behavior of the database scan
// process and determines the disposition of various records.
//
// The protocol around it makes the assumption that the underlying
// implementation is concurrency safe.
type RecordFilter interface {
	// Filter receives the key and value as decoded from the RecordDecoder type.
	Filter(key, value interface{}) (filterResult FilterResult)
}

// RecordOperator is responsible for taking action upon each entity that is
// passed to it.
//
// The protocol around it makes the assumption that the underlying
// implementation is concurrency safe.
type RecordOperator interface {
	// Take action on a given record. If the action returns an error, the entire
	// scan process stops.
	Operate(key, value interface{}) (err *OperatorError)
}
