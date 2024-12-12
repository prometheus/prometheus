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

package encoding

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
	"unsafe"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdbv2/hash"
	"github.com/prometheus/prometheus/tsdbv2/keys"
)

type SeriesKind byte

const (
	SeriesFloat SeriesKind = 1 + iota
	SeriesHistogram
	SeriesFloatHistogram
	SeriesExemplar
	SeriesMetadata
)

var AllKinds = []SeriesKind{
	SeriesFloat,
	SeriesHistogram,
	SeriesFloatHistogram,
	SeriesExemplar,
	SeriesMetadata,
}

const (
	TenantID   = "__tenant__"
	RootTenant = ^uint64(8)
)

const Sep = '\x00'

var SepBytes = []byte{Sep}

const (
	PrefixOffset    = 0
	TenantOffset    = PrefixOffset + 1
	ShardOffset     = TenantOffset + 8
	FieldOffset     = ShardOffset + 8
	ContainerOffset = FieldOffset + 8
	DataKeySize     = ContainerOffset + 8
)

func DataComponents(key []byte) (tenant, shard uint64) {
	tenant = binary.BigEndian.Uint64(key[TenantOffset:])
	shard = binary.BigEndian.Uint64(key[ShardOffset:])
	return
}

func ContainerKey(key []byte) (co uint64) {
	co = binary.BigEndian.Uint64(key[ContainerOffset:])
	return
}

func DebugDataKey(key []byte) {
	fmt.Printf("prefix=%q tenant=%d shard=%d field=%d container=%d\n",
		keys.Prefix(key[PrefixOffset]),
		binary.BigEndian.Uint64(key[TenantOffset:]),
		binary.BigEndian.Uint64(key[ShardOffset:]),
		binary.BigEndian.Uint64(key[FieldOffset:]),
		binary.BigEndian.Uint64(key[ContainerOffset:]),
	)
}

func AppendDataKey(tenant, shard, field, container uint64, b []byte) []byte {
	b = append(b, byte(keys.Data))
	b = binary.BigEndian.AppendUint64(b, tenant)
	b = binary.BigEndian.AppendUint64(b, shard)
	b = binary.BigEndian.AppendUint64(b, field)
	b = binary.BigEndian.AppendUint64(b, container)
	return b
}

func AppendExistenceKey(tenant, shard, field, container uint64, b []byte) []byte {
	b = binary.BigEndian.AppendUint64(b, tenant)
	b = append(b, byte(keys.Existence))
	b = binary.BigEndian.AppendUint64(b, shard)
	b = binary.BigEndian.AppendUint64(b, field)
	b = binary.BigEndian.AppendUint64(b, container)
	return b
}

func MakeBlobTranslationKey(tenant, field, hash uint64) []byte {
	return AppendBlobTranslationKey(tenant, field, hash, nil)
}

func AppendBlobTranslationKey(tenant, field, hash uint64, o []byte) []byte {
	o = append(o[:0], byte(keys.TrsnalteBlobKey))
	o = binary.BigEndian.AppendUint64(o, tenant)
	o = binary.BigEndian.AppendUint64(o, field)
	o = binary.BigEndian.AppendUint64(o, hash)
	return o
}

func MakeTextTranslationKey(tenant, field uint64, data []byte) []byte {
	return AppendTextTranslationKey(tenant, field, data, nil)
}

func AppendTextTranslationKey(tenant, field uint64, data, o []byte) []byte {
	o = append(o[:0], byte(keys.TranslateTextKey))
	o = binary.BigEndian.AppendUint64(o, tenant)
	o = binary.BigEndian.AppendUint64(o, field)
	o = append(o, data...)
	return o
}

func MakeTranslationTextID(tenant, field, id uint64) []byte {
	return AppendTranslationTextID(tenant, field, id, nil)
}

func AppendTranslationTextID(tenant, field, id uint64, o []byte) []byte {
	o = append(o[:0], byte(keys.TranslateTextID))
	o = binary.BigEndian.AppendUint64(o, tenant)
	o = binary.BigEndian.AppendUint64(o, field)
	o = binary.BigEndian.AppendUint64(o, id)
	return o
}

func MakeBlobTranslationID(tenant, field, id uint64) []byte {
	return AppendBlobTranslationID(tenant, field, id, nil)
}

func AppendBlobTranslationID(tenant, field, id uint64, o []byte) []byte {
	o = append(o[:0], byte(keys.TranslateBlobID))
	o = binary.BigEndian.AppendUint64(o, tenant)
	o = binary.BigEndian.AppendUint64(o, field)
	o = binary.BigEndian.AppendUint64(o, id)
	return o
}

const (
	LabelPrefixOfffset = 1 + // prefix
		8 + // tenant
		8 // field hash
)

func AppendLabel(la *labels.Label, o []byte) []byte {
	o = append(o, unsafe.Slice(unsafe.StringData(la.Name), len(la.Name))...)
	o = append(o, Sep)
	o = append(o, unsafe.Slice(unsafe.StringData(la.Value), len(la.Value))...)
	return o
}

func MakeLabelTranslationKey(tenant uint64, la *labels.Label) []byte {
	o := []byte(la.Name)
	field := hash.Bytes(o)
	o = append(o, SepBytes...)
	o = append(o, []byte(la.Value)...)
	return MakeTextTranslationKey(tenant, field, o)
}

func DebugLabelKey(key []byte) {
	pre, val, _ := bytes.Cut(key, SepBytes)
	tenant := binary.BigEndian.Uint64(pre[1:])
	field := binary.BigEndian.Uint64(pre[1+8:])
	name := pre[1+8+8:]

	fmt.Printf(
		"prefix=%q tenant=%d field=%d name=%q value=%q\n",
		keys.Prefix(pre[0]), tenant, field, string(name), string(val),
	)
}

func MakeLabelTranslationKeyPrefix(tenant uint64) []byte {
	o := []byte{byte(keys.TranslateTextKey)}
	o = binary.BigEndian.AppendUint64(o, tenant)
	return o
}

func AppendSeq(tenant, field uint64, o []byte) []byte {
	o = slices.Grow(o[:0], 17)
	o = append(o, byte(keys.Seq))
	o = binary.BigEndian.AppendUint64(o, tenant)
	o = binary.BigEndian.AppendUint64(o, field)
	return o
}

func AppendFieldMapping(tenant, shard, field uint64, o []byte) []byte {
	o = append(o, byte(keys.FieldsPerShard))
	o = binary.BigEndian.AppendUint64(o, tenant)
	o = binary.BigEndian.AppendUint64(o, shard)
	o = binary.BigEndian.AppendUint64(o, field)
	return o
}

func FieldPerShardComponent(key []byte) (tenant, shard, base uint64) {
	tenant = binary.BigEndian.Uint64(key[1:])
	shard = binary.BigEndian.Uint64(key[1+8:])
	base = binary.BigEndian.Uint64(key[1+8+8:])
	return
}

func MakeSeriesRefCacheKey(tenant, ref uint64) []byte {
	o := []byte{byte(keys.SeriesRefCache)}
	o = binary.BigEndian.AppendUint64(o, tenant)
	o = binary.BigEndian.AppendUint64(o, ref)
	return o
}
