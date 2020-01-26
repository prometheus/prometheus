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

package firestore

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

// Returns a negative number, zero, or a positive number depending on whether a is
// less than, equal to, or greater than b according to Firestore's ordering of
// values.
func compareValues(a, b *pb.Value) int {
	ta := typeOrder(a)
	tb := typeOrder(b)
	if ta != tb {
		return compareInt64s(int64(ta), int64(tb))
	}
	switch a := a.ValueType.(type) {
	case *pb.Value_NullValue:
		return 0 // nulls are equal

	case *pb.Value_BooleanValue:
		av := a.BooleanValue
		bv := b.GetBooleanValue()
		switch {
		case av && !bv:
			return 1
		case bv && !av:
			return -1
		default:
			return 0
		}

	case *pb.Value_IntegerValue:
		return compareNumbers(float64(a.IntegerValue), toFloat(b))

	case *pb.Value_DoubleValue:
		return compareNumbers(a.DoubleValue, toFloat(b))

	case *pb.Value_TimestampValue:
		return compareTimestamps(a.TimestampValue, b.GetTimestampValue())

	case *pb.Value_StringValue:
		return strings.Compare(a.StringValue, b.GetStringValue())

	case *pb.Value_BytesValue:
		return bytes.Compare(a.BytesValue, b.GetBytesValue())

	case *pb.Value_ReferenceValue:
		return compareReferences(a.ReferenceValue, b.GetReferenceValue())

	case *pb.Value_GeoPointValue:
		ag := a.GeoPointValue
		bg := b.GetGeoPointValue()
		if ag.Latitude != bg.Latitude {
			return compareFloat64s(ag.Latitude, bg.Latitude)
		}
		return compareFloat64s(ag.Longitude, bg.Longitude)

	case *pb.Value_ArrayValue:
		return compareArrays(a.ArrayValue.Values, b.GetArrayValue().Values)

	case *pb.Value_MapValue:
		return compareMaps(a.MapValue.Fields, b.GetMapValue().Fields)

	default:
		panic(fmt.Sprintf("bad value type: %v", a))
	}
}

// Treats NaN as less than any non-NaN.
func compareNumbers(a, b float64) int {
	switch {
	case math.IsNaN(a):
		if math.IsNaN(b) {
			return 0
		}
		return -1
	case math.IsNaN(b):
		return 1
	default:
		return compareFloat64s(a, b)
	}
}

// Return v as a float64, assuming it's an Integer or Double.
func toFloat(v *pb.Value) float64 {
	if x, ok := v.ValueType.(*pb.Value_IntegerValue); ok {
		return float64(x.IntegerValue)
	}
	return v.GetDoubleValue()
}

func compareTimestamps(a, b *tspb.Timestamp) int {
	if c := compareInt64s(a.Seconds, b.Seconds); c != 0 {
		return c
	}
	return compareInt64s(int64(a.Nanos), int64(b.Nanos))
}

func compareReferences(a, b string) int {
	// Compare path components lexicographically.
	pa := strings.Split(a, "/")
	pb := strings.Split(b, "/")
	return compareSequences(len(pa), len(pb), func(i int) int {
		return strings.Compare(pa[i], pb[i])
	})
}

func compareArrays(a, b []*pb.Value) int {
	return compareSequences(len(a), len(b), func(i int) int {
		return compareValues(a[i], b[i])
	})
}

func compareMaps(a, b map[string]*pb.Value) int {
	sortedKeys := func(m map[string]*pb.Value) []string {
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return ks
	}

	aks := sortedKeys(a)
	bks := sortedKeys(b)
	return compareSequences(len(aks), len(bks), func(i int) int {
		if c := strings.Compare(aks[i], bks[i]); c != 0 {
			return c
		}
		k := aks[i]
		return compareValues(a[k], b[k])
	})
}

func compareSequences(len1, len2 int, compare func(int) int) int {
	for i := 0; i < len1 && i < len2; i++ {
		if c := compare(i); c != 0 {
			return c
		}
	}
	return compareInt64s(int64(len1), int64(len2))
}

func compareFloat64s(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareInt64s(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Return an integer corresponding to the type of value stored in v, such that
// comparing the resulting integers gives the Firestore ordering for types.
func typeOrder(v *pb.Value) int {
	switch v.ValueType.(type) {
	case *pb.Value_NullValue:
		return 0
	case *pb.Value_BooleanValue:
		return 1
	case *pb.Value_IntegerValue:
		return 2
	case *pb.Value_DoubleValue:
		return 2
	case *pb.Value_TimestampValue:
		return 3
	case *pb.Value_StringValue:
		return 4
	case *pb.Value_BytesValue:
		return 5
	case *pb.Value_ReferenceValue:
		return 6
	case *pb.Value_GeoPointValue:
		return 7
	case *pb.Value_ArrayValue:
		return 8
	case *pb.Value_MapValue:
		return 9
	default:
		panic(fmt.Sprintf("bad value type: %v", v))
	}
}
