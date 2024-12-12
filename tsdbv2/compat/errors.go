// Copyright 2024 The Prometheus Authors
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

package compat

import "errors"

var (
	// ErrInvalidSample is returned if an appended sample is not valid and can't
	// be ingested.
	ErrInvalidSample = errors.New("invalid sample")
	// ErrInvalidExemplar is returned if an appended exemplar is not valid and can't
	// be ingested.
	ErrInvalidExemplar = errors.New("invalid exemplar")
	// ErrAppenderClosed is returned if an appender has already be successfully
	// rolled back or committed.
	ErrAppenderClosed = errors.New("appender closed")
)
