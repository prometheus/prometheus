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

//go:build !windows
// +build !windows

package chunks

// HeadChunkFilePreallocationSize is the size to which the m-map file should be preallocated when a new file is cut.
// Windows needs pre-allocations while the other OS does not. But we observed that a 0 pre-allocation causes unit tests to flake.
// This small allocation for non-Windows OSes removes the flake.
var HeadChunkFilePreallocationSize int64 = MinWriteBufferSize * 2
