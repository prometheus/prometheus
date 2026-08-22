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

package tsdb

import (
	"context"
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/stretchr/testify/require"
)

func TestAddUnsortedLabels(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)

	add := func(lbls labels.Labels, expectErr string) {
		app := h.Appender(context.Background())
		_, err := app.Append(0, lbls, 0, 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), expectErr)
	}

	// Labels not sorted: "b" comes before "a".
	add(labels.FromStrings("b", "1", "a", "2"), "labels are not sorted")
	// Labels not sorted: "z" comes before "m".
	add(labels.FromStrings("z", "1", "m", "2", "a", "3"), "labels are not sorted")
	// Already sorted labels should succeed.
	app := h.Appender(context.Background())
	_, err := app.Append(0, labels.FromStrings("a", "1", "b", "2"), 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
}
