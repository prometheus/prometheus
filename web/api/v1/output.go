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

const (
	limitParam = "limit"
)

func validateModifiers(r *http.Request) error {
	params := r.URL.Query()
	if p, ok := params[limitParam]; ok {
		if l := len(p); l != 1 {
			return fmt.Errorf("limit only takes 1 argument, got %d", l)
		}
	}
	return nil
}

func applyModifiers(r *http.Request, res *promql.Result) (*promql.Result, error) {
	params := r.URL.Query()
	if p, ok := params[limitParam]; ok {
		outSize, err := strconv.ParseUint(p[0], 10, 64)
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
	return res, nil
}
