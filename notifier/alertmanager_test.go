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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
)

func TestPostPath(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{
			in:  "",
			out: "/api/v2/alerts",
		},
		{
			in:  "/",
			out: "/api/v2/alerts",
		},
		{
			in:  "/prefix",
			out: "/prefix/api/v2/alerts",
		},
		{
			in:  "/prefix//",
			out: "/prefix/api/v2/alerts",
		},
		{
			in:  "prefix//",
			out: "/prefix/api/v2/alerts",
		},
	}
	for _, c := range cases {
		require.Equal(t, c.out, postPath(c.in, config.AlertmanagerAPIVersionV2))
	}
}

func TestLabelSetNotReused(t *testing.T) {
	tg := makeInputTargetGroup()
	_, _, err := AlertmanagerFromGroup(tg, &config.AlertmanagerConfig{})

	require.NoError(t, err)

	// Target modified during alertmanager extraction
	require.Equal(t, tg, makeInputTargetGroup())
}
