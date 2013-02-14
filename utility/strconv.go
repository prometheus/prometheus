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

package utility

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var durationRE = regexp.MustCompile("^([0-9]+)([ywdhms]+)$")

func DurationToString(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	factors := map[string]int64{
		"y": 60 * 60 * 24 * 365,
		"d": 60 * 60 * 24,
		"h": 60 * 60,
		"m": 60,
		"s": 1,
	}
	unit := "s"
	switch int64(0) {
	case seconds % factors["y"]:
		unit = "y"
	case seconds % factors["d"]:
		unit = "d"
	case seconds % factors["h"]:
		unit = "h"
	case seconds % factors["m"]:
		unit = "m"
	}
	return fmt.Sprintf("%v%v", seconds/factors[unit], unit)
}

func StringToDuration(durationStr string) (duration time.Duration, err error) {
	matches := durationRE.FindStringSubmatch(durationStr)
	if len(matches) != 3 {
		err = fmt.Errorf("Not a valid duration string: '%v'", durationStr)
		return
	}
	durationSeconds, _ := strconv.Atoi(matches[1])
	duration = time.Duration(durationSeconds) * time.Second
	unit := matches[2]
	switch unit {
	case "y":
		duration *= 60 * 60 * 24 * 365
	case "w":
		duration *= 60 * 60 * 24 * 7
	case "d":
		duration *= 60 * 60 * 24
	case "h":
		duration *= 60 * 60
	case "m":
		duration *= 60
	case "s":
		duration *= 1
	default:
		panic("Invalid time unit in duration string.")
	}
	return
}
