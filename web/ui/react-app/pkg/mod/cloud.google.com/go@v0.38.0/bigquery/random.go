// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"math/rand"
	"os"
	"sync"
	"time"
)

// Support for random values (typically job IDs and insert IDs).

const alphanum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var (
	rngMu sync.Mutex
	rng   = rand.New(rand.NewSource(time.Now().UnixNano() ^ int64(os.Getpid())))
)

// For testing.
var randomIDFn = randomID

// As of August 2017, the BigQuery service uses 27 alphanumeric characters for
// suffixes.
const randomIDLen = 27

func randomID() string {
	// This is used for both job IDs and insert IDs.
	var b [randomIDLen]byte
	rngMu.Lock()
	for i := 0; i < len(b); i++ {
		b[i] = alphanum[rng.Intn(len(alphanum))]
	}
	rngMu.Unlock()
	return string(b[:])
}

// Seed seeds this package's random number generator, used for generating job and
// insert IDs. Use Seed to obtain repeatable, deterministic behavior from bigquery
// clients. Seed should be called before any clients are created.
func Seed(s int64) {
	rng = rand.New(rand.NewSource(s))
}
