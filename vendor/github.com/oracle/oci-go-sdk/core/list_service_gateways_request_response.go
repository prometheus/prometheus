// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListServiceGatewaysRequest wrapper for the ListServiceGateways operation
type ListServiceGatewaysRequest struct {

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" contributesTo:"query" name:"compartmentId"`

	// The OCID of the VCN.
	VcnId *string `mandatory:"false" contributesTo:"query" name:"vcnId"`

	// The maximum number of items to return in a paginated "List" call.
	// Example: `500`
	Limit *int `mandatory:"false" contributesTo:"query" name:"limit"`

	// The value of the `opc-next-page` response header from the previous "List" call.
	Page *string `mandatory:"false" contributesTo:"query" name:"page"`

	// The field to sort by. You can provide one sort order (`sortOrder`). Default order for
	// TIMECREATED is descending. Default order for DISPLAYNAME is ascending. The DISPLAYNAME
	// sort order is case sensitive.
	// **Note:** In general, some "List" operations (for example, `ListInstances`) let you
	// optionally filter by Availability Domain if the scope of the resource type is within a
	// single Availability Domain. If you call one of these "List" operations without specifying
	// an Availability Domain, the resources are grouped by Availability Domain, then sorted.
	SortBy ListServiceGatewaysSortByEnum `mandatory:"false" contributesTo:"query" name:"sortBy" omitEmpty:"true"`

	// The sort order to use, either ascending (`ASC`) or descending (`DESC`). The DISPLAYNAME sort order
	// is case sensitive.
	SortOrder ListServiceGatewaysSortOrderEnum `mandatory:"false" contributesTo:"query" name:"sortOrder" omitEmpty:"true"`

	// A filter to return only resources that match the given lifecycle state.  The state value is case-insensitive.
	LifecycleState ServiceGatewayLifecycleStateEnum `mandatory:"false" contributesTo:"query" name:"lifecycleState" omitEmpty:"true"`

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request ListServiceGatewaysRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request ListServiceGatewaysRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request ListServiceGatewaysRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// ListServiceGatewaysResponse wrapper for the ListServiceGateways operation
type ListServiceGatewaysResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// A list of []ServiceGateway instances
	Items []ServiceGateway `presentIn:"body"`

	// For pagination of a list of items. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListServiceGatewaysResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response ListServiceGatewaysResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}

// ListServiceGatewaysSortByEnum Enum with underlying type: string
type ListServiceGatewaysSortByEnum string

// Set of constants representing the allowable values for ListServiceGatewaysSortBy
const (
	ListServiceGatewaysSortByTimecreated ListServiceGatewaysSortByEnum = "TIMECREATED"
	ListServiceGatewaysSortByDisplayname ListServiceGatewaysSortByEnum = "DISPLAYNAME"
)

var mappingListServiceGatewaysSortBy = map[string]ListServiceGatewaysSortByEnum{
	"TIMECREATED": ListServiceGatewaysSortByTimecreated,
	"DISPLAYNAME": ListServiceGatewaysSortByDisplayname,
}

// GetListServiceGatewaysSortByEnumValues Enumerates the set of values for ListServiceGatewaysSortBy
func GetListServiceGatewaysSortByEnumValues() []ListServiceGatewaysSortByEnum {
	values := make([]ListServiceGatewaysSortByEnum, 0)
	for _, v := range mappingListServiceGatewaysSortBy {
		values = append(values, v)
	}
	return values
}

// ListServiceGatewaysSortOrderEnum Enum with underlying type: string
type ListServiceGatewaysSortOrderEnum string

// Set of constants representing the allowable values for ListServiceGatewaysSortOrder
const (
	ListServiceGatewaysSortOrderAsc  ListServiceGatewaysSortOrderEnum = "ASC"
	ListServiceGatewaysSortOrderDesc ListServiceGatewaysSortOrderEnum = "DESC"
)

var mappingListServiceGatewaysSortOrder = map[string]ListServiceGatewaysSortOrderEnum{
	"ASC":  ListServiceGatewaysSortOrderAsc,
	"DESC": ListServiceGatewaysSortOrderDesc,
}

// GetListServiceGatewaysSortOrderEnumValues Enumerates the set of values for ListServiceGatewaysSortOrder
func GetListServiceGatewaysSortOrderEnumValues() []ListServiceGatewaysSortOrderEnum {
	values := make([]ListServiceGatewaysSortOrderEnum, 0)
	for _, v := range mappingListServiceGatewaysSortOrder {
		values = append(values, v)
	}
	return values
}
