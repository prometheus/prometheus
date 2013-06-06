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

package rules

import (
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/metric"
	"time"
)

// A Rule encapsulates a vector expression which is evaluated at a specified
// interval and acted upon (currently either recorded or used for alerting).
type Rule interface {
	// Name returns the name of the rule.
	Name() string
	// EvalRaw evaluates the rule's vector expression without triggering any
	// other actions, like recording or alerting.
	EvalRaw(timestamp time.Time, storage *metric.TieredStorage) (ast.Vector, error)
	// Eval evaluates the rule, including any associated recording or alerting actions.
	Eval(timestamp time.Time, storage *metric.TieredStorage) (ast.Vector, error)
	// ToDotGraph returns a Graphviz dot graph of the rule.
	ToDotGraph() string
	// String returns a human-readable string representation of the rule.
	String() string
}
