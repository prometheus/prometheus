// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListConsoleHistoriesRequest wrapper for the ListConsoleHistories operation
type ListConsoleHistoriesRequest struct {

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" contributesTo:"query" name:"compartmentId"`

	// The name of the Availability Domain.
	// Example: `Uocm:PHX-AD-1`
	AvailabilityDomain *string `mandatory:"false" contributesTo:"query" name:"availabilityDomain"`

	// The maximum number of items to return in a paginated "List" call.
	// Example: `500`
	Limit *int `mandatory:"false" contributesTo:"query" name:"limit"`

	// The value of the `opc-next-page` response header from the previous "List" call.
	Page *string `mandatory:"false" contributesTo:"query" name:"page"`

	// The OCID of the instance.
	InstanceId *string `mandatory:"false" contributesTo:"query" name:"instanceId"`

	// The field to sort by. You can provide one sort order (`sortOrder`). Default order for
	// TIMECREATED is descending. Default order for DISPLAYNAME is ascending. The DISPLAYNAME
	// sort order is case sensitive.
	// **Note:** In general, some "List" operations (for example, `ListInstances`) let you
	// optionally filter by Availability Domain if the scope of the resource type is within a
	// single Availability Domain. If you call one of these "List" operations without specifying
	// an Availability Domain, the resources are grouped by Availability Domain, then sorted.
	SortBy ListConsoleHistoriesSortByEnum `mandatory:"false" contributesTo:"query" name:"sortBy" omitEmpty:"true"`

	// The sort order to use, either ascending (`ASC`) or descending (`DESC`). The DISPLAYNAME sort order
	// is case sensitive.
	SortOrder ListConsoleHistoriesSortOrderEnum `mandatory:"false" contributesTo:"query" name:"sortOrder" omitEmpty:"true"`

	// A filter to only return resources that match the given lifecycle state.  The state value is case-insensitive.
	LifecycleState ConsoleHistoryLifecycleStateEnum `mandatory:"false" contributesTo:"query" name:"lifecycleState" omitEmpty:"true"`

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request ListConsoleHistoriesRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request ListConsoleHistoriesRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request ListConsoleHistoriesRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// ListConsoleHistoriesResponse wrapper for the ListConsoleHistories operation
type ListConsoleHistoriesResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// A list of []ConsoleHistory instances
	Items []ConsoleHistory `presentIn:"body"`

	// For pagination of a list of items. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListConsoleHistoriesResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response ListConsoleHistoriesResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}

// ListConsoleHistoriesSortByEnum Enum with underlying type: string
type ListConsoleHistoriesSortByEnum string

// Set of constants representing the allowable values for ListConsoleHistoriesSortBy
const (
	ListConsoleHistoriesSortByTimecreated ListConsoleHistoriesSortByEnum = "TIMECREATED"
	ListConsoleHistoriesSortByDisplayname ListConsoleHistoriesSortByEnum = "DISPLAYNAME"
)

var mappingListConsoleHistoriesSortBy = map[string]ListConsoleHistoriesSortByEnum{
	"TIMECREATED": ListConsoleHistoriesSortByTimecreated,
	"DISPLAYNAME": ListConsoleHistoriesSortByDisplayname,
}

// GetListConsoleHistoriesSortByEnumValues Enumerates the set of values for ListConsoleHistoriesSortBy
func GetListConsoleHistoriesSortByEnumValues() []ListConsoleHistoriesSortByEnum {
	values := make([]ListConsoleHistoriesSortByEnum, 0)
	for _, v := range mappingListConsoleHistoriesSortBy {
		values = append(values, v)
	}
	return values
}

// ListConsoleHistoriesSortOrderEnum Enum with underlying type: string
type ListConsoleHistoriesSortOrderEnum string

// Set of constants representing the allowable values for ListConsoleHistoriesSortOrder
const (
	ListConsoleHistoriesSortOrderAsc  ListConsoleHistoriesSortOrderEnum = "ASC"
	ListConsoleHistoriesSortOrderDesc ListConsoleHistoriesSortOrderEnum = "DESC"
)

var mappingListConsoleHistoriesSortOrder = map[string]ListConsoleHistoriesSortOrderEnum{
	"ASC":  ListConsoleHistoriesSortOrderAsc,
	"DESC": ListConsoleHistoriesSortOrderDesc,
}

// GetListConsoleHistoriesSortOrderEnumValues Enumerates the set of values for ListConsoleHistoriesSortOrder
func GetListConsoleHistoriesSortOrderEnumValues() []ListConsoleHistoriesSortOrderEnum {
	values := make([]ListConsoleHistoriesSortOrderEnum, 0)
	for _, v := range mappingListConsoleHistoriesSortOrder {
		values = append(values, v)
	}
	return values
}
