// Copyright 2024 The Prometheus Authors
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

package promql

import (
	"errors"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

func TestRecoverEvaluatorRuntime(t *testing.T) {
	var output []interface{}
	logger := log.Logger(log.LoggerFunc(func(keyvals ...interface{}) error {
		output = append(output, keyvals...)
		return nil
	}))
	ev := &evaluator{logger: logger}

	expr, _ := parser.ParseExpr("sum(up)")

	var err error

	defer func() {
		require.EqualError(t, err, "unexpected error: runtime error: index out of range [123] with length 0")
		require.Contains(t, output, "sum(up)")
	}()
	defer ev.recover(expr, nil, &err)

	// Cause a runtime panic.
	var a []int
	a[123] = 1
}

func TestRecoverEvaluatorError(t *testing.T) {
	ev := &evaluator{logger: log.NewNopLogger()}
	var err error

	e := errors.New("custom error")

	defer func() {
		require.EqualError(t, err, e.Error())
	}()
	defer ev.recover(nil, nil, &err)

	panic(e)
}

func TestRecoverEvaluatorErrorWithWarnings(t *testing.T) {
	ev := &evaluator{logger: log.NewNopLogger()}
	var err error
	var ws annotations.Annotations

	warnings := annotations.New().Add(errors.New("custom warning"))
	e := errWithWarnings{
		err:      errors.New("custom error"),
		warnings: warnings,
	}

	defer func() {
		require.EqualError(t, err, e.Error())
		require.Equal(t, warnings, ws, "wrong warning message")
	}()
	defer ev.recover(nil, &ws, &err)

	panic(e)
}
