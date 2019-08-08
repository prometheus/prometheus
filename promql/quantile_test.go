// Copyright 2015 The Prometheus Authors
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

package promql

import (
	"math"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestBucketQuantile(t *testing.T) {
	var bks = make([]bucket, 0)

	bks = append(bks, bucket{upperBound: 0.100000, count: 0.000000})
	bks = append(bks, bucket{upperBound: 0.200000, count: 0.000000})
	bks = append(bks, bucket{upperBound: 0.400000, count: 0.000000})
	bks = append(bks, bucket{upperBound: 0.800000, count: 9.701762})
	bks = append(bks, bucket{upperBound: 1.600000, count: 46.463782})
	bks = append(bks, bucket{upperBound: 3.200000, count: 58.328698})
	bks = append(bks, bucket{upperBound: 6.400000, count: 65.677861})
	bks = append(bks, bucket{upperBound: 12.800000, count: 70.311609})
	bks = append(bks, bucket{upperBound: 25.600000, count: 72.026843})
	bks = append(bks, bucket{upperBound: 51.200000, count: 75.758632})
	bks = append(bks, bucket{upperBound: 102.400000, count: 83.790729})
	bks = append(bks, bucket{upperBound: 204.800000, count: 91.656571})
	bks = append(bks, bucket{upperBound: 409.600000, count: 113.088153})
	bks = append(bks, bucket{upperBound: 819.200000, count: 113.920766})
	bks = append(bks, bucket{upperBound: 1638.400000, count: 130.186402})
	bks = append(bks, bucket{upperBound: 3276.800000, count: 138.251215})
	bks = append(bks, bucket{upperBound: 6553.600000, count: 142.551112})
	bks = append(bks, bucket{upperBound: 13107.200000, count: 146.384342})
	bks = append(bks, bucket{upperBound: 26214.400000, count: 142.517469})
	bks = append(bks, bucket{upperBound: 52428.800000, count: 142.617469})
	bks = append(bks, bucket{upperBound: 104857.600000, count: 142.617469})
	bks = append(bks, bucket{upperBound: 209715.200000, count: 142.617469})
	bks = append(bks, bucket{upperBound: 419430.400000, count: 143.617469})
	bks = append(bks, bucket{upperBound: 838860.800000, count: 145.591751})
	bks = append(bks, bucket{upperBound: 1677721.600000, count: 138.950803})
	bks = append(bks, bucket{upperBound: math.Inf(1), count: 150.258418})

	q := bucketQuantile(0.99, bks)
	t.Log("bucketQuantile: ", q, ", 18th upper bound: ", bks[18].upperBound)
	testutil.Assert(t, q < bks[18].upperBound, "Expect < %f but got %f", bks[18].upperBound, q)
}
