// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fileutil

import (
	"bufio"
	"errors"
	"os"
)

var errDirectIOUnsupported = errors.New("direct IO is unsupported")

type BufWriter interface {
	Write([]byte) (int, error)
	Flush() error
	Reset(f *os.File) error
}

// writer is a specialized wrapper around bufio.Writer.
// It is used when Direct IO isn't enabled, as using directIOWriter in such cases is impractical.
type writer struct {
	*bufio.Writer
}

func (b *writer) Reset(f *os.File) error {
	b.Writer.Reset(f)
	return nil
}
