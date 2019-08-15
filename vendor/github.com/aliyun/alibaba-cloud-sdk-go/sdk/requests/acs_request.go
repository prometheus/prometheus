/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package requests

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/errors"
)

const (
	RPC = "RPC"
	ROA = "ROA"

	HTTP  = "HTTP"
	HTTPS = "HTTPS"

	DefaultHttpPort = "80"

	GET     = "GET"
	PUT     = "PUT"
	POST    = "POST"
	DELETE  = "DELETE"
	HEAD    = "HEAD"
	OPTIONS = "OPTIONS"

	Json = "application/json"
	Xml  = "application/xml"
	Raw  = "application/octet-stream"
	Form = "application/x-www-form-urlencoded"

	Header = "Header"
	Query  = "Query"
	Body   = "Body"
	Path   = "Path"

	HeaderSeparator = "\n"
)

// interface
type AcsRequest interface {
	GetScheme() string
	GetMethod() string
	GetDomain() string
	GetPort() string
	GetRegionId() string
	GetHeaders() map[string]string
	GetQueryParams() map[string]string
	GetFormParams() map[string]string
	GetContent() []byte
	GetBodyReader() io.Reader
	GetStyle() string
	GetProduct() string
	GetVersion() string
	SetVersion(version string)
	GetActionName() string
	GetAcceptFormat() string
	GetLocationServiceCode() string
	GetLocationEndpointType() string
	GetReadTimeout() time.Duration
	GetConnectTimeout() time.Duration
	SetReadTimeout(readTimeout time.Duration)
	SetConnectTimeout(connectTimeout time.Duration)
	SetHTTPSInsecure(isInsecure bool)
	GetHTTPSInsecure() *bool

	GetUserAgent() map[string]string

	SetStringToSign(stringToSign string)
	GetStringToSign() string

	SetDomain(domain string)
	SetContent(content []byte)
	SetScheme(scheme string)
	BuildUrl() string
	BuildQueries() string

	addHeaderParam(key, value string)
	addQueryParam(key, value string)
	addFormParam(key, value string)
	addPathParam(key, value string)
}

// base class
type baseRequest struct {
	Scheme         string
	Method         string
	Domain         string
	Port           string
	RegionId       string
	ReadTimeout    time.Duration
	ConnectTimeout time.Duration
	isInsecure     *bool

	userAgent map[string]string
	product   string
	version   string

	actionName string

	AcceptFormat string

	QueryParams map[string]string
	Headers     map[string]string
	FormParams  map[string]string
	Content     []byte

	locationServiceCode  string
	locationEndpointType string

	queries string

	stringToSign string
}

func (request *baseRequest) GetQueryParams() map[string]string {
	return request.QueryParams
}

func (request *baseRequest) GetFormParams() map[string]string {
	return request.FormParams
}

func (request *baseRequest) GetReadTimeout() time.Duration {
	return request.ReadTimeout
}

func (request *baseRequest) GetConnectTimeout() time.Duration {
	return request.ConnectTimeout
}

func (request *baseRequest) SetReadTimeout(readTimeout time.Duration) {
	request.ReadTimeout = readTimeout
}

func (request *baseRequest) SetConnectTimeout(connectTimeout time.Duration) {
	request.ConnectTimeout = connectTimeout
}

func (request *baseRequest) GetHTTPSInsecure() *bool {
	return request.isInsecure
}

func (request *baseRequest) SetHTTPSInsecure(isInsecure bool) {
	request.isInsecure = &isInsecure
}

func (request *baseRequest) GetContent() []byte {
	return request.Content
}

func (request *baseRequest) SetVersion(version string) {
	request.version = version
}

func (request *baseRequest) GetVersion() string {
	return request.version
}

func (request *baseRequest) GetActionName() string {
	return request.actionName
}

func (request *baseRequest) SetContent(content []byte) {
	request.Content = content
}

func (request *baseRequest) GetUserAgent() map[string]string {
	return request.userAgent
}

func (request *baseRequest) AppendUserAgent(key, value string) {
	newkey := true
	if request.userAgent == nil {
		request.userAgent = make(map[string]string)
	}
	if strings.ToLower(key) != "core" && strings.ToLower(key) != "go" {
		for tag, _ := range request.userAgent {
			if tag == key {
				request.userAgent[tag] = value
				newkey = false
			}
		}
		if newkey {
			request.userAgent[key] = value
		}
	}
}

func (request *baseRequest) addHeaderParam(key, value string) {
	request.Headers[key] = value
}

func (request *baseRequest) addQueryParam(key, value string) {
	request.QueryParams[key] = value
}

func (request *baseRequest) addFormParam(key, value string) {
	request.FormParams[key] = value
}

func (request *baseRequest) GetAcceptFormat() string {
	return request.AcceptFormat
}

func (request *baseRequest) GetLocationServiceCode() string {
	return request.locationServiceCode
}

func (request *baseRequest) GetLocationEndpointType() string {
	return request.locationEndpointType
}

func (request *baseRequest) GetProduct() string {
	return request.product
}

func (request *baseRequest) GetScheme() string {
	return request.Scheme
}

func (request *baseRequest) SetScheme(scheme string) {
	request.Scheme = scheme
}

func (request *baseRequest) GetMethod() string {
	return request.Method
}

func (request *baseRequest) GetDomain() string {
	return request.Domain
}

func (request *baseRequest) SetDomain(host string) {
	request.Domain = host
}

func (request *baseRequest) GetPort() string {
	return request.Port
}

func (request *baseRequest) GetRegionId() string {
	return request.RegionId
}

func (request *baseRequest) GetHeaders() map[string]string {
	return request.Headers
}

func (request *baseRequest) SetContentType(contentType string) {
	request.addHeaderParam("Content-Type", contentType)
}

func (request *baseRequest) GetContentType() (contentType string, contains bool) {
	contentType, contains = request.Headers["Content-Type"]
	return
}

func (request *baseRequest) SetStringToSign(stringToSign string) {
	request.stringToSign = stringToSign
}

func (request *baseRequest) GetStringToSign() string {
	return request.stringToSign
}

func defaultBaseRequest() (request *baseRequest) {
	request = &baseRequest{
		Scheme:       "",
		AcceptFormat: "JSON",
		Method:       GET,
		QueryParams:  make(map[string]string),
		Headers: map[string]string{
			"x-sdk-client":      "golang/1.0.0",
			"x-sdk-invoke-type": "normal",
			"Accept-Encoding":   "identity",
		},
		FormParams: make(map[string]string),
	}
	return
}

func InitParams(request AcsRequest) (err error) {
	requestValue := reflect.ValueOf(request).Elem()
	err = flatRepeatedList(requestValue, request, "", "")
	return
}

func flatRepeatedList(dataValue reflect.Value, request AcsRequest, position, prefix string) (err error) {
	dataType := dataValue.Type()
	for i := 0; i < dataType.NumField(); i++ {
		field := dataType.Field(i)
		name, containsNameTag := field.Tag.Lookup("name")
		fieldPosition := position
		if fieldPosition == "" {
			fieldPosition, _ = field.Tag.Lookup("position")
		}
		typeTag, containsTypeTag := field.Tag.Lookup("type")
		if containsNameTag {
			if !containsTypeTag {
				// simple param
				key := prefix + name
				value := dataValue.Field(i).String()
				if dataValue.Field(i).Kind().String() == "map" {
					byt, _ := json.Marshal(dataValue.Field(i).Interface())
					value = string(byt)
				}
				err = addParam(request, fieldPosition, key, value)
				if err != nil {
					return
				}
			} else if typeTag == "Repeated" {
				// repeated param
				repeatedFieldValue := dataValue.Field(i)
				if repeatedFieldValue.Kind() != reflect.Slice {
					// possible value: {"[]string", "*[]struct"}, we must call Elem() in the last condition
					repeatedFieldValue = repeatedFieldValue.Elem()
				}
				if repeatedFieldValue.IsValid() && !repeatedFieldValue.IsNil() {
					for m := 0; m < repeatedFieldValue.Len(); m++ {
						elementValue := repeatedFieldValue.Index(m)
						key := prefix + name + "." + strconv.Itoa(m+1)
						if elementValue.Type().Kind().String() == "string" {
							value := elementValue.String()
							err = addParam(request, fieldPosition, key, value)
							if err != nil {
								return
							}
						} else {
							err = flatRepeatedList(elementValue, request, fieldPosition, key+".")
							if err != nil {
								return
							}
						}
					}
				}
			} else if typeTag == "Struct" {
				valueField := dataValue.Field(i)
				if valueField.Kind() == reflect.Struct {
					if valueField.IsValid() && valueField.String() != "" {
						valueFieldType := valueField.Type()
						for m := 0; m < valueFieldType.NumField(); m++ {
							fieldName := valueFieldType.Field(m).Name
							elementValue := valueField.FieldByName(fieldName)
							key := prefix + name + "." + fieldName
							if elementValue.Type().String() == "[]string" {
								for j := 0; j < elementValue.Len(); j++ {
									err = addParam(request, fieldPosition, key+"."+strconv.Itoa(j+1), elementValue.Index(j).String())
									if err != nil {
										return
									}
								}
							} else {
								// 	value := elementValue.String()
								// 	err = addParam(request, fieldPosition, key, value)
								// 	if err != nil {
								// 		return
								// 	}
								// } else {
								// err = flatRepeatedList(elementValue, request, fieldPosition, key+".")
								// if err != nil {
								// 	return
								// }
								// }
							}
						}
					}
				}
			}
		}
	}
	return
}

func addParam(request AcsRequest, position, name, value string) (err error) {
	if len(value) > 0 {
		switch position {
		case Header:
			request.addHeaderParam(name, value)
		case Query:
			request.addQueryParam(name, value)
		case Path:
			request.addPathParam(name, value)
		case Body:
			request.addFormParam(name, value)
		default:
			errMsg := fmt.Sprintf(errors.UnsupportedParamPositionErrorMessage, position)
			err = errors.NewClientError(errors.UnsupportedParamPositionErrorCode, errMsg, nil)
		}
	}
	return
}
