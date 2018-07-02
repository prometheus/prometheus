// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListSecurityListsRequest wrapper for the ListSecurityLists operation
type ListSecurityListsRequest struct {

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" contributesTo:"query" name:"compartmentId"`

	// The OCID of the VCN.
	VcnId *string `mandatory:"true" contributesTo:"query" name:"vcnId"`

	// The maximum number of items to return in a paginated "List" call.
	// Example: `500`
	Limit *int `mandatory:"false" contributesTo:"query" name:"limit"`

	// The value of the `opc-next-page` response header from the previous "List" call.
	Page *string `mandatory:"false" contributesTo:"query" name:"page"`

	// A filter to return only resources that match the given display name exactly.
	DisplayName *string `mandatory:"false" contributesTo:"query" name:"displayName"`

	// The field to sort by. You can provide one sort order (`sortOrder`). Default order for
	// TIMECREATED is descending. Default order for DISPLAYNAME is ascending. The DISPLAYNAME
	// sort order is case sensitive.
	// **Note:** In general, some "List" operations (for example, `ListInstances`) let you
	// optionally filter by Availability Domain if the scope of the resource type is within a
	// single Availability Domain. If you call one of these "List" operations without specifying
	// an Availability Domain, the resources are grouped by Availability Domain, then sorted.
	SortBy ListSecurityListsSortByEnum `mandatory:"false" contributesTo:"query" name:"sortBy" omitEmpty:"true"`

	// The sort order to use, either ascending (`ASC`) or descending (`DESC`). The DISPLAYNAME sort order
	// is case sensitive.
	SortOrder ListSecurityListsSortOrderEnum `mandatory:"false" contributesTo:"query" name:"sortOrder" omitEmpty:"true"`

	// A filter to only return resources that match the given lifecycle state.  The state value is case-insensitive.
	LifecycleState SecurityListLifecycleStateEnum `mandatory:"false" contributesTo:"query" name:"lifecycleState" omitEmpty:"true"`

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request ListSecurityListsRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request ListSecurityListsRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request ListSecurityListsRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// ListSecurityListsResponse wrapper for the ListSecurityLists operation
type ListSecurityListsResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// A list of []SecurityList instances
	Items []SecurityList `presentIn:"body"`

	// For pagination of a list of items. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListSecurityListsResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response ListSecurityListsResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}

// ListSecurityListsSortByEnum Enum with underlying type: string
type ListSecurityListsSortByEnum string

// Set of constants representing the allowable values for ListSecurityListsSortBy
const (
	ListSecurityListsSortByTimecreated ListSecurityListsSortByEnum = "TIMECREATED"
	ListSecurityListsSortByDisplayname ListSecurityListsSortByEnum = "DISPLAYNAME"
)

var mappingListSecurityListsSortBy = map[string]ListSecurityListsSortByEnum{
	"TIMECREATED": ListSecurityListsSortByTimecreated,
	"DISPLAYNAME": ListSecurityListsSortByDisplayname,
}

// GetListSecurityListsSortByEnumValues Enumerates the set of values for ListSecurityListsSortBy
func GetListSecurityListsSortByEnumValues() []ListSecurityListsSortByEnum {
	values := make([]ListSecurityListsSortByEnum, 0)
	for _, v := range mappingListSecurityListsSortBy {
		values = append(values, v)
	}
	return values
}

// ListSecurityListsSortOrderEnum Enum with underlying type: string
type ListSecurityListsSortOrderEnum string

// Set of constants representing the allowable values for ListSecurityListsSortOrder
const (
	ListSecurityListsSortOrderAsc  ListSecurityListsSortOrderEnum = "ASC"
	ListSecurityListsSortOrderDesc ListSecurityListsSortOrderEnum = "DESC"
)

var mappingListSecurityListsSortOrder = map[string]ListSecurityListsSortOrderEnum{
	"ASC":  ListSecurityListsSortOrderAsc,
	"DESC": ListSecurityListsSortOrderDesc,
}

// GetListSecurityListsSortOrderEnumValues Enumerates the set of values for ListSecurityListsSortOrder
func GetListSecurityListsSortOrderEnumValues() []ListSecurityListsSortOrderEnum {
	values := make([]ListSecurityListsSortOrderEnum, 0)
	for _, v := range mappingListSecurityListsSortOrder {
		values = append(values, v)
	}
	return values
}
