// Copyright 2014 The Prometheus Authors
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

package codable

import (
	"bytes"
	"encoding"
	"reflect"
	"testing"
)

func newFingerprint(fp int64) *Fingerprint {
	cfp := Fingerprint(fp)
	return &cfp
}

func newLabelName(ln string) *LabelName {
	cln := LabelName(ln)
	return &cln
}

func TestUint64(t *testing.T) {
	var b bytes.Buffer
	const n = uint64(422010471112345)
	if err := EncodeUint64(&b, n); err != nil {
		t.Fatal(err)
	}
	got, err := DecodeUint64(&b)
	if err != nil {
		t.Fatal(err)
	}
	if got != n {
		t.Errorf("want %d, got %d", n, got)
	}
}

var scenarios = []struct {
	in    encoding.BinaryMarshaler
	out   encoding.BinaryUnmarshaler
	equal func(in, out interface{}) bool
}{
	{
		in: &Metric{
			"label_1": "value_2",
			"label_2": "value_2",
			"label_3": "value_3",
		},
		out: &Metric{},
	}, {
		in:  newFingerprint(12345),
		out: newFingerprint(0),
	}, {
		in:  &Fingerprints{1, 2, 56, 1234},
		out: &Fingerprints{},
	}, {
		in:  &Fingerprints{1, 2, 56, 1234},
		out: &FingerprintSet{},
		equal: func(in, out interface{}) bool {
			inSet := FingerprintSet{}
			for _, fp := range *(in.(*Fingerprints)) {
				inSet[fp] = struct{}{}
			}
			return reflect.DeepEqual(inSet, *(out.(*FingerprintSet)))
		},
	}, {
		in: &FingerprintSet{
			1:    struct{}{},
			2:    struct{}{},
			56:   struct{}{},
			1234: struct{}{},
		},
		out: &FingerprintSet{},
	}, {
		in: &FingerprintSet{
			1:    struct{}{},
			2:    struct{}{},
			56:   struct{}{},
			1234: struct{}{},
		},
		out: &Fingerprints{},
		equal: func(in, out interface{}) bool {
			outSet := FingerprintSet{}
			for _, fp := range *(out.(*Fingerprints)) {
				outSet[fp] = struct{}{}
			}
			return reflect.DeepEqual(outSet, *(in.(*FingerprintSet)))
		},
	}, {
		in: &LabelPair{
			Name:  "label_name",
			Value: "label_value",
		},
		out: &LabelPair{},
	}, {
		in:  newLabelName("label_name"),
		out: newLabelName(""),
	}, {
		in:  &LabelValues{"value_1", "value_2", "value_3"},
		out: &LabelValues{},
	}, {
		in:  &LabelValues{"value_1", "value_2", "value_3"},
		out: &LabelValueSet{},
		equal: func(in, out interface{}) bool {
			inSet := LabelValueSet{}
			for _, lv := range *(in.(*LabelValues)) {
				inSet[lv] = struct{}{}
			}
			return reflect.DeepEqual(inSet, *(out.(*LabelValueSet)))
		},
	}, {
		in: &LabelValueSet{
			"value_1": struct{}{},
			"value_2": struct{}{},
			"value_3": struct{}{},
		},
		out: &LabelValueSet{},
	}, {
		in: &LabelValueSet{
			"value_1": struct{}{},
			"value_2": struct{}{},
			"value_3": struct{}{},
		},
		out: &LabelValues{},
		equal: func(in, out interface{}) bool {
			outSet := LabelValueSet{}
			for _, lv := range *(out.(*LabelValues)) {
				outSet[lv] = struct{}{}
			}
			return reflect.DeepEqual(outSet, *(in.(*LabelValueSet)))
		},
	}, {
		in:  &TimeRange{42, 2001},
		out: &TimeRange{},
	},
}

func TestCodec(t *testing.T) {
	for i, s := range scenarios {
		encoded, err := s.in.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.out.UnmarshalBinary(encoded); err != nil {
			t.Fatal(err)
		}
		equal := s.equal
		if equal == nil {
			equal = reflect.DeepEqual
		}
		if !equal(s.in, s.out) {
			t.Errorf("%d. Got: %v; want %v; encoded bytes are: %v", i, s.out, s.in, encoded)
		}
	}
}
