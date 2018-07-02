// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

package core

import (
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

// ListVolumeGroupBackupsRequest wrapper for the ListVolumeGroupBackups operation
type ListVolumeGroupBackupsRequest struct {

	// The OCID of the compartment.
	CompartmentId *string `mandatory:"true" contributesTo:"query" name:"compartmentId"`

	// The OCID of the volume group.
	VolumeGroupId *string `mandatory:"false" contributesTo:"query" name:"volumeGroupId"`

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
	SortBy ListVolumeGroupBackupsSortByEnum `mandatory:"false" contributesTo:"query" name:"sortBy" omitEmpty:"true"`

	// The sort order to use, either ascending (`ASC`) or descending (`DESC`). The DISPLAYNAME sort order
	// is case sensitive.
	SortOrder ListVolumeGroupBackupsSortOrderEnum `mandatory:"false" contributesTo:"query" name:"sortOrder" omitEmpty:"true"`

	// Unique Oracle-assigned identifier for the request.
	// If you need to contact Oracle about a particular request, please provide the request ID.
	OpcRequestId *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`

	// Metadata about the request. This information will not be transmitted to the service, but
	// represents information that the SDK will consume to drive retry behavior.
	RequestMetadata common.RequestMetadata
}

func (request ListVolumeGroupBackupsRequest) String() string {
	return common.PointerString(request)
}

// HTTPRequest implements the OCIRequest interface
func (request ListVolumeGroupBackupsRequest) HTTPRequest(method, path string) (http.Request, error) {
	return common.MakeDefaultHTTPRequestWithTaggedStruct(method, path, request)
}

// RetryPolicy implements the OCIRetryableRequest interface. This retrieves the specified retry policy.
func (request ListVolumeGroupBackupsRequest) RetryPolicy() *common.RetryPolicy {
	return request.RequestMetadata.RetryPolicy
}

// ListVolumeGroupBackupsResponse wrapper for the ListVolumeGroupBackups operation
type ListVolumeGroupBackupsResponse struct {

	// The underlying http response
	RawResponse *http.Response

	// A list of []VolumeGroupBackup instances
	Items []VolumeGroupBackup `presentIn:"body"`

	// For pagination of a list of items. When paging through a list, if this header appears in the response,
	// then a partial list might have been returned. Include this value as the `page` parameter for the
	// subsequent GET request to get the next batch of items.
	OpcNextPage *string `presentIn:"header" name:"opc-next-page"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about
	// a particular request, please provide the request ID.
	OpcRequestId *string `presentIn:"header" name:"opc-request-id"`
}

func (response ListVolumeGroupBackupsResponse) String() string {
	return common.PointerString(response)
}

// HTTPResponse implements the OCIResponse interface
func (response ListVolumeGroupBackupsResponse) HTTPResponse() *http.Response {
	return response.RawResponse
}

// ListVolumeGroupBackupsSortByEnum Enum with underlying type: string
type ListVolumeGroupBackupsSortByEnum string

// Set of constants representing the allowable values for ListVolumeGroupBackupsSortBy
const (
	ListVolumeGroupBackupsSortByTimecreated ListVolumeGroupBackupsSortByEnum = "TIMECREATED"
	ListVolumeGroupBackupsSortByDisplayname ListVolumeGroupBackupsSortByEnum = "DISPLAYNAME"
)

var mappingListVolumeGroupBackupsSortBy = map[string]ListVolumeGroupBackupsSortByEnum{
	"TIMECREATED": ListVolumeGroupBackupsSortByTimecreated,
	"DISPLAYNAME": ListVolumeGroupBackupsSortByDisplayname,
}

// GetListVolumeGroupBackupsSortByEnumValues Enumerates the set of values for ListVolumeGroupBackupsSortBy
func GetListVolumeGroupBackupsSortByEnumValues() []ListVolumeGroupBackupsSortByEnum {
	values := make([]ListVolumeGroupBackupsSortByEnum, 0)
	for _, v := range mappingListVolumeGroupBackupsSortBy {
		values = append(values, v)
	}
	return values
}

// ListVolumeGroupBackupsSortOrderEnum Enum with underlying type: string
type ListVolumeGroupBackupsSortOrderEnum string

// Set of constants representing the allowable values for ListVolumeGroupBackupsSortOrder
const (
	ListVolumeGroupBackupsSortOrderAsc  ListVolumeGroupBackupsSortOrderEnum = "ASC"
	ListVolumeGroupBackupsSortOrderDesc ListVolumeGroupBackupsSortOrderEnum = "DESC"
)

var mappingListVolumeGroupBackupsSortOrder = map[string]ListVolumeGroupBackupsSortOrderEnum{
	"ASC":  ListVolumeGroupBackupsSortOrderAsc,
	"DESC": ListVolumeGroupBackupsSortOrderDesc,
}

// GetListVolumeGroupBackupsSortOrderEnumValues Enumerates the set of values for ListVolumeGroupBackupsSortOrder
func GetListVolumeGroupBackupsSortOrderEnumValues() []ListVolumeGroupBackupsSortOrderEnum {
	values := make([]ListVolumeGroupBackupsSortOrderEnum, 0)
	for _, v := range mappingListVolumeGroupBackupsSortOrder {
		values = append(values, v)
	}
	return values
}
