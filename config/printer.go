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
	"github.com/prometheus/prometheus/utility"
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
	return indentStr(indent, "%v\n%v", global, strings.Join(jobs, "\n"))
}

func labelsToString(indent int, labels model.LabelSet) string {
	str := indentStr(indent, "labels {\n")
	labelStrings := []string{}
	for label, value := range labels {
		labelStrings = append(labelStrings, indentStr(indent+1, "%v = \"%v\"", label, value))
	}
	str += strings.Join(labelStrings, ",\n") + "\n"
	str += indentStr(indent, "}\n")
	return str
}

func stringListToString(indent int, list []string) string {
	listString := []string{}
	for _, item := range list {
		listString = append(listString, indentStr(indent, "\"%v\"", item))
	}
	return strings.Join(listString, ",\n") + "\n"
}

func (global *GlobalConfig) ToString(indent int) string {
	str := indentStr(indent, "global {\n")
	str += indentStr(indent+1, "scrape_interval = \"%s\"\n", utility.DurationToString(global.ScrapeInterval))
	str += indentStr(indent+1, "evaluation_interval = \"%s\"\n", utility.DurationToString(global.EvaluationInterval))
	if len(global.Labels) > 0 {
		str += labelsToString(indent+1, global.Labels)
	}
	str += indentStr(indent+1, "rule_files = [\n")
	str += stringListToString(indent+2, global.RuleFiles)
	str += indentStr(indent+1, "]\n")
	str += indentStr(indent, "}\n")
	return str
}

func (job *JobConfig) ToString(indent int) string {
	str := indentStr(indent, "job {\n")
	str += indentStr(indent+1, "name = \"%v\"\n", job.Name)
	str += indentStr(indent+1, "scrape_interval = \"%s\"\n", utility.DurationToString(job.ScrapeInterval))
	for _, targets := range job.Targets {
		str += indentStr(indent+1, "targets {\n")
		str += indentStr(indent+2, "endpoints = [\n")
		str += stringListToString(indent+3, targets.Endpoints)
		str += indentStr(indent+2, "]\n")
		if len(targets.Labels) > 0 {
			str += labelsToString(indent+2, targets.Labels)
		}
		str += indentStr(indent+1, "}\n")
	}
	str += indentStr(indent, "}\n")
	return str
}
