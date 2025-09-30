// Copyright 2025 The Prometheus Authors
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

package promtoollib

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestCheckScrapeConfigs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lookbackDelta model.Duration
		expectError   bool
	}{
		{
			name:          "scrape interval less than lookback delta",
			lookbackDelta: model.Duration(11 * time.Minute),
			expectError:   false,
		},
		{
			name:          "scrape interval greater than lookback delta",
			lookbackDelta: model.Duration(5 * time.Minute),
			expectError:   true,
		},
		{
			name:          "scrape interval same as lookback delta",
			lookbackDelta: model.Duration(10 * time.Minute),
			expectError:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Non-fatal linting.
			code := CheckConfig(false, false, NewConfigLintConfig(LintOptionTooLongScrapeInterval, false, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			require.Equal(t, successExitCode, code, "Non-fatal linting should return success")
			// Fatal linting.
			code = CheckConfig(false, false, NewConfigLintConfig(LintOptionTooLongScrapeInterval, true, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			if tc.expectError {
				require.Equal(t, lintErrExitCode, code, "Fatal linting should return error")
			} else {
				require.Equal(t, successExitCode, code, "Fatal linting should return success when there are no problems")
			}
			// Check syntax only, no linting.
			code = CheckConfig(false, true, NewConfigLintConfig(LintOptionTooLongScrapeInterval, true, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			require.Equal(t, successExitCode, code, "Fatal linting should return success when checking syntax only")
			// Lint option "none" should disable linting.
			code = CheckConfig(false, false, NewConfigLintConfig(lintOptionNone+","+LintOptionTooLongScrapeInterval, true, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			require.Equal(t, successExitCode, code, `Fatal linting should return success when lint option "none" is specified`)
		})
	}
}

func TestCheckScrapeConfigsWithOutput(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lookbackDelta model.Duration
		expectError   bool
	}{
		{
			name:          "scrape interval less than lookback delta",
			lookbackDelta: model.Duration(11 * time.Minute),
			expectError:   false,
		},
		{
			name:          "scrape interval greater than lookback delta",
			lookbackDelta: model.Duration(5 * time.Minute),
			expectError:   true,
		},
		{
			name:          "scrape interval same as lookback delta",
			lookbackDelta: model.Duration(10 * time.Minute),
			expectError:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Non-fatal linting.
			code, output := CheckConfigWithOutput(false, false, NewConfigLintConfig(LintOptionTooLongScrapeInterval, false, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			require.Equal(t, successExitCode, code, "Non-fatal linting should return success")
			require.NotEmpty(t, output, "Non-fatal linting should produce output")
			// Fatal linting.
			code, output = CheckConfigWithOutput(false, false, NewConfigLintConfig(LintOptionTooLongScrapeInterval, true, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			if tc.expectError {
				require.Equal(t, lintErrExitCode, code, "Fatal linting should return error")
			} else {
				require.Equal(t, successExitCode, code, "Fatal linting should return success when there are no problems")
			}
			require.NotEmpty(t, output, "Non-fatal linting should produce output")
			// Check syntax only, no linting.
			code, output = CheckConfigWithOutput(false, true, NewConfigLintConfig(LintOptionTooLongScrapeInterval, true, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			require.Equal(t, successExitCode, code, "Fatal linting should return success when checking syntax only")
			require.NotEmpty(t, output, "Non-fatal linting should produce output")
			// Lint option "none" should disable linting.
			code, output = CheckConfigWithOutput(false, false, NewConfigLintConfig(lintOptionNone+","+LintOptionTooLongScrapeInterval, true, false, tc.lookbackDelta), "./testdata/prometheus-config.lint.too_long_scrape_interval.yml")
			require.Equal(t, successExitCode, code, `Fatal linting should return success when lint option "none" is specified`)
			require.NotEmpty(t, output, "Non-fatal linting should produce output")
		})
	}
}
