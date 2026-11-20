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

package deprecation

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func TestMarkDeprecated(t *testing.T) {
	const (
		retention = "Using the --storage.tsdb.retention.time CLI flag"
		endpoints = "Using the kubernetes_sd_configs role: endpoints configuration field"
		stats     = "Using a non-standard /api/v1 query stats parameter"
		samples   = "Exposing the prometheus_remote_storage_samples_in_total metric"
	)

	for _, tc := range []struct {
		name  string
		marks func()
		want  string
	}{
		{
			name:  "nothing marked exposes no series at all",
			marks: func() {},
			want:  "",
		},
		{
			name:  "a marked item is exposed at 1",
			marks: func() { MarkDeprecated(retention, FutureMinorRelease) },
			want: `
# HELP prometheus_deprecation_notice Whether a deprecated feature, flag, configuration field, or behaviour has been seen in use since startup.
# TYPE prometheus_deprecation_notice gauge
prometheus_deprecation_notice{details_url="https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices",expected_removal_in="future_minor_release",name="Using the --storage.tsdb.retention.time CLI flag",scope="user"} 1
`,
		},
		{
			name: "marking the same item repeatedly keeps a single series at 1",
			marks: func() {
				MarkDeprecated(retention, FutureMinorRelease)
				MarkDeprecated(retention, FutureMinorRelease)
				MarkDeprecated(retention, FutureMinorRelease)
			},
			want: `
# HELP prometheus_deprecation_notice Whether a deprecated feature, flag, configuration field, or behaviour has been seen in use since startup.
# TYPE prometheus_deprecation_notice gauge
prometheus_deprecation_notice{details_url="https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices",expected_removal_in="future_minor_release",name="Using the --storage.tsdb.retention.time CLI flag",scope="user"} 1
`,
		},
		{
			name: "each item gets its own series, with its own removal target",
			marks: func() {
				MarkDeprecated(retention, FutureMinorRelease)
				MarkDeprecated(endpoints, FutureMinorRelease)
				MarkDeprecated(stats, NextMajorRelease)
			},
			want: `
# HELP prometheus_deprecation_notice Whether a deprecated feature, flag, configuration field, or behaviour has been seen in use since startup.
# TYPE prometheus_deprecation_notice gauge
prometheus_deprecation_notice{details_url="https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices",expected_removal_in="next_major_release",name="Using a non-standard /api/v1 query stats parameter",scope="user"} 1
prometheus_deprecation_notice{details_url="https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices",expected_removal_in="future_minor_release",name="Using the --storage.tsdb.retention.time CLI flag",scope="user"} 1
prometheus_deprecation_notice{details_url="https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices",expected_removal_in="future_minor_release",name="Using the kubernetes_sd_configs role: endpoints configuration field",scope="user"} 1
`,
		},
		{
			name: "an item the user cannot clear is scope=internal",
			marks: func() {
				MarkDeprecated(retention, FutureMinorRelease)
				MarkDeprecatedInternal(samples, FutureMinorRelease)
			},
			want: `
# HELP prometheus_deprecation_notice Whether a deprecated feature, flag, configuration field, or behaviour has been seen in use since startup.
# TYPE prometheus_deprecation_notice gauge
prometheus_deprecation_notice{details_url="https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices",expected_removal_in="future_minor_release",name="Exposing the prometheus_remote_storage_samples_in_total metric",scope="internal"} 1
prometheus_deprecation_notice{details_url="https://prometheus.io/docs/prometheus/latest/stability/#deprecation-notices",expected_removal_in="future_minor_release",name="Using the --storage.tsdb.retention.time CLI flag",scope="user"} 1
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deprecationNotice.Reset()
			reg := prometheus.NewPedanticRegistry()
			Register(reg)

			tc.marks()

			require.NoError(t, testutil.GatherAndCompare(
				reg, strings.NewReader(tc.want), "prometheus_deprecation_notice",
			))
		})
	}
}

// TestWithoutRegisterNothingIsExposed checks what library consumers of this
// repository rely on: code they call can mark as much as it likes, their own
// registry stays clean because they never registered the metric.
func TestWithoutRegisterNothingIsExposed(t *testing.T) {
	deprecationNotice.Reset()
	consumerReg := prometheus.NewPedanticRegistry()

	MarkDeprecated("Using the --log.level CLI flag", FutureMinorRelease)
	MarkDeprecatedInternal("Exposing the prometheus_remote_storage_samples_in_total metric", FutureMinorRelease)

	mfs, err := consumerReg.Gather()
	require.NoError(t, err)
	require.Empty(t, mfs)
}
