// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListImagesRequest wrapper for the ListImages operation
type ListImagesRequest struct {

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" contributesTo:"query" name:"compartmentId"`

	// A filter to return only resources that match the given display name exactly.
	DisplayName *string `mandatory:"false" contributesTo:"query" name:"displayName"`

	// The image's operating system.
	// Example: `Oracle Linux`
	OperatingSystem *string `mandatory:"false" contributesTo:"query" name:"operatingSystem"`

	// The image's operating system version.
	// Example: `7.2`
	OperatingSystemVersion *string `mandatory:"false" contributesTo:"query" name:"operatingSystemVersion"`

	// Shape name.
	Shape *string `mandatory:"false" contributesTo:"query" name:"shape"`

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
	SortBy ListImagesSortByEnum `mandatory:"false" contributesTo:"query" name:"sortBy" omitEmpty:"true"`

	// The sort order to use, either ascending (`ASC`) or descending (`DESC`). The DISPLAYNAME sort order
	// is case sensitive.
	SortOrder ListImagesSortOrderEnum `mandatory:"false" contributesTo:"query" name:"sortOrder" omitEmpty:"true"`

	// A filter to only return resources that match the given lifecycle state.  The state value is case-insensitive.
	LifecycleState ImageLifecycleStateEnum `mandatory:"false" contributesTo:"query" name:"lifecycleState" omitEmpty:"true"`

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request ListImagesRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request ListImagesRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request ListImagesRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// ListImagesResponse wrapper for the ListImages operation
type ListImagesResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// A list of []Image instances
	Items []Image `presentIn:"body"`

	// For pagination of a list of items. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListImagesResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response ListImagesResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}

// ListImagesSortByEnum Enum with underlying type: string
type ListImagesSortByEnum string

// Set of constants representing the allowable values for ListImagesSortBy
const (
	ListImagesSortByTimecreated ListImagesSortByEnum = "TIMECREATED"
	ListImagesSortByDisplayname ListImagesSortByEnum = "DISPLAYNAME"
)

var mappingListImagesSortBy = map[string]ListImagesSortByEnum{
	"TIMECREATED": ListImagesSortByTimecreated,
	"DISPLAYNAME": ListImagesSortByDisplayname,
}

// GetListImagesSortByEnumValues Enumerates the set of values for ListImagesSortBy
func GetListImagesSortByEnumValues() []ListImagesSortByEnum {
	values := make([]ListImagesSortByEnum, 0)
	for _, v := range mappingListImagesSortBy {
		values = append(values, v)
	}
	return values
}

// ListImagesSortOrderEnum Enum with underlying type: string
type ListImagesSortOrderEnum string

// Set of constants representing the allowable values for ListImagesSortOrder
const (
	ListImagesSortOrderAsc  ListImagesSortOrderEnum = "ASC"
	ListImagesSortOrderDesc ListImagesSortOrderEnum = "DESC"
)

var mappingListImagesSortOrder = map[string]ListImagesSortOrderEnum{
	"ASC":  ListImagesSortOrderAsc,
	"DESC": ListImagesSortOrderDesc,
}

// GetListImagesSortOrderEnumValues Enumerates the set of values for ListImagesSortOrder
func GetListImagesSortOrderEnumValues() []ListImagesSortOrderEnum {
	values := make([]ListImagesSortOrderEnum, 0)
	for _, v := range mappingListImagesSortOrder {
		values = append(values, v)
	}
	return values
}
