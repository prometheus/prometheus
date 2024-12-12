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

package translate

import (
	"encoding/binary"
	"sync"

	"github.com/cockroachdb/pebble"

	"github.com/prometheus/prometheus/tsdbv2/encoding"
	"github.com/prometheus/prometheus/tsdbv2/hash"
)

type Translator interface {
	// NextSeq returns a monotomically increasing uint64 for a given tenant, field
	// locality.
	//
	// Values are in the sequence 1, 2, 3 ...
	//
	// Returned uint64 must be treated as coming from memory. It is resposnility of the receiver
	// to persist the value with tenantID, field locality. Use [encoding.AppendSeq] to
	// create the key for persisting the sequence number. Sequence number value must be
	// encoded with [binary.BigEndian.PutUint64].
	NextSeq(tenantID, field uint64) (uint64, error)
	// TranslateBlob assigns a unique monotomically increasing uint64 sequence number
	// to data slice.
	//
	// data is stored in content addressable manner, where data hash is stored as part
	// of the key created by encoding.MakeBlobTranslationKey.
	//
	// data is arbitrary in size and we don't use blobs for search. This is the reason
	// we store hashes instead. Treat this method as a store/retrival of arbitrady
	// data chunks.
	//
	// Use key encoding.MakeBlobTranslationID(tenant, field, id) to access the raw
	// blob data. and  encoding.MakeBlobTranslationKey(tenant, field, hash) to get
	// back the id.
	//
	// Same [ tenant, field, data ] locality retuens the same uint64 value. There
	// is no caching allowed for implementation. Consumers of this interface must
	// do caching elsewhere.
	TranslateBlob(tenant, field uint64, data []byte) (uint64, error)
	// TranslateText assigns a unique monotomically increasing uint64 sequence number
	// to data slice.
	//
	// data is treated as a  string. It is assigned as the last component for encoding.MakeTextTranslationKey
	// which allows prefix based iteration on the underlying pebble storage to find
	// which id was assigned to data.
	//
	// Use encoding.MakeTextTranslationKey(tenant, field, data) to get back the id and
	// encoding.MakeTranslationTextID(tenant, field, id) to get back the raw data slice.
	//
	// Same [ tenant, field, data ] combination retuens the same uint64 value. There
	// is no caching allowed for implementation. Consumers of this interface must
	// do caching elsewhere.
	TranslateText(tenant, baseField uint64, data []byte) (id uint64, err error)
}

type Transtale struct {
	db     *pebble.DB
	fields struct {
		data map[uint64]uint64
		sync.Mutex
	}
	mu sync.Mutex
}

var _ Translator = (*Transtale)(nil)

func New(db *pebble.DB) *Transtale {
	tr := &Transtale{db: db}
	tr.fields.data = make(map[uint64]uint64)
	return tr
}

func (tr *Transtale) NextSeq(tenant, field uint64) (uint64, error) {
	key := encoding.AppendSeq(tenant, field, nil)
	hash := hash.Bytes(key)
	tr.fields.Lock()
	defer tr.fields.Unlock()
	id, ok := tr.fields.data[hash]
	if !ok {
		id = tr.seqFromStore(key)
	}
	id++
	tr.fields.data[hash] = id
	return id, nil
}

func (tr *Transtale) TranslateBlob(tenant, field uint64, data []byte) (id uint64, err error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	hash := hash.Bytes(data)
	trKey := encoding.MakeBlobTranslationKey(tenant, field, hash)
	// try to read existing
	value, done, verr := tr.db.Get(trKey)
	if verr == nil {
		id = binary.BigEndian.Uint64(value)
		done.Close()
		return
	}

	id, err = tr.NextSeq(tenant, field)
	if err != nil {
		return
	}
	trID := encoding.MakeBlobTranslationID(tenant, field, id)

	idBytes := trID[len(trID)-8:]
	err = tr.db.Set(trKey, idBytes, nil)
	if err != nil {
		return
	}
	err = tr.db.Set(trID, data, nil)
	return
}

func (tr *Transtale) TranslateText(tenant, field uint64, data []byte) (id uint64, err error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	trKey := encoding.MakeTextTranslationKey(tenant, field, data)
	value, done, verr := tr.db.Get(trKey)
	if verr == nil {
		id = binary.BigEndian.Uint64(value)
		done.Close()
		return
	}

	id, err = tr.NextSeq(tenant, field)
	if err != nil {
		return
	}
	trID := encoding.MakeTranslationTextID(tenant, field, id)

	// There is no need to encode id again. We know the last component of trID is
	// the encoded id. We can reuse the encoded chunk safely.
	idBytes := trID[len(trID)-8:]
	err = tr.db.Set(trKey, idBytes, nil)
	if err != nil {
		return
	}
	err = tr.db.Set(trID, data, nil)
	return
}

func (tr *Transtale) seqFromStore(key []byte) (id uint64) {
	value, done, err := tr.db.Get(key)
	if err == nil {
		id = binary.BigEndian.Uint64(value)
		done.Close()
	}
	return
}
