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

package rules

import (
	"context"
	"net/url"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
)

// RuleHealth describes the health state of a rule.
type RuleHealth string

// The possible health states of a rule based on the last execution.
const (
	HealthUnknown RuleHealth = "unknown"
	HealthGood    RuleHealth = "ok"
	HealthBad     RuleHealth = "err"
)

// A Rule encapsulates a vector expression which is evaluated at a specified
// interval and acted upon (currently either recorded or used for alerting).
type Rule interface {
	Name() string
	// Labels of the rule.
	Labels() labels.Labels
	// Eval evaluates the rule, including any associated recording or alerting actions.
	Eval(context.Context, time.Time, QueryFunc, *url.URL, int) (promql.Vector, error)
	// String returns a human-readable string representation of the rule.
	String() string
	// Query returns the rule query expression.
	Query() parser.Expr
	// SetLastError sets the current error experienced by the rule.
	SetLastError(error)
	// LastError returns the last error experienced by the rule.
	LastError() error
	// SetHealth sets the current health of the rule.
	SetHealth(RuleHealth)
	// Health returns the current health of the rule.
	Health() RuleHealth
	SetEvaluationDuration(time.Duration)
	// GetEvaluationDuration returns last evaluation duration.
	// NOTE: Used dynamically by rules.html template.
	GetEvaluationDuration() time.Duration
	SetEvaluationTimestamp(time.Time)
	// GetEvaluationTimestamp returns last evaluation timestamp.
	// NOTE: Used dynamically by rules.html template.
	GetEvaluationTimestamp() time.Time
}
