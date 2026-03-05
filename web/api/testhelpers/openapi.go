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
	"strings"
	"sync"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	valerrors "github.com/pb33f/libopenapi-validator/errors"
	"github.com/stretchr/testify/require"
)

var (
	openAPIValidator31   validator.Validator
	openAPIValidator32   validator.Validator
	openAPIValidatorOnce sync.Once
	openAPIValidatorErr  error
)

// loadOpenAPIValidators loads and caches both OpenAPI 3.1 and 3.2 validators from golden files.
func loadOpenAPIValidators() (v31, v32 validator.Validator, err error) {
	openAPIValidatorOnce.Do(func() {
		// Load OpenAPI 3.1 validator.
		goldenPath31 := filepath.Join("testdata", "openapi_3.1_golden.yaml")
		specBytes31, err := os.ReadFile(goldenPath31)
		if err != nil {
			openAPIValidatorErr = fmt.Errorf("failed to read OpenAPI 3.1 spec from %s: %w", goldenPath31, err)
			return
		}

		doc31, err := libopenapi.NewDocument(specBytes31)
		if err != nil {
			openAPIValidatorErr = fmt.Errorf("failed to parse OpenAPI 3.1 document: %w", err)
			return
		}

		v31, errs := validator.NewValidator(doc31)
		if len(errs) > 0 {
			openAPIValidatorErr = fmt.Errorf("failed to create OpenAPI 3.1 validator: %v", errs)
			return
		}

		openAPIValidator31 = v31

		// Load OpenAPI 3.2 validator.
		goldenPath32 := filepath.Join("testdata", "openapi_3.2_golden.yaml")
		specBytes32, err := os.ReadFile(goldenPath32)
		if err != nil {
			openAPIValidatorErr = fmt.Errorf("failed to read OpenAPI 3.2 spec from %s: %w", goldenPath32, err)
			return
		}

		doc32, err := libopenapi.NewDocument(specBytes32)
		if err != nil {
			openAPIValidatorErr = fmt.Errorf("failed to parse OpenAPI 3.2 document: %w", err)
			return
		}

		v32, errs := validator.NewValidator(doc32)
		if len(errs) > 0 {
			openAPIValidatorErr = fmt.Errorf("failed to create OpenAPI 3.2 validator: %v", errs)
			return
		}

		openAPIValidator32 = v32
	})

	if openAPIValidatorErr != nil {
		return nil, nil, openAPIValidatorErr
	}

	return openAPIValidator31, openAPIValidator32, nil
}

// ValidateOpenAPI validates the request and response against both OpenAPI 3.1 and 3.2 specifications.
// This ensures API endpoints are compatible with both OpenAPI versions.
// Returns the response for chaining.
func (r *Response) ValidateOpenAPI() *Response {
	r.t.Helper()

	// Load both validators (cached after first call).
	v31, v32, err := loadOpenAPIValidators()
	require.NoError(r.t, err, "failed to load OpenAPI validators")

	// Validate against OpenAPI 3.1 spec.
	if r.request != nil {
		r.validateRequestWithVersion(v31, "3.1")
	}
	r.validateResponseWithVersion(v31, "3.1")

	// Validate against OpenAPI 3.2 spec.
	if r.request != nil {
		r.validateRequestWithVersion(v32, "3.2")
	}
	r.validateResponseWithVersion(v32, "3.2")

	return r
}

// validateRequestWithVersion validates the HTTP request against a specific OpenAPI version's spec.
func (r *Response) validateRequestWithVersion(v validator.Validator, version string) {
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
		// Check if the error is because the path doesn't exist in this version.
		// Some endpoints (like /notifications/live) only exist in 3.2, not 3.1.
		if isPathNotFoundError(errors) && version == "3.1" && strings.Contains(r.request.URL.Path, "/notifications/live") {
			// Expected: /notifications/live is only in OpenAPI 3.2.
			return
		}

		var errorMessages []string
		for _, e := range errors {
			errorMessages = append(errorMessages, e.Error())
		}
		require.Fail(r.t, fmt.Sprintf("OpenAPI %s request validation failed", version),
			"Request to %s %s failed OpenAPI %s validation:\n%v",
			r.request.Method, r.request.URL.Path, version, errorMessages)
	}
}

// validateResponseWithVersion validates the HTTP response against a specific OpenAPI version's spec.
func (r *Response) validateResponseWithVersion(v validator.Validator, version string) {
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
		// Check if the error is because the path doesn't exist in this version.
		// Some endpoints (like /notifications/live) only exist in 3.2, not 3.1.
		if isPathNotFoundError(errors) && version == "3.1" && strings.Contains(r.request.URL.Path, "/notifications/live") {
			// Expected: /notifications/live is only in OpenAPI 3.2.
			return
		}

		var errorMessages []string
		for _, e := range errors {
			errorMessages = append(errorMessages, e.Error())
		}
		require.Fail(r.t, fmt.Sprintf("OpenAPI %s response validation failed", version),
			"Response from %s %s (status %d) failed OpenAPI %s validation:\n%v",
			r.request.Method, r.request.URL.Path, r.StatusCode, version, errorMessages)
	}
}

// isPathNotFoundError checks if the validation errors indicate a path was not found in the spec.
func isPathNotFoundError(errors []*valerrors.ValidationError) bool {
	for _, err := range errors {
		errStr := err.Error()
		// Check for common "path not found" error messages from libopenapi-validator.
		if strings.Contains(errStr, "path") && (strings.Contains(errStr, "not found") || strings.Contains(errStr, "does not exist")) {
			return true
		}
		if strings.Contains(errStr, "GET /notifications/live") || strings.Contains(errStr, "/notifications/live not found") {
			return true
		}
	}
	return false
}
