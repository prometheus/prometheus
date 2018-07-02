// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// UpdateVolumeGroupBackupRequest wrapper for the UpdateVolumeGroupBackup operation
type UpdateVolumeGroupBackupRequest struct {

	// The Oracle Cloud ID (OCID) that uniquely identifies the volume group backup.
	VolumeGroupBackupId *string `mandatory:"true" contributesTo:"path" name:"volumeGroupBackupId"`

	// Update volume group backup fields
	UpdateVolumeGroupBackupDetails `contributesTo:"body"`

	// For optimistic concurrency control. In the PUT or DELETE call for a resource, set the `if-match`
	// parameter to the value of the etag from a previous GET or POST response for that resource.  The resource
	// will be updated or deleted only if the etag you provide matches the resource's current etag value.
	IfMatch *string `mandatory:"false" contributesTo:"header" name:"if-match"`

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request UpdateVolumeGroupBackupRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request UpdateVolumeGroupBackupRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request UpdateVolumeGroupBackupRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// UpdateVolumeGroupBackupResponse wrapper for the UpdateVolumeGroupBackup operation
type UpdateVolumeGroupBackupResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The VolumeGroupBackup instance
	VolumeGroupBackup `presentIn:"body"`

	// For optimistic concurrency control. See `if-match`.
	Etag *string `presentIn:"header" name:"etag"`
}

func (response UpdateVolumeGroupBackupResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response UpdateVolumeGroupBackupResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}
