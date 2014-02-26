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

package test

import (
	"code.google.com/p/goprotobuf/proto"

	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/utility/test"
)

const cacheCapacity = 0

type (
	// Pair models a prospective (key, value) double that will be committed
	// to a database.
	Pair interface {
		Get() (key proto.Message, value interface{})
	}

	// Pairs models a list of Pair for disk committing.
	Pairs []Pair

	// Preparer readies a LevelDB store for a given raw state given the
	// fixtures definitions passed into it.
	Preparer interface {
		// Prepare furnishes the database and returns its path along
		// with any encountered anomalies.
		Prepare(namespace string, f FixtureFactory) test.TemporaryDirectory
	}

	// FixtureFactory is an iterator emitting fixture data.
	FixtureFactory interface {
		// HasNext indicates whether the FixtureFactory has more pending
		// fixture data to build.
		HasNext() (has bool)
		// Next emits the next (key, value) double for storage.
		Next() (key proto.Message, value interface{})
	}

	preparer struct {
		tester test.Tester
	}

	cassetteFactory struct {
		index int
		count int
		pairs Pairs
	}
)

func (p preparer) Prepare(n string, f FixtureFactory) (t test.TemporaryDirectory) {
	t = test.NewTemporaryDirectory(n, p.tester)
	persistence, err := leveldb.NewLevelDBPersistence(leveldb.LevelDBOptions{
		Path:           t.Path(),
		CacheSizeBytes: cacheCapacity,
	})
	if err != nil {
		defer t.Close()
		p.tester.Fatal(err)
	}

	defer persistence.Close()

	for f.HasNext() {
		key, value := f.Next()

		switch v := value.(type) {
		case proto.Message:
			err = persistence.Put(key, v)
		case []byte:
			err = persistence.PutRaw(key, v)
		default:
			panic("illegal value type")
		}
		if err != nil {
			defer t.Close()
			p.tester.Fatal(err)
		}
	}

	return
}

// HasNext implements FixtureFactory.
func (f cassetteFactory) HasNext() bool {
	return f.index < f.count
}

// Next implements FixtureFactory.
func (f *cassetteFactory) Next() (key proto.Message, value interface{}) {
	key, value = f.pairs[f.index].Get()

	f.index++

	return
}

// NewPreparer creates a new Preparer for use in testing scenarios.
func NewPreparer(t test.Tester) Preparer {
	return preparer{t}
}

// NewCassetteFactory builds a new FixtureFactory that uses Pairs as the basis
// for generated fixture data.
func NewCassetteFactory(pairs Pairs) FixtureFactory {
	return &cassetteFactory{
		pairs: pairs,
		count: len(pairs),
	}
}
