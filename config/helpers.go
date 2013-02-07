// Copyright 2013 Prometheus Team
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

package config

import (
	"fmt"
	"github.com/prometheus/prometheus/model"
	"log"
	"regexp"
	"strconv"
	"time"
)

// Unfortunately, more global variables that are needed for parsing.
var tmpJobOptions = map[string]string{}
var tmpJobTargets = []Targets{}
var tmpTargetEndpoints = []string{}
var tmpTargetLabels = model.LabelSet{}

func configError(error string, v ...interface{}) {
	message := fmt.Sprintf(error, v...)
	log.Fatal(fmt.Sprintf("Line %v, char %v: %s", yyline, yypos, message))
}

func PushJobOption(option string, value string) {
	tmpJobOptions[option] = value
}

func PushJobTargets() {
	targets := Targets{
		Endpoints: tmpTargetEndpoints,
		Labels:    tmpTargetLabels,
	}
	tmpJobTargets = append(tmpJobTargets, targets)
	tmpTargetLabels = model.LabelSet{}
	tmpTargetEndpoints = []string{}
}

func PushTargetEndpoints(endpoints []string) {
	for _, endpoint := range endpoints {
		tmpTargetEndpoints = append(tmpTargetEndpoints, endpoint)
	}
}

func PushTargetLabels(labels model.LabelSet) {
	for k, v := range labels {
		tmpTargetLabels[k] = v
	}
}

func PopJob() {
	if err := parsedConfig.AddJob(tmpJobOptions, tmpJobTargets); err != nil {
		configError(err.Error())
	}
	tmpJobOptions = map[string]string{}
	tmpJobTargets = []Targets{}
}

func stringToDuration(durationStr string) time.Duration {
	durationRE := regexp.MustCompile("^([0-9]+)([ywdhms]+)$")
	matches := durationRE.FindStringSubmatch(durationStr)
	if len(matches) != 3 {
		configError("Not a valid duration string: '%v'", durationStr)
	}
	value, _ := strconv.Atoi(matches[1])
	unit := matches[2]
	switch unit {
	case "y":
		value *= 60 * 60 * 24 * 365
	case "w":
		value *= 60 * 60 * 24 * 7
	case "d":
		value *= 60 * 60 * 24
	case "h":
		value *= 60 * 60
	case "m":
		value *= 60
	}
	return time.Duration(value) * time.Second
}
