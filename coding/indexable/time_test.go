// Copyright 2013 Prometheus Team
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

package indexable

import (
	"math/rand"
	"testing"
	"testing/quick"

	clientmodel "github.com/prometheus/client_golang/model"
)

func TestTimeEndToEnd(t *testing.T) {
	tester := func(x int) bool {
		random := rand.New(rand.NewSource(int64(x)))
		buffer := make([]byte, 8)
		incoming := clientmodel.TimestampFromUnix(random.Int63())

		EncodeTimeInto(buffer, incoming)
		outgoing := DecodeTime(buffer)

		return incoming.Equal(outgoing) && incoming.Unix() == outgoing.Unix()
	}

	if err := quick.Check(tester, nil); err != nil {
		t.Error(err)
	}
}
