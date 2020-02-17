// Copyright 2020 The Prometheus Authors
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

package tsdbconfig

import (
	"time"

	"github.com/alecthomas/units"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/tsdb"
)

// Options is tsdb.Option version with defined units.
// This is required as tsdb.Option fields are unit agnostic (time).
type Options struct {
	WALSegmentSize         units.Base2Bytes
	RetentionDuration      model.Duration
	MaxBytes               units.Base2Bytes
	NoLockfile             bool
	AllowOverlappingBlocks bool
	WALCompression         bool
	StripeSize             int
	MinBlockDuration       model.Duration
	MaxBlockDuration       model.Duration
}

func (opts Options) ToTSDBOptions() tsdb.Options {
	return tsdb.Options{
		WALSegmentSize:         int(opts.WALSegmentSize),
		RetentionDuration:      int64(time.Duration(opts.RetentionDuration) / time.Millisecond),
		MaxBytes:               int64(opts.MaxBytes),
		NoLockfile:             opts.NoLockfile,
		AllowOverlappingBlocks: opts.AllowOverlappingBlocks,
		WALCompression:         opts.WALCompression,
		StripeSize:             opts.StripeSize,
		MinBlockDuration:       int64(time.Duration(opts.MinBlockDuration) / time.Millisecond),
		MaxBlockDuration:       int64(time.Duration(opts.MaxBlockDuration) / time.Millisecond),
	}
}
