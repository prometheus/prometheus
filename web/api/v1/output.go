// Copyright 2020 The Prometheus Authors
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

package v1

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/prometheus/prometheus/promql"
)

// modifier is a function that alters the value of a query result.
type modifier struct {
	name  string
	apply func([]string, *promql.Result) (*promql.Result, error)
}

// modifiers is an ordered list of registered modifiers.
var modifiers = []modifier{
	{
		name:  "limit",
		apply: limitModifier,
	},
}

func applyModifiers(r *http.Request, res *promql.Result) (*promql.Result, error) {
	params := r.URL.Query()
	var err error
	for _, f := range modifiers {
		if v, ok := params[f.name]; ok {
			res, err = f.apply(v, res)
			if err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}

func limitModifier(limit []string, res *promql.Result) (*promql.Result, error) {
	if l := len(limit); l != 1 {
		return res, fmt.Errorf("limit only takes 1 argument, got %d", l)
	}
	outSize, err := strconv.ParseUint(limit[0], 10, 64)
	if err != nil {
		return nil, err
	}
	switch result := res.Value.(type) {
	case promql.Matrix:
		res.Value = result[:outSize]
		return res, nil
	case promql.Vector:
		res.Value = result[:outSize]
		return res, nil
	default:
		panic("unknown return type")
	}
}
