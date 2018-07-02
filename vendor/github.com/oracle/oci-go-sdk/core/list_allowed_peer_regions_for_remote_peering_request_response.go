// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListAllowedPeerRegionsForRemotePeeringRequest wrapper for the ListAllowedPeerRegionsForRemotePeering operation
type ListAllowedPeerRegionsForRemotePeeringRequest struct {

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request ListAllowedPeerRegionsForRemotePeeringRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request ListAllowedPeerRegionsForRemotePeeringRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request ListAllowedPeerRegionsForRemotePeeringRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// ListAllowedPeerRegionsForRemotePeeringResponse wrapper for the ListAllowedPeerRegionsForRemotePeering operation
type ListAllowedPeerRegionsForRemotePeeringResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// The []PeerRegionForRemotePeering instance
	Items []PeerRegionForRemotePeering `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListAllowedPeerRegionsForRemotePeeringResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response ListAllowedPeerRegionsForRemotePeeringResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}
