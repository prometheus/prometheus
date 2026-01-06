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

package writev2

import (
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/prompb"
)

func TestInteropV2UnmarshalWithV1_DeterministicEmpty(t *testing.T) {
	expectedV1Empty := &prompb.WriteRequest{}
	for _, tc := range []struct{ incoming *Request }{
		{
			incoming: &Request{}, // Technically wrong, should be at least empty string in symbol.
		},
		{
			incoming: &Request{
				Symbols: []string{""},
			}, // NOTE: Without reserved fields, failed with "corrupted" ghost TimeSeries element.
		},
		{
			incoming: &Request{
				Symbols: []string{"", "__name__", "metric1"},
				Timeseries: []TimeSeries{
					{LabelsRefs: []uint32{1, 2}},
					{Samples: []Sample{{Value: 21.4, Timestamp: time.Now().UnixMilli()}}},
				}, // NOTE:  Without reserved fields, proto: illegal wireType 7
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			in, err := proto.Marshal(tc.incoming)
			require.NoError(t, err)

			// Test accidental unmarshal of v2 payload with v1 proto.
			out := &prompb.WriteRequest{}
			require.NoError(t, proto.Unmarshal(in, out))

			// Drop unknowns, we expect them when incoming payload had some fields.
			// This field & method will be likely gone after gogo removal.
			out.XXX_unrecognized = nil // NOTE: out.XXX_DiscardUnknown() does not work with nullables.

			require.Equal(t, expectedV1Empty, out)
		})
	}
}

func TestInteropV1UnmarshalWithV2_DeterministicEmpty(t *testing.T) {
	expectedV2Empty := &Request{}
	for _, tc := range []struct{ incoming *prompb.WriteRequest }{
		{
			incoming: &prompb.WriteRequest{},
		},
		{
			incoming: &prompb.WriteRequest{
				Timeseries: []prompb.TimeSeries{
					{
						Labels:  []prompb.Label{{Name: "__name__", Value: "metric1"}},
						Samples: []prompb.Sample{{Value: 21.4, Timestamp: time.Now().UnixMilli()}},
					},
				},
			},
			// NOTE: Without reserved fields, results in corrupted v2.Request.Symbols.
		},
	} {
		t.Run("", func(t *testing.T) {
			in, err := proto.Marshal(tc.incoming)
			require.NoError(t, err)

			// Test accidental unmarshal of v1 payload with v2 proto.
			out := &Request{}
			require.NoError(t, proto.Unmarshal(in, out))

			// Drop unknowns, we expect them when incoming payload had some fields.
			// This field & method will be likely gone after gogo removal.
			out.XXX_unrecognized = nil // NOTE: out.XXX_DiscardUnknown() does not work with nullables.

			require.Equal(t, expectedV2Empty, out)
		})
	}
}
