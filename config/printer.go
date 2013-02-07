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
	"strings"
)

func indentStr(indent int, str string, v ...interface{}) string {
	indentStr := ""
	for i := 0; i < indent; i++ {
		indentStr += "\t"
	}
	return fmt.Sprintf(indentStr+str, v...)
}

func (config *Config) ToString(indent int) string {
	global := config.Global.ToString(indent)
	jobs := []string{}
	for _, job := range config.Jobs {
		jobs = append(jobs, job.ToString(indent))
	}
	return indentStr(indent, "%v\n%v\n", global, strings.Join(jobs, "\n"))
}

func labelsToString(indent int, labels model.LabelSet) string {
	str := indentStr(indent, "labels {\n")
	for label, value := range labels {
		str += indentStr(indent+1, "%v = \"%v\",\n", label, value)
	}
	str += indentStr(indent, "}\n")
	return str
}

func (global *GlobalConfig) ToString(indent int) string {
	str := indentStr(indent, "global {\n")
	str += indentStr(indent+1, "scrape_interval = \"%vs\"\n", global.ScrapeInterval)
	str += indentStr(indent+1, "evaluation_interval = \"%vs\"\n", global.EvaluationInterval)
	str += labelsToString(indent+1, global.Labels)
	str += indentStr(indent, "}\n")
	str += indentStr(indent+1, "rule_files = [\n")
	for _, ruleFile := range global.RuleFiles {
		str += indentStr(indent+2, "\"%v\",\n", ruleFile)
	}
	str += indentStr(indent+1, "]\n")
	return str
}

func (job *JobConfig) ToString(indent int) string {
	str := indentStr(indent, "job {\n")
	str += indentStr(indent+1, "job {\n")
	str += indentStr(indent+1, "name = \"%v\"\n", job.Name)
	str += indentStr(indent+1, "scrape_interval = \"%vs\"\n", job.ScrapeInterval)
	for _, targets := range job.Targets {
		str += indentStr(indent+1, "targets {\n")
		str += indentStr(indent+2, "endpoints = [\n")
		for _, endpoint := range targets.Endpoints {
			str += indentStr(indent+3, "\"%v\",\n", endpoint)
		}
		str += indentStr(indent+2, "]\n")
		str += labelsToString(indent+2, targets.Labels)
		str += indentStr(indent+1, "}\n")
	}
	str += indentStr(indent, "}\n")
	return str
}
