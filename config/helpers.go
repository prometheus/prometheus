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
)

// Unfortunately, more global variables that are needed for parsing.
var tmpJobOptions = map[string]string{}
var tmpJobTargets = []Targets{}
var tmpTargetEndpoints = []string{}
var tmpTargetLabels = model.LabelSet{}

func configError(error string, v ...interface{}) {
	message := fmt.Sprintf(error, v...)
	// TODO: Don't just die here. Pass errors back all the way to the caller
	// instead.
	log.Fatalf("Line %v, char %v: %s", yyline, yypos, message)
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
