/*
Copyright 2016 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package gax is a snapshot from github.com/googleapis/gax-go/v2 with minor modifications.
package gax

import (
	"time"

	"google.golang.org/grpc/codes"
)

// CallOption is a generic interface for modifying the behavior of outbound calls.
type CallOption interface {
	Resolve(*CallSettings)
}

type callOptions []CallOption

// Resolve resolves all call options individually.
func (opts callOptions) Resolve(s *CallSettings) *CallSettings {
	for _, opt := range opts {
		opt.Resolve(s)
	}
	return s
}

// CallSettings encapsulates the call settings for a particular API call.
type CallSettings struct {
	Timeout       time.Duration
	RetrySettings RetrySettings
}

// RetrySettings are per-call configurable settings for retrying upon transient failure.
type RetrySettings struct {
	RetryCodes      map[codes.Code]bool
	BackoffSettings BackoffSettings
}

// BackoffSettings are parameters to the exponential backoff algorithm for retrying.
type BackoffSettings struct {
	DelayTimeoutSettings MultipliableDuration
	RPCTimeoutSettings   MultipliableDuration
}

// MultipliableDuration defines parameters for backoff settings.
type MultipliableDuration struct {
	Initial    time.Duration
	Max        time.Duration
	Multiplier float64
}

// Resolve merges the receiver CallSettings into the given CallSettings.
func (w CallSettings) Resolve(s *CallSettings) {
	s.Timeout = w.Timeout
	s.RetrySettings = w.RetrySettings

	s.RetrySettings.RetryCodes = make(map[codes.Code]bool, len(w.RetrySettings.RetryCodes))
	for key, value := range w.RetrySettings.RetryCodes {
		s.RetrySettings.RetryCodes[key] = value
	}
}

type withRetryCodes []codes.Code

func (w withRetryCodes) Resolve(s *CallSettings) {
	s.RetrySettings.RetryCodes = make(map[codes.Code]bool)
	for _, code := range w {
		s.RetrySettings.RetryCodes[code] = true
	}
}

// WithRetryCodes sets a list of Google API canonical error codes upon which a
// retry should be attempted.
func WithRetryCodes(retryCodes []codes.Code) CallOption {
	return withRetryCodes(retryCodes)
}

type withDelayTimeoutSettings MultipliableDuration

func (w withDelayTimeoutSettings) Resolve(s *CallSettings) {
	s.RetrySettings.BackoffSettings.DelayTimeoutSettings = MultipliableDuration(w)
}

// WithDelayTimeoutSettings specifies:
// - The initial delay time, in milliseconds, between the completion of
//   the first failed request and the initiation of the first retrying
//   request.
// - The multiplier by which to increase the delay time between the
//   completion of failed requests, and the initiation of the subsequent
//   retrying request.
// - The maximum delay time, in milliseconds, between requests. When this
//   value is reached, `RetryDelayMultiplier` will no longer be used to
//   increase delay time.
func WithDelayTimeoutSettings(initial time.Duration, max time.Duration, multiplier float64) CallOption {
	return withDelayTimeoutSettings(MultipliableDuration{initial, max, multiplier})
}
