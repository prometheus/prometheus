// Copyright The Prometheus Authors
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

package rw2arrow

import (
	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

var (
	zstdEncoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	zstdDecoder, _ = zstd.NewReader(nil)
)

func compressSnappy(dst, src []byte) []byte {
	return snappy.Encode(dst, src)
}

func decompressSnappy(dst, src []byte) ([]byte, error) {
	return snappy.Decode(dst, src)
}

func compressZstd(dst, src []byte) []byte {
	return zstdEncoder.EncodeAll(src, dst[:0])
}

func decompressZstd(dst, src []byte) ([]byte, error) {
	return zstdDecoder.DecodeAll(src, dst[:0])
}
