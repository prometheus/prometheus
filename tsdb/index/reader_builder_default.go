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

//go:build !windows
// +build !windows

package index

import (
	"fmt"
	"io"
	"syscall"
)

func newReader(b ByteSlice, c io.Closer) (*Reader, error) {
	return createReader(b, c, func(r *Reader) error {
		if err := syscall.Mlock(r.b.Range(int(r.toc.Symbols), int(r.toc.Series))); err != nil {
			fmt.Printf("unable to call mlock, err: %v\n", err)
			return nil
		}
		r.closeReaderFunc = func() error {
			if err := syscall.Munlock(r.b.Range(int(r.toc.Symbols), int(r.toc.Series))); err != nil {
				fmt.Printf("unable to call munlock, err: %v\n", err)
			}
			return nil
		}
		return nil
	})
}
