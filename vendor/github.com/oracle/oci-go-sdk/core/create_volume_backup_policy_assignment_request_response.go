// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// CreateVolumeBackupPolicyAssignmentRequest wrapper for the CreateVolumeBackupPolicyAssignment operation
type CreateVolumeBackupPolicyAssignmentRequest struct {

	// Request to assign a specified policy to a particular asset.
	CreateVolumeBackupPolicyAssignmentDetails `contributesTo:"body"`

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request CreateVolumeBackupPolicyAssignmentRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request CreateVolumeBackupPolicyAssignmentRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request CreateVolumeBackupPolicyAssignmentRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// CreateVolumeBackupPolicyAssignmentResponse wrapper for the CreateVolumeBackupPolicyAssignment operation
type CreateVolumeBackupPolicyAssignmentResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The VolumeBackupPolicyAssignment instance
	VolumeBackupPolicyAssignment `presentIn:"body"`

	// For optimistic concurrency control. See `if-match`.
	Etag *string `presentIn:"header" name:"etag"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response CreateVolumeBackupPolicyAssignmentResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response CreateVolumeBackupPolicyAssignmentResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}
