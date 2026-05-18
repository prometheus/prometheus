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

package chunks

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// HeadChunkFilePreallocationSize is the size to which the m-map file should be preallocated when a new file is cut.
// Windows needs pre-allocation to m-map the file.
var HeadChunkFilePreallocationSize int64 = MaxHeadChunkFileSize

// removeChunkFile deletes a head chunk file, briefly retrying on transient
// Windows errors caused by antivirus or file-indexing processes that open a
// freshly created file without granting FILE_SHARE_DELETE. While such a handle
// is held, os.Remove fails with ERROR_SHARING_VIOLATION (or occasionally
// ERROR_ACCESS_DENIED for a file already marked for deletion). This mirrors
// the retry policy used by Go's own testing package; see
// https://github.com/prometheus/prometheus/issues/16176 and
// https://go.dev/issue/51442.
func removeChunkFile(path string) error {
	const (
		initialBackoff = 1 * time.Millisecond
		maxBackoff     = 250 * time.Millisecond
		deadline       = 2 * time.Second
	)

	backoff := initialBackoff
	start := time.Now()
	for {
		err := os.Remove(path)
		if err == nil || !isRetryableRemoveError(err) {
			return err
		}
		if time.Since(start) >= deadline {
			return err
		}
		time.Sleep(backoff)
		backoff = min(backoff*2, maxBackoff)
	}
}

// isRetryableRemoveError reports whether err is a Windows filesystem error that
// typically clears itself when retried, as used by Go's testing.removeAll.
func isRetryableRemoveError(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
