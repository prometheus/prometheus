// Copyright 2014 Google LLC
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

package tracecontext

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
)

var validData = []byte{0, 0, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 1, 97, 98, 99, 100, 101, 102, 103, 104, 2, 1}

func TestDecode(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantTraceID []byte
		wantSpanID  uint64
		wantOpts    byte
		wantOk      bool
	}{
		{
			name:        "nil data",
			data:        nil,
			wantTraceID: nil,
			wantSpanID:  0,
			wantOpts:    0,
			wantOk:      false,
		},
		{
			name:        "short data",
			data:        []byte{0, 0, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77},
			wantTraceID: nil,
			wantSpanID:  0,
			wantOpts:    0,
			wantOk:      false,
		},
		{
			name:        "wrong field number",
			data:        []byte{0, 1, 64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77},
			wantTraceID: nil,
			wantSpanID:  0,
			wantOpts:    0,
			wantOk:      false,
		},
		{
			name:        "valid data",
			data:        validData,
			wantTraceID: []byte{64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79},
			wantSpanID:  0x6867666564636261,
			wantOpts:    1,
			wantOk:      true,
		},
	}
	for _, tt := range tests {
		gotTraceID, gotSpanID, gotOpts, gotOk := Decode(tt.data)
		if !testutil.Equal(gotTraceID, tt.wantTraceID) {
			t.Errorf("%s: Decode() gotTraceID = %v, want %v", tt.name, gotTraceID, tt.wantTraceID)
		}
		if gotSpanID != tt.wantSpanID {
			t.Errorf("%s: Decode() gotSpanID = %v, want %v", tt.name, gotSpanID, tt.wantSpanID)
		}
		if gotOpts != tt.wantOpts {
			t.Errorf("%s: Decode() gotOpts = %v, want %v", tt.name, gotOpts, tt.wantOpts)
		}
		if gotOk != tt.wantOk {
			t.Errorf("%s: Decode() gotOk = %v, want %v", tt.name, gotOk, tt.wantOk)
		}
	}
}

func TestEncode(t *testing.T) {
	tests := []struct {
		name     string
		dst      []byte
		traceID  []byte
		spanID   uint64
		opts     byte
		wantN    int
		wantData []byte
	}{
		{
			name:     "short data",
			dst:      make([]byte, 0),
			traceID:  []byte("00112233445566"),
			spanID:   0x6867666564636261,
			opts:     1,
			wantN:    -1,
			wantData: make([]byte, 0),
		},
		{
			name:     "valid data",
			dst:      make([]byte, Len),
			traceID:  []byte{64, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79},
			spanID:   0x6867666564636261,
			opts:     1,
			wantN:    Len,
			wantData: validData,
		},
	}
	for _, tt := range tests {
		gotN := Encode(tt.dst, tt.traceID, tt.spanID, tt.opts)
		if gotN != tt.wantN {
			t.Errorf("%s: n = %v, want %v", tt.name, gotN, tt.wantN)
		}
		if gotData := tt.dst; !testutil.Equal(gotData, tt.wantData) {
			t.Errorf("%s: dst = %v, want %v", tt.name, gotData, tt.wantData)
		}
	}
}

func BenchmarkDecode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Decode(validData)
	}
}

func BenchmarkEncode(b *testing.B) {
	for i := 0; i < b.N; i++ {
		traceID := make([]byte, 16)
		var opts byte
		Encode(validData, traceID, 0, opts)
	}
}
