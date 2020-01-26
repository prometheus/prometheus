# \ResponseBodyServiceApi

All URIs are relative to *http://localhost*

Method | HTTP request | Description
------------- | ------------- | -------------
[**GetResponseBody**](ResponseBodyServiceApi.md#GetResponseBody) | **Get** /responsebody/{data} | 
[**ListResponseBodies**](ResponseBodyServiceApi.md#ListResponseBodies) | **Get** /responsebodies/{data} | 
[**ListResponseStrings**](ResponseBodyServiceApi.md#ListResponseStrings) | **Get** /responsestrings/{data} | 


# **GetResponseBody**
> ExamplepbResponseBodyOutResponse GetResponseBody($data)




### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **data** | **string**|  | 

### Return type

[**ExamplepbResponseBodyOutResponse**](examplepbResponseBodyOutResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ListResponseBodies**
> []ExamplepbRepeatedResponseBodyOutResponse ListResponseBodies($data)




### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **data** | **string**|  | 

### Return type

[**[]ExamplepbRepeatedResponseBodyOutResponse**](examplepbRepeatedResponseBodyOutResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **ListResponseStrings**
> []string ListResponseStrings($data)




### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **data** | **string**|  | 

### Return type

**[]string**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

