// Copyright 2017, 2018 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package base provides common storage code.
package base

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/prompb"
)

// Storage represents generic storage.
type Storage interface {
	// Read runs queries in the storage and returns the same amount of matrixes.
	// Event if they are empty, they must be present in the returned slice.
	Read(context.Context, *prompb.Query) (*prompb.QueryResult, error)

	// // Write puts data into storage.
	// Write(context.Context, *prompb.WriteRequest) error

	// prometheus.Collector
}

// Query represents query against stored data.
type Query struct {
	Start    model.Time
	End      model.Time
	Matchers Matchers
}

func (q Query) String() string {
	return fmt.Sprintf("[%d,%d,%s]", q.Start, q.End, q.Matchers)
}

type MatchType int

const (
	MatchEqual MatchType = iota
	MatchNotEqual
	MatchRegexp
	MatchNotRegexp
)

func (m MatchType) String() string {
	switch m {
	case MatchEqual:
		return "="
	case MatchNotEqual:
		return "!="
	case MatchRegexp:
		return "=~"
	case MatchNotRegexp:
		return "!~"
	default:
		panic("unknown match type")
	}
}

type Matcher struct {
	Name  string
	Type  MatchType
	Value string
	re    *regexp.Regexp
}

func (m Matcher) String() string {
	return fmt.Sprintf("%s%s%q", m.Name, m.Type, m.Value)
}

type Matchers []Matcher

var emptyLabel = &prompb.Label{}

func (ms Matchers) String() string {
	res := make([]string, len(ms))
	for i, m := range ms {
		res[i] = m.String()
	}
	return "{" + strings.Join(res, ",") + "}"
}

func (ms Matchers) MatchLabels(labels []*prompb.Label) bool {
	for _, m := range ms {
		if (m.re == nil) && (m.Type == MatchRegexp || m.Type == MatchNotRegexp) {
			m.re = regexp.MustCompile("^(?:" + m.Value + ")$")
		}

		label := emptyLabel
		for _, l := range labels {
			if m.Name == l.Name {
				label = l
				break
			}
		}

		// return false if not matches, continue to the next matcher otherwise
		switch m.Type {
		case MatchEqual:
			if m.Value != label.Value {
				return false
			}
		case MatchNotEqual:
			if m.Value == label.Value {
				return false
			}
		case MatchRegexp:
			if !m.re.MatchString(label.Value) {
				return false
			}
		case MatchNotRegexp:
			if m.re.MatchString(label.Value) {
				return false
			}
		default:
			panic("unknown match type")
		}
	}

	return true
}

// check interfaces
var (
	_ fmt.Stringer = Query{}
	_ fmt.Stringer = MatchType(0)
	_ fmt.Stringer = Matcher{}
	_ fmt.Stringer = Matchers{}
)
