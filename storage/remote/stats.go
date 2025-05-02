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

package remote

import (
	"errors"
	"net/http"
	"strconv"
)

const (
	rw20WrittenSamplesHeader    = "X-Prometheus-Remote-Write-Samples-Written"
	rw20WrittenHistogramsHeader = "X-Prometheus-Remote-Write-Histograms-Written"
	rw20WrittenExemplarsHeader  = "X-Prometheus-Remote-Write-Exemplars-Written"
)

// WriteResponseStats represents the response write statistics specified in https://github.com/prometheus/docs/pull/2486
type WriteResponseStats struct {
	// Samples represents X-Prometheus-Remote-Write-Written-Samples
	Samples int
	// Histograms represents X-Prometheus-Remote-Write-Written-Histograms
	Histograms int
	// Exemplars represents X-Prometheus-Remote-Write-Written-Exemplars
	Exemplars int

	// Confirmed means we can trust those statistics from the point of view
	// of the PRW 2.0 spec. When parsed from headers, it means we got at least one
	// response header from the Receiver to confirm those numbers, meaning it must
	// be a at least 2.0 Receiver. See ParseWriteResponseStats for details.
	Confirmed bool
}

// NoDataWritten returns true if statistics indicate no data was written.
func (s WriteResponseStats) NoDataWritten() bool {
	return (s.Samples + s.Histograms + s.Exemplars) == 0
}

// AllSamples returns both float and histogram sample numbers.
func (s WriteResponseStats) AllSamples() int {
	return s.Samples + s.Histograms
}

// Add returns the sum of this WriteResponseStats plus the given WriteResponseStats.
func (s WriteResponseStats) Add(rs WriteResponseStats) WriteResponseStats {
	s.Confirmed = rs.Confirmed
	s.Samples += rs.Samples
	s.Histograms += rs.Histograms
	s.Exemplars += rs.Exemplars
	return s
}

// SetHeaders sets response headers in a given response writer.
// Make sure to use it before http.ResponseWriter.WriteHeader and .Write.
func (s WriteResponseStats) SetHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set(rw20WrittenSamplesHeader, strconv.Itoa(s.Samples))
	h.Set(rw20WrittenHistogramsHeader, strconv.Itoa(s.Histograms))
	h.Set(rw20WrittenExemplarsHeader, strconv.Itoa(s.Exemplars))
}

// ParseWriteResponseStats returns WriteResponseStats parsed from the response headers.
//
// As per 2.0 spec, missing header means 0. However, abrupt HTTP errors, 1.0 Receivers
// or buggy 2.0 Receivers might result in no response headers specified and that
// might NOT necessarily mean nothing was written. To represent that we set
// s.Confirmed = true only when see at least on response header.
//
// Error is returned when any of the header fails to parse as int64.
func ParseWriteResponseStats(r *http.Response) (s WriteResponseStats, err error) {
	var (
		errs []error
		h    = r.Header
	)
	if v := h.Get(rw20WrittenSamplesHeader); v != "" { // Empty means zero.
		s.Confirmed = true
		if s.Samples, err = strconv.Atoi(v); err != nil {
			s.Samples = 0
			errs = append(errs, err)
		}
	}
	if v := h.Get(rw20WrittenHistogramsHeader); v != "" { // Empty means zero.
		s.Confirmed = true
		if s.Histograms, err = strconv.Atoi(v); err != nil {
			s.Histograms = 0
			errs = append(errs, err)
		}
	}
	if v := h.Get(rw20WrittenExemplarsHeader); v != "" { // Empty means zero.
		s.Confirmed = true
		if s.Exemplars, err = strconv.Atoi(v); err != nil {
			s.Exemplars = 0
			errs = append(errs, err)
		}
	}
	return s, errors.Join(errs...)
}
