// Copyright 2021 The Prometheus Authors
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
	"reflect"
	"time"

	"github.com/go-kit/log"

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
func CheckSD(sdConfigFiles, sdJobName string, sdTimeout time.Duration, noDefaultScrapePort bool) int {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))

	cfg, err := config.LoadFile(sdConfigFiles, false, false, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot load config", err)
		return failureExitCode
	}

	var scrapeConfig *config.ScrapeConfig
	jobs := []string{}
	jobMatched := false
	for _, v := range cfg.ScrapeConfigs {
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
		d, err := cfg.NewDiscoverer(discovery.DiscovererOptions{Logger: logger})
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not create new discoverer", err)
			return failureExitCode
		}
		go d.Run(ctx, targetGroupChan)
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
		results = append(results, getSDCheckResult(tgs, scrapeConfig, noDefaultScrapePort)...)
	}

	res, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not marshal result json: %s", err)
		return failureExitCode
	}

	fmt.Printf("%s", res)
	return successExitCode
}

func getSDCheckResult(targetGroups []*targetgroup.Group, scrapeConfig *config.ScrapeConfig, noDefaultScrapePort bool) []sdCheckResult {
	sdCheckResults := []sdCheckResult{}
	for _, targetGroup := range targetGroups {
		for _, target := range targetGroup.Targets {
			labelSlice := make([]labels.Label, 0, len(target)+len(targetGroup.Labels))

			for name, value := range target {
				labelSlice = append(labelSlice, labels.Label{Name: string(name), Value: string(value)})
			}

			for name, value := range targetGroup.Labels {
				if _, ok := target[name]; !ok {
					labelSlice = append(labelSlice, labels.Label{Name: string(name), Value: string(value)})
				}
			}

			targetLabels := labels.New(labelSlice...)
			res, orig, err := scrape.PopulateLabels(targetLabels, scrapeConfig, noDefaultScrapePort)
			result := sdCheckResult{
				DiscoveredLabels: orig,
				Labels:           res,
				Error:            err,
			}

			duplicateRes := false
			for _, sdCheckRes := range sdCheckResults {
				if reflect.DeepEqual(sdCheckRes, result) {
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
