// Copyright 2023 The Prometheus Authors
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
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "github.com/prometheus/prometheus/plugins" // Register plugins.
)

func newAPI(url *url.URL, roundTripper http.RoundTripper, headers map[string]string) (v1.API, error) {
	if url.Scheme == "" {
		url.Scheme = "http"
	}
	config := api.Config{
		Address:      url.String(),
		RoundTripper: roundTripper,
	}

	if len(headers) > 0 {
		config.RoundTripper = promhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			for key, value := range headers {
				req.Header.Add(key, value)
			}
			return roundTripper.RoundTrip(req)
		})
	}

	// Create new client.
	client, err := api.NewClient(config)
	if err != nil {
		return nil, err
	}

	api := v1.NewAPI(client)
	return api, nil
}

// QueryInstant performs an instant query against a Prometheus server.
func QueryInstant(url *url.URL, roundTripper http.RoundTripper, query, evalTime string, p printer) int {
	api, err := newAPI(url, roundTripper, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	eTime := time.Now()
	if evalTime != "" {
		eTime, err = parseTime(evalTime)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing evaluation time:", err)
			return failureExitCode
		}
	}

	// Run query against client.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.Query(ctx, query, eTime) // Ignoring warnings for now.
	cancel()
	if err != nil {
		return handleAPIError(err)
	}

	p.printValue(val)

	return successExitCode
}

// QueryRange performs a range query against a Prometheus server.
func QueryRange(url *url.URL, roundTripper http.RoundTripper, headers map[string]string, query, start, end string, step time.Duration, p printer) int {
	api, err := newAPI(url, roundTripper, headers)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	var stime, etime time.Time

	if end == "" {
		etime = time.Now()
	} else {
		etime, err = parseTime(end)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing end time:", err)
			return failureExitCode
		}
	}

	if start == "" {
		stime = etime.Add(-5 * time.Minute)
	} else {
		stime, err = parseTime(start)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing start time:", err)
			return failureExitCode
		}
	}

	if !stime.Before(etime) {
		fmt.Fprintln(os.Stderr, "start time is not before end time")
		return failureExitCode
	}

	if step == 0 {
		resolution := math.Max(math.Floor(etime.Sub(stime).Seconds()/250), 1)
		// Convert seconds to nanoseconds such that time.Duration parses correctly.
		step = time.Duration(resolution) * time.Second
	}

	// Run query against client.
	r := v1.Range{Start: stime, End: etime, Step: step}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.QueryRange(ctx, query, r) // Ignoring warnings for now.
	cancel()

	if err != nil {
		return handleAPIError(err)
	}

	p.printValue(val)
	return successExitCode
}

// QuerySeries queries for a series against a Prometheus server.
func QuerySeries(url *url.URL, roundTripper http.RoundTripper, matchers []string, start, end string, p printer) int {
	api, err := newAPI(url, roundTripper, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	stime, etime, err := parseStartTimeAndEndTime(start, end)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// Run query against client.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.Series(ctx, matchers, stime, etime) // Ignoring warnings for now.
	cancel()

	if err != nil {
		return handleAPIError(err)
	}

	p.printSeries(val)
	return successExitCode
}

// QueryLabels queries for label values against a Prometheus server.
func QueryLabels(url *url.URL, roundTripper http.RoundTripper, matchers []string, name, start, end string, p printer) int {
	api, err := newAPI(url, roundTripper, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	stime, etime, err := parseStartTimeAndEndTime(start, end)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// Run query against client.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, warn, err := api.LabelValues(ctx, name, matchers, stime, etime)
	cancel()

	for _, v := range warn {
		fmt.Fprintln(os.Stderr, "query warning:", v)
	}
	if err != nil {
		return handleAPIError(err)
	}

	p.printLabelValues(val)
	return successExitCode
}

func handleAPIError(err error) int {
	var apiErr *v1.Error
	if errors.As(err, &apiErr) && apiErr.Detail != "" {
		fmt.Fprintf(os.Stderr, "query error: %v (detail: %s)\n", apiErr, strings.TrimSpace(apiErr.Detail))
	} else {
		fmt.Fprintln(os.Stderr, "query error:", err)
	}

	return failureExitCode
}

func parseStartTimeAndEndTime(start, end string) (time.Time, time.Time, error) {
	var (
		minTime = time.Now().Add(-9999 * time.Hour)
		maxTime = time.Now().Add(9999 * time.Hour)
		err     error
	)

	stime := minTime
	etime := maxTime

	if start != "" {
		stime, err = parseTime(start)
		if err != nil {
			return stime, etime, fmt.Errorf("error parsing start time: %w", err)
		}
	}

	if end != "" {
		etime, err = parseTime(end)
		if err != nil {
			return stime, etime, fmt.Errorf("error parsing end time: %w", err)
		}
	}
	return stime, etime, nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
}
