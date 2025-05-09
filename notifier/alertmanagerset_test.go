// Copyright 2013 The Prometheus Authors
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

package notifier

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomDo(t *testing.T) {
	const testURL = "http://testurl.com/"
	const testBody = "testbody"

	var received bool
	h := alertmanagerSet{
		opts: &Options{
			Do: func(_ context.Context, _ *http.Client, req *http.Request) (*http.Response, error) {
				received = true
				body, err := io.ReadAll(req.Body)

				require.NoError(t, err)

				require.Equal(t, testBody, string(body))

				require.Equal(t, testURL, req.URL.String())

				return &http.Response{
					Body: io.NopCloser(bytes.NewBuffer(nil)),
				}, nil
			},
		},
	}

	h.sendOne(context.Background(), nil, testURL, []byte(testBody))

	require.True(t, received, "Expected to receive an alert, but didn't")
}
