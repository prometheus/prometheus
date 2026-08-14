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
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/scrape"
)

type sdCheckResult struct {
	DiscoveredLabels labels.Labels `json:"discoveredLabels"`
	Labels           labels.Labels `json:"labels"`
	Error            error         `json:"error,omitempty"`
}

// CheckSD performs service discovery for the given job name and reports the results.
func CheckSD(sdConfigFiles, sdJobName string, sdTimeout time.Duration, _ prometheus.Registerer) int {
	logger := promslog.New(&promslog.Config{})

	cfg, err := config.LoadFile(sdConfigFiles, false, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot load config", err)
		return failureExitCode
	}

	var scrapeConfig *config.ScrapeConfig
	scfgs, err := cfg.GetScrapeConfigs()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot load scrape configs", err)
		return failureExitCode
	}

	jobs := []string{}
	jobMatched := false
	for _, v := range scfgs {
		jobs = append(jobs, v.JobName)
		if v.JobName == sdJobName {
			jobMatched = true
			scrapeConfig = v
			break
		}
	}

	if !jobMatched {
		fmt.Fprintf(os.Stderr, "Job %s not found. Select one of:\n", sdJobName)
		for _, job := range jobs {
			fmt.Fprintf(os.Stderr, "\t%s\n", job)
		}
		return failureExitCode
	}

	targetGroupChan := make(chan []*targetgroup.Group)
	ctx, cancel := context.WithTimeout(context.Background(), sdTimeout)
	defer cancel()

	for _, cfg := range scrapeConfig.ServiceDiscoveryConfigs {
		reg := prometheus.NewRegistry()
		refreshMetrics := discovery.NewRefreshMetrics(reg)
		metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
		err := metrics.Register()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not register service discovery metrics", err)
			return failureExitCode
		}

		d, err := cfg.NewDiscoverer(discovery.DiscovererOptions{Logger: logger, Metrics: metrics})
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not create new discoverer", err)
			return failureExitCode
		}
		go func() {
			d.Run(ctx, targetGroupChan)
			metrics.Unregister()
			refreshMetrics.Unregister()
		}()
	}

	var targetGroups []*targetgroup.Group
	sdCheckResults := make(map[string][]*targetgroup.Group)
outerLoop:
	for {
		select {
		case targetGroups = <-targetGroupChan:
			for _, tg := range targetGroups {
				sdCheckResults[tg.Source] = append(sdCheckResults[tg.Source], tg)
			}
		case <-ctx.Done():
			break outerLoop
		}
	}
	results := []sdCheckResult{}
	for _, tgs := range sdCheckResults {
		results = append(results, getSDCheckResult(tgs, scrapeConfig)...)
	}

	res, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not marshal result json: %s", err)
		return failureExitCode
	}

	fmt.Printf("%s", res)
	return successExitCode
}

func getSDCheckResult(targetGroups []*targetgroup.Group, scrapeConfig *config.ScrapeConfig) []sdCheckResult {
	sdCheckResults := []sdCheckResult{}
	lb := labels.NewBuilder(labels.EmptyLabels())
	for _, targetGroup := range targetGroups {
		for _, target := range targetGroup.Targets {
			lb.Reset(labels.EmptyLabels())

			for name, value := range target {
				lb.Set(string(name), string(value))
			}

			for name, value := range targetGroup.Labels {
				if _, ok := target[name]; !ok {
					lb.Set(string(name), string(value))
				}
			}

			scrape.PopulateDiscoveredLabels(lb, scrapeConfig, target, targetGroup.Labels)
			orig := lb.Labels()
			res, err := scrape.PopulateLabels(lb, scrapeConfig, target, targetGroup.Labels)
			result := sdCheckResult{
				DiscoveredLabels: orig,
				Labels:           res,
				Error:            err,
			}

			duplicateRes := false
			for _, sdCheckRes := range sdCheckResults {
				if cmp.Equal(sdCheckRes, result, cmp.Comparer(labels.Equal)) {
					duplicateRes = true
					break
				}
			}

			if !duplicateRes {
				sdCheckResults = append(sdCheckResults, result)
			}
		}
	}
	return sdCheckResults
}
