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

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

// backtestServer serves a fixed range query result, standing in for a Prometheus
// server holding the historical data.
func backtestServer(t *testing.T, body string) *url.URL {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/query_range", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(body))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	return u
}

func TestBacktestAlert(t *testing.T) {
	t.Parallel()

	// The result of `http_requests < 100` at a 1m step: below the threshold from
	// 1m to 4m, back above it at 5m, below again at 6m and 7m.
	const result = `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"__name__":"http_requests","instance":"0"},
		 "values":[[60,"90"],[120,"90"],[180,"90"],[240,"90"],[360,"90"],[420,"90"]]}
	]}}`

	at := func(sec int64) time.Time { return time.Unix(sec, 0).UTC() }
	instance := map[string]string{"alertname": "Backtest", "instance": "0"}

	for _, tc := range []struct {
		name          string
		forDuration   time.Duration
		keepFiringFor time.Duration
		want          []alertPeriod
	}{
		{
			name: "no for",
			want: []alertPeriod{
				{Labels: instance, State: "firing", Start: at(60), End: at(240)},
				{Labels: instance, State: "firing", Start: at(360), End: at(420)},
			},
		},
		{
			name:        "for 2m",
			forDuration: 2 * time.Minute,
			want: []alertPeriod{
				{Labels: instance, State: "pending", Start: at(60), End: at(120)},
				{Labels: instance, State: "firing", Start: at(180), End: at(240)},
				{Labels: instance, State: "pending", Start: at(360), End: at(420)},
			},
		},
		{
			name:        "for 5m never fires",
			forDuration: 5 * time.Minute,
			want: []alertPeriod{
				{Labels: instance, State: "pending", Start: at(60), End: at(240)},
				{Labels: instance, State: "pending", Start: at(360), End: at(420)},
			},
		},
		{
			name:          "keep_firing_for bridges the gap",
			keepFiringFor: 2 * time.Minute,
			want: []alertPeriod{
				{Labels: instance, State: "firing", Start: at(60), End: at(480)},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := backtestConfig{
				name:          "Backtest",
				expr:          "http_requests < 100",
				forDuration:   tc.forDuration,
				keepFiringFor: tc.keepFiringFor,
				start:         "0",
				end:           "480",
				interval:      time.Minute,
			}

			got, err := backtestAlert(context.Background(), backtestServer(t, result), http.DefaultTransport, cfg, parser.NewParser(parser.Options{}))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestBacktestAlertCommand exercises the command through the real CLI, which is
// the only way to cover the kingpin flag wiring.
func TestBacktestAlertCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	t.Parallel()

	const result = `{"status":"success","data":{"resultType":"matrix","result":[
		{"metric":{"__name__":"http_requests","instance":"0"},
		 "values":[[60,"90"],[120,"90"],[180,"90"],[240,"90"]]}
	]}}`

	out, err := exec.Command(promtoolPath, "-test.main", "query", "backtest",
		backtestServer(t, result).String(), "http_requests < 100",
		"--for=2m", "--start=0", "--end=480", "--interval=1m",
		"--label=severity=page", "--header=X-Test=1",
	).CombinedOutput()
	require.NoError(t, err, "promtool output: %s", out)

	require.Contains(t, string(out), "pending")
	require.Contains(t, string(out), "firing")
	require.Contains(t, string(out), `severity="page"`)
}

func TestBacktestAlertErrors(t *testing.T) {
	t.Parallel()

	const empty = `{"status":"success","data":{"resultType":"matrix","result":[]}}`

	for _, tc := range []struct {
		name string
		cfg  backtestConfig
		err  string
	}{
		{
			name: "invalid expression",
			cfg:  backtestConfig{expr: "http_requests <", interval: time.Minute},
			err:  "unexpected end of input",
		},
		{
			name: "invalid start time",
			cfg:  backtestConfig{expr: "up", start: "yesterday", interval: time.Minute},
			err:  `start time: cannot parse "yesterday"`,
		},
		{
			name: "range too long for the interval",
			cfg:  backtestConfig{expr: "up", start: "0", end: "660000", interval: time.Second},
			err:  "exceeded maximum of 11000 evaluations",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := backtestAlert(context.Background(), backtestServer(t, empty), http.DefaultTransport, tc.cfg, parser.NewParser(parser.Options{}))
			require.ErrorContains(t, err, tc.err)
		})
	}
}
