// Copyright 2013 The Prometheus Authors
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

package strutil

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var durationRE = regexp.MustCompile("^([0-9]+)([ywdhms]+)$")

// DurationToString formats a time.Duration as a string with the assumption that
// a year always has 365 days and a day always has 24h. (The former doesn't work
// in leap years, the latter is broken by DST switches, not to speak about leap
// seconds, but those are not even treated properly by the duration strings in
// the standard library.)
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

// StringToDuration parses a string into a time.Duration, assuming that a year
// always has 365d, a week 7d, a day 24h. See DurationToString for problems with
// that.
func StringToDuration(durationStr string) (duration time.Duration, err error) {
	matches := durationRE.FindStringSubmatch(durationStr)
	if len(matches) != 3 {
		err = fmt.Errorf("not a valid duration string: %q", durationStr)
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

// TableLinkForExpression creates an escaped relative link to the table view of
// the provided expression.
func TableLinkForExpression(expr string) string {
	// url.QueryEscape percent-escapes everything except spaces, for which it
	// uses "+". However, in the non-query part of a URI, only percent-escaped
	// spaces are legal, so we need to manually replace "+" with "%20" after
	// query-escaping the string.
	//
	// See also:
	// http://stackoverflow.com/questions/1634271/url-encoding-the-space-character-or-20.
	urlData := url.QueryEscape(fmt.Sprintf(`[{"expr":%q,"tab":1}]`, expr))
	return fmt.Sprintf("/graph#%s", strings.Replace(urlData, "+", "%20", -1))
}

// GraphLinkForExpression creates an escaped relative link to the graph view of
// the provided expression.
func GraphLinkForExpression(expr string) string {
	urlData := url.QueryEscape(fmt.Sprintf(`[{"expr":%q,"tab":0}]`, expr))
	return fmt.Sprintf("/graph#%s", strings.Replace(urlData, "+", "%20", -1))
}
