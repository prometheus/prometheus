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

package wlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// minValidTimeFilename is a small marker file, stored alongside the WAL segments and
// checkpoints, that records the mint every truncation has used. Blocks on disk cannot always
// stand in for it: a block produced by CompactSelectedSeries or CompactStaleHead carries the
// FromSelectedSeries or FromStaleSeries compaction hint and is intentionally excluded from
// the search for the head's minValidTime on restart, and a truncation whose range had
// nothing left to write never produces a block at all. Without this file, minValidTime can
// then be recovered as a value lower than where the WAL was actually truncated, letting
// replay walk into samples whose series record was already dropped from the checkpoint and
// log them as unknown series references, even though no data was ever lost.
const minValidTimeFilename = "min-valid-time"

// WriteMinValidTime persists mint, the mint a WAL truncation has just used, to dir. It is
// safe to call repeatedly; each call overwrites the previous value. Callers only need to
// track the highest mint they have used, since ReadMinValidTime callers only care about the
// most permissive (highest) truncation point ever reached.
func WriteMinValidTime(dir string, mint int64) error {
	tmp := filepath.Join(dir, minValidTimeFilename+".tmp")
	if err := os.WriteFile(tmp, []byte(strconv.FormatInt(mint, 10)), 0o666); err != nil {
		return fmt.Errorf("write min valid time: %w", err)
	}
	return fileutil.Replace(tmp, filepath.Join(dir, minValidTimeFilename))
}

// ReadMinValidTime reads back the mint last persisted by WriteMinValidTime for dir. ok is
// false, with no error, if dir's WAL has never been truncated yet, i.e. the file does not
// exist.
func ReadMinValidTime(dir string) (mint int64, ok bool, err error) {
	b, err := os.ReadFile(filepath.Join(dir, minValidTimeFilename))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read min valid time: %w", err)
	}
	mint, err = strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse min valid time: %w", err)
	}
	return mint, true, nil
}
