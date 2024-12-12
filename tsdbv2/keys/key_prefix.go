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

package keys

type Prefix byte

const (
	// Data stores bitmap containers.
	Data Prefix = 1 + iota
	// Existence store existence bits for equality encoded bitmaps. Used to answer
	// netated filters.
	Existence

	// TranslateTextKey records all text keyys whose value is a uint64 that is a reference
	// to TranslateTextID id.
	TranslateTextKey
	// TranslateTextID records  text data.
	TranslateTextID

	// TrsnalteBlobKey records all blob hashes whose value is a uint64 that is a reference
	// to TranslateBlobID id.
	TrsnalteBlobKey
	// TranslateBlobID records  blob data.
	TranslateBlobID
	// Seq records all auto incrementing uint64.
	Seq
	FieldsPerShard

	// SeriesRefCache storage keyspace for [cache.Series].
	SeriesRefCache
)

func (p Prefix) String() string {
	switch p {
	case Data:
		return "data"
	case Existence:
		return "exists"
	case TranslateTextKey:
		return "text_key"
	case TranslateTextID:
		return "text_id"
	case TrsnalteBlobKey:
		return "blob_key"
	case TranslateBlobID:
		return "blob_id"
	case Seq:
		return "seq"
	case FieldsPerShard:
		return "fields_per_shard"
	case SeriesRefCache:
		return "series_ref_cache"
	default:
		return ""
	}
}
