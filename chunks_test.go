// Copyright 2017 The Prometheus Authors
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

package tsdb

import (
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/chunks"
)

type mockChunkReader map[uint64]chunks.Chunk

func (cr mockChunkReader) Chunk(ref uint64) (chunks.Chunk, error) {
	chk, ok := cr[ref]
	if ok {
		return chk, nil
	}

	return nil, errors.New("Chunk with ref not found")
}

func (cr mockChunkReader) Close() error {
	return nil
}
