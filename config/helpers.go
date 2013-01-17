package config

import (
	"fmt"
	"github.com/matttproud/prometheus/model"
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
