// Copyright 2016 The CMux Authors. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package cmux

import (
	"bytes"
	"io"
)

// bufferedReader is an optimized implementation of io.Reader that behaves like
// ```
// io.MultiReader(bytes.NewReader(buffer.Bytes()), io.TeeReader(source, buffer))
// ```
// without allocating.
type bufferedReader struct {
	source     io.Reader
	buffer     *bytes.Buffer
	bufferRead int
	bufferSize int
}

func (s *bufferedReader) Read(p []byte) (int, error) {
	// Functionality of bytes.Reader.
	bn := copy(p, s.buffer.Bytes()[s.bufferRead:s.bufferSize])
	s.bufferRead += bn

	p = p[bn:]

	// Funtionality of io.TeeReader.
	sn, sErr := s.source.Read(p)
	if sn > 0 {
		if wn, wErr := s.buffer.Write(p[:sn]); wErr != nil {
			return bn + wn, wErr
		}
	}
	return bn + sn, sErr
}
