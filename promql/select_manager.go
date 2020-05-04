// Copyright 2019 The Prometheus Authors
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

package promql

import (
	"context"
	"sync"

	"github.com/prometheus/prometheus/pkg/gate"
	"github.com/prometheus/prometheus/storage"
)

type selectManagerFunc func() Selector

func (f selectManagerFunc) NewSelector() Selector { return f() }

// A sequentialSelector processes added select requests immediately by the given order.
type sequentialSelector struct {
	warnings storage.Warnings
	err      error
}

// NewSequentialSelectManager creates a new sequential select manager.
func NewSequentialSelectManager() SelectManager {
	return selectManagerFunc(func() Selector { return &sequentialSelector{} })
}

func (qm *sequentialSelector) Select(req func() (storage.Warnings, error)) error {
	wrn, err := req()
	qm.warnings, qm.err = append(qm.warnings, wrn...), err
	return err
}

func (qm *sequentialSelector) Run(ctx context.Context) (storage.Warnings, error) {
	warnings, err := qm.warnings, qm.err
	qm.warnings, qm.err = nil, nil
	return warnings, err
}

// A concurrentSelector processes added select requests concurrently.
// It limits amount of requests executed at the same time by the given concurrency limit.
type concurrentSelector struct {
	selectGate *gate.Gate
	requests   []func() (storage.Warnings, error)
}

// NewConcurrentSelectManager creates a new concurrent select manager.
func NewConcurrentSelectManager(concurrencyLimit int) SelectManager {
	return selectManagerFunc(func() Selector { return &concurrentSelector{selectGate: gate.New(concurrencyLimit)} })
}

func (qm *concurrentSelector) Select(req func() (storage.Warnings, error)) error {
	qm.requests = append(qm.requests, req)
	return nil
}

func (qm *concurrentSelector) Run(ctx context.Context) (storage.Warnings, error) {
	defer func() { qm.requests = nil }()

	reqSize := len(qm.requests)
	if reqSize == 0 {
		return nil, nil
	}
	if reqSize == 1 {
		return qm.requests[0]()
	}

	type response struct {
		warnings storage.Warnings
		err      error
	}

	var (
		wg        sync.WaitGroup
		responses = make(chan response, reqSize)
	)

	wg.Add(reqSize)
	for _, req := range qm.requests {
		go func(req func() (storage.Warnings, error)) {
			defer wg.Done()

			if err := qm.selectGate.Start(ctx); err != nil {
				responses <- response{nil, err}
				return
			}
			defer qm.selectGate.Done()

			if wrn, err := req(); err != nil {
				responses <- response{wrn, err}
			}
		}(req)
	}
	wg.Wait()
	close(responses)

	var (
		warnings storage.Warnings
		err      error
	)
	for res := range responses {
		if res.warnings != nil {
			warnings = append(warnings, res.warnings...)
		}
		if err != nil {
			err = res.err
		}
	}
	return warnings, err
}
