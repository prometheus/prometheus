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

// removeChunkFile deletes a head chunk file, retrying briefly on transient
// Windows sharing violations. Antivirus or file-indexing processes may open a
// newly created file without granting FILE_SHARE_DELETE, causing os.Remove to
// return ERROR_SHARING_VIOLATION (or ERROR_ACCESS_DENIED on a file already
// marked for deletion). The retry mirrors Go's own testing.removeAll policy;
// see https://github.com/prometheus/prometheus/issues/16176 and
// https://go.dev/issue/51442.
func removeChunkFile(path string) error {
	const (
		initialBackoff = 1 * time.Millisecond
		maxBackoff     = 250 * time.Millisecond
		deadline       = 2 * time.Second
	)
	start := time.Now()
	sleep := initialBackoff
	for {
		err := os.Remove(path)
		if err == nil {
			return nil
		}
		if !isWindowsSharingViolation(err) {
			return err
		}
		if time.Since(start)+sleep > deadline {
			return err
		}
		time.Sleep(sleep)
		sleep *= 2
		if sleep > maxBackoff {
			sleep = maxBackoff
		}
	}
}

// isWindowsSharingViolation reports whether err is a transient Windows
// sharing violation that may be retried.
func isWindowsSharingViolation(err error) bool {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, windows.ERROR_SHARING_VIOLATION) ||
			errors.Is(pathErr.Err, windows.ERROR_ACCESS_DENIED)
	}
	return false
}
