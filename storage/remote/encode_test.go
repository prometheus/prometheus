// Copyright 2018 The Prometheus Authors
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

package remote

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestEncoderReadResponseSampleLimit(t *testing.T) {
	for _, format := range []prompb.ReadRequest_Response{prompb.ReadRequest_RAW, prompb.ReadRequest_RAW_STREAMED} {
		t.Run(format.String(), func(t *testing.T) {
			b := bytes.Buffer{}

			enc, err := NewEncoder(&b, &prompb.ReadRequest{
				Queries:      make([]*prompb.Query, 2),
				ResponseType: format,
			}, 2)
			require.NoError(t, err)

			h := http.Header{}
			for k, v := range enc.ContentHeaders() {
				h.Set(k, v)
			}

			ts1 := []*prompb.TimeSeries{
				{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b")), Samples: []*prompb.Sample{{}}},
				{Labels: labelsToLabelsProto(labels.FromStrings("foo", "bar", "a", "b2")), Samples: []*prompb.Sample{{}, {}}},
			}

			err = enc.Encode(ts1, 0)
			require.Error(t, err)
			_, ok := err.(HTTPError)
			require.True(t, ok)
		})
	}
}
