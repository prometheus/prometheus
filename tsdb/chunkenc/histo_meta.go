// Copyright 2021 The Prometheus Authors
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

// The code in this file was largely written by Damian Gryski as part of
// https://github.com/dgryski/go-tsz and published under the license below.
// It was modified to accommodate reading from byte slices without modifying
// the underlying bytes, which would panic when reading from mmap'd
// read-only byte slices.
package chunkenc

import "github.com/prometheus/prometheus/pkg/histogram"

func writeHistoChunkMeta(b *bstream, schema int32, posSpans, negSpans []histogram.Span) {
	putInt64VBBucket(b, int64(schema))
	putHistoChunkMetaSpans(b, posSpans)
	putHistoChunkMetaSpans(b, negSpans)
}

func putHistoChunkMetaSpans(b *bstream, spans []histogram.Span) {
	putInt64VBBucket(b, int64(len(spans)))
	for _, s := range spans {
		putInt64VBBucket(b, int64(s.Length))
		putInt64VBBucket(b, int64(s.Offset))
	}
}

func readHistoChunkMeta(b *bstreamReader) (int32, []histogram.Span, []histogram.Span, error) {

	v, err := readInt64VBBucket(b)
	if err != nil {
		return 0, nil, nil, err
	}
	schema := int32(v)

	posSpans, err := readHistoChunkMetaSpans(b)
	if err != nil {
		return 0, nil, nil, err
	}

	negSpans, err := readHistoChunkMetaSpans(b)
	if err != nil {
		return 0, nil, nil, err
	}

	return schema, posSpans, negSpans, nil
}

func readHistoChunkMetaSpans(b *bstreamReader) ([]histogram.Span, error) {
	var spans []histogram.Span
	num, err := readInt64VBBucket(b)
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(num); i++ {

		length, err := readInt64VBBucket(b)
		if err != nil {
			return nil, err
		}

		offset, err := readInt64VBBucket(b)
		if err != nil {
			return nil, err
		}

		spans = append(spans, histogram.Span{
			Length: uint32(length),
			Offset: int32(offset),
		})
	}
	return spans, nil
}
