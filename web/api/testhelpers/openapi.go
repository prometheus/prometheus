// Copyright The Prometheus Authors
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

// This file provides OpenAPI-specific test utilities for validating spec compliance.
package testhelpers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/stretchr/testify/require"
)

var (
	openAPIValidator     validator.Validator
	openAPIValidatorOnce sync.Once
	openAPIValidatorErr  error
)

// loadOpenAPIValidator loads and caches the OpenAPI validator from the golden file.
func loadOpenAPIValidator() (validator.Validator, error) {
	openAPIValidatorOnce.Do(func() {
		goldenPath := filepath.Join("testdata", "openapi_golden.yaml")
		specBytes, err := os.ReadFile(goldenPath)
		if err != nil {
			openAPIValidatorErr = fmt.Errorf("failed to read OpenAPI spec from %s: %w", goldenPath, err)
			return
		}

		doc, err := libopenapi.NewDocument(specBytes)
		if err != nil {
			openAPIValidatorErr = fmt.Errorf("failed to parse OpenAPI document: %w", err)
			return
		}

		v, errs := validator.NewValidator(doc)
		if len(errs) > 0 {
			openAPIValidatorErr = fmt.Errorf("failed to create OpenAPI validator: %v", errs)
			return
		}

		openAPIValidator = v
	})

	return openAPIValidator, openAPIValidatorErr
}

// ValidateOpenAPI validates the request and response against the OpenAPI specification.
// Returns the response for chaining.
func (r *Response) ValidateOpenAPI() *Response {
	r.t.Helper()

	// Load the validator (cached after first call).
	v, err := loadOpenAPIValidator()
	require.NoError(r.t, err, "failed to load OpenAPI validator")

	// Validate the request.
	if r.request != nil {
		r.validateRequest(v)
	}

	// Validate the response.
	r.validateResponse(v)

	return r
}

// validateRequest validates the HTTP request against the OpenAPI spec.
func (r *Response) validateRequest(v validator.Validator) {
	r.t.Helper()

	// Create a validation request from the original request.
	validationReq := &http.Request{
		Method: r.request.Method,
		URL:    r.request.URL,
		Header: r.request.Header,
		Body:   io.NopCloser(bytes.NewReader(r.requestBody)),
	}

	// Validate the request.
	valid, errors := v.ValidateHttpRequest(validationReq)
	if !valid {
		var errorMessages []string
		for _, e := range errors {
			errorMessages = append(errorMessages, e.Error())
		}
		require.Fail(r.t, "OpenAPI request validation failed",
			"Request to %s %s failed validation:\n%v",
			r.request.Method, r.request.URL.Path, errorMessages)
	}
}

// validateResponse validates the HTTP response against the OpenAPI spec.
func (r *Response) validateResponse(v validator.Validator) {
	r.t.Helper()

	// Create a validation request (needed for response validation context).
	validationReq := &http.Request{
		Method: r.request.Method,
		URL:    r.request.URL,
		Header: r.request.Header,
	}

	// Create a response for validation.
	validationResp := &http.Response{
		StatusCode: r.StatusCode,
		Header:     r.responseHeader,
		Body:       io.NopCloser(bytes.NewReader([]byte(r.Body))),
		Request:    validationReq,
	}

	// Validate the response.
	valid, errors := v.ValidateHttpResponse(validationReq, validationResp)
	if !valid {
		var errorMessages []string
		for _, e := range errors {
			errorMessages = append(errorMessages, e.Error())
		}
		require.Fail(r.t, "OpenAPI response validation failed",
			"Response from %s %s (status %d) failed validation:\n%v",
			r.request.Method, r.request.URL.Path, r.StatusCode, errorMessages)
	}
}
