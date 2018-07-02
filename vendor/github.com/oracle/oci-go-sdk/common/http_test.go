// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.

package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//Test data structures, avoid import cycle
type TestupdateUserDetails struct {
	Description string  `mandatory:"false" json:"description,omitempty"`
	Name        *string `mandatory:"false" json:"name"`
	SomeNumbers []int   `mandatory:"false" json:"numbers"`
}

type listCompartmentsRequest struct {
	CompartmentID string   `mandatory:"true" contributesTo:"query" name:"compartmentId"`
	Page          string   `mandatory:"false" contributesTo:"query" name:"page"`
	Limit         int32    `mandatory:"false" contributesTo:"query" name:"limit"`
	Fields        []string `mandatory:"true" contributesTo:"query" name:"fields" collectionFormat:"csv"`
}

type updateUserRequest struct {
	UserID                string `mandatory:"true" contributesTo:"path" name:"userId"`
	TestupdateUserDetails `contributesTo:"body"`
	IfMatch               string `mandatory:"false" contributesTo:"header" name:"if-match"`
}

type TestcreateAPIKeyDetails struct {
	Key string `mandatory:"true" json:"key"`
}

type TestcreateAPIKeyDetailsPtr struct {
	Key     *string  `mandatory:"true" json:"key"`
	TheTime *SDKTime `mandatory:"true" json:"theTime"`
}

type uploadAPIKeyRequest struct {
	UserID                  string `mandatory:"true" contributesTo:"path" name:"userId"`
	TestcreateAPIKeyDetails `contributesTo:"body"`
	OpcRetryToken           string `mandatory:"false" contributesTo:"header" name:"opc-retry-token"`
}

type uploadAPIKeyRequestPtr struct {
	UserID                     *string `mandatory:"true" contributesTo:"path" name:"userId"`
	TestcreateAPIKeyDetailsPtr `contributesTo:"body"`
	OpcRetryToken              *string `mandatory:"false" contributesTo:"header" name:"opc-retry-token"`
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func TestHttpMarshallerInvalidStruct(t *testing.T) {
	request := http.Request{}
	err := HTTPRequestMarshaller("asdf", &request)
	assert.Error(t, err, nil)
}

func TestHttpRequestMarshallerQuery(t *testing.T) {
	s := listCompartmentsRequest{CompartmentID: "ocid1", Page: "p", Limit: 23, Fields: []string{"one", "two", "three"}}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/")
	e := HTTPRequestMarshaller(s, &request)
	query := request.URL.Query()
	assert.NoError(t, e)
	assert.True(t, query.Get("compartmentId") == "ocid1")
	assert.True(t, query.Get("page") == "p")
	assert.True(t, query.Get("limit") == "23")
	assert.True(t, query.Get("fields") == "one,two,three")
}

func TestMakeDefault(t *testing.T) {
	r := MakeDefaultHTTPRequest(http.MethodPost, "/one/two")
	assert.NotEmpty(t, r.Header.Get(requestHeaderDate))
	assert.NotEmpty(t, r.Header.Get(requestHeaderOpcClientInfo))
}

func TestHttpMarshallerSimpleHeader(t *testing.T) {
	s := updateUserRequest{UserID: "id1", IfMatch: "n=as", TestupdateUserDetails: TestupdateUserDetails{Description: "name of"}}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	HTTPRequestMarshaller(s, &request)
	header := request.Header
	assert.True(t, header.Get(requestHeaderIfMatch) == "n=as")
}

func TestHttpMarshallerSimpleStruct(t *testing.T) {
	s := uploadAPIKeyRequest{UserID: "111", OpcRetryToken: "token", TestcreateAPIKeyDetails: TestcreateAPIKeyDetails{Key: "thekey"}}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	HTTPRequestMarshaller(s, &request)
	assert.True(t, strings.Contains(request.URL.Path, "111"))
}

func TestHttpMarshallerSimpleBody(t *testing.T) {
	desc := "theDescription"
	s := updateUserRequest{UserID: "id1", IfMatch: "n=as", TestupdateUserDetails: TestupdateUserDetails{
		Description: desc, SomeNumbers: []int{}}}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	HTTPRequestMarshaller(s, &request)
	body, _ := ioutil.ReadAll(request.Body)
	var content map[string]string
	json.Unmarshal(body, &content)
	assert.Contains(t, content, "description")
	assert.Contains(t, content, "numbers")
	assert.NotContains(t, content, "name")
	assert.Equal(t, "", content["numbers"])

	if val, ok := content["description"]; !ok || val != desc {
		assert.Fail(t, "Should contain: "+desc)
	}
}

func TestHttpMarshallerEmptyPath(t *testing.T) {
	type testData struct {
		userID      string
		httpMethod  string
		expectError bool
	}

	testDataSet := []testData{
		{
			userID:      "id1",
			httpMethod:  http.MethodGet,
			expectError: false,
		},
		{
			userID:      "",
			httpMethod:  http.MethodGet,
			expectError: true,
		},
		{
			userID:      "",
			httpMethod:  http.MethodPut,
			expectError: true,
		},
		{
			userID:      "",
			httpMethod:  http.MethodHead,
			expectError: true,
		},
		{
			userID:      "",
			httpMethod:  http.MethodDelete,
			expectError: true,
		},
		{
			userID:      "",
			httpMethod:  http.MethodPost,
			expectError: true,
		},
	}

	for _, testData := range testDataSet {
		// user id contributes to path
		s := updateUserRequest{UserID: testData.userID}
		request := MakeDefaultHTTPRequest(testData.httpMethod, "/")
		err := HTTPRequestMarshaller(s, &request)
		assert.Equal(t, testData.expectError, err != nil)
	}
}

func TestHttpMarshalerAll(t *testing.T) {
	desc := "theDescription"
	type inc string
	includes := []inc{inc("One"), inc("Two")}

	s := struct {
		ID           string                `contributesTo:"path"`
		Name         string                `contributesTo:"query" name:"name"`
		When         *SDKTime              `contributesTo:"query" name:"when"`
		Income       float32               `contributesTo:"query" name:"income"`
		Include      []inc                 `contributesTo:"query" name:"includes" collectionFormat:"csv"`
		IncludeMulti []inc                 `contributesTo:"query" name:"includesMulti" collectionFormat:"multi"`
		Male         bool                  `contributesTo:"header" name:"male"`
		Details      TestupdateUserDetails `contributesTo:"body"`
	}{
		"101", "tapir", now(), 3.23, includes, includes, true, TestupdateUserDetails{Description: desc},
	}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/")
	e := HTTPRequestMarshaller(s, &request)
	assert.NoError(t, e)
	var content map[string]string
	body, _ := ioutil.ReadAll(request.Body)
	json.Unmarshal(body, &content)
	when := s.When.Format(time.RFC3339)
	assert.True(t, request.URL.Path == "/101")
	assert.True(t, request.URL.Query().Get("name") == s.Name)
	assert.True(t, request.URL.Query().Get("income") == strconv.FormatFloat(float64(s.Income), 'f', 6, 32))
	assert.True(t, request.URL.Query().Get("when") == when)
	assert.True(t, request.URL.Query().Get("includes") == "One,Two")
	assert.True(t, reflect.DeepEqual(request.URL.Query()["includesMulti"], []string{"One", "Two"}))
	assert.Contains(t, content, "description")
	assert.Equal(t, request.Header.Get(requestHeaderContentType), "application/json")
	if val, ok := content["description"]; !ok || val != desc {
		assert.Fail(t, "Should contain: "+desc)
	}
}

func TestHttpMarshalerPointers(t *testing.T) {

	n := new(string)
	*n = "theName"
	s := struct {
		Name *string `contributesTo:"query" name:"name"`
	}{
		n,
	}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	HTTPRequestMarshaller(s, &request)
	assert.NotNil(t, request)
	assert.True(t, request.URL.Query().Get("name") == *s.Name)
}

func TestHttpMarshalerPointersErrorHeader(t *testing.T) {

	n := new(string)
	*n = "theName"
	s := struct {
		Name *string `mandatory:"true" contributesTo:"header" name:"name"`
	}{
		nil,
	}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	e := HTTPRequestMarshaller(s, &request)
	assert.Error(t, e)
}

func TestHttpMarshalerPointersErrorPath(t *testing.T) {

	n := new(string)
	*n = "theName"
	s := struct {
		Name *string `mandatory:"true" contributesTo:"path" name:"name"`
	}{
		nil,
	}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	e := HTTPRequestMarshaller(s, &request)
	assert.Error(t, e)
}

func TestHttpMarshallerSimpleStructPointers(t *testing.T) {
	now := SDKTime{time.Now()}
	s := uploadAPIKeyRequestPtr{
		UserID:        String("111"),
		OpcRetryToken: nil,
		TestcreateAPIKeyDetailsPtr: TestcreateAPIKeyDetailsPtr{
			Key:     String("thekey"),
			TheTime: &now,
		}}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	HTTPRequestMarshaller(s, &request)
	all, _ := ioutil.ReadAll(request.Body)
	assert.True(t, len(all) > 2)
	assert.Equal(t, "", request.Header.Get(requestHeaderOpcRetryToken))
	assert.True(t, strings.Contains(request.URL.Path, "111"))
	assert.True(t, strings.Contains(string(all), "thekey"))
	assert.Contains(t, string(all), now.Format(time.RFC3339))
}

func TestHttpMarshallerSimpleStructPointersFilled(t *testing.T) {
	s := uploadAPIKeyRequestPtr{
		UserID:                     String("111"),
		OpcRetryToken:              String("token"),
		TestcreateAPIKeyDetailsPtr: TestcreateAPIKeyDetailsPtr{Key: String("thekey")}}
	request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
	HTTPRequestMarshaller(s, &request)
	assert.Equal(t, "token", request.Header.Get(requestHeaderOpcRetryToken))
	assert.True(t, strings.Contains(request.URL.Path, "111"))

}

func TestHttpMarshalerUntaggedFields(t *testing.T) {
	s := struct {
		Name  string `contributesTo:"query" name:"name"`
		AList []string
		AMap  map[string]int
		TestupdateUserDetails
	}{
		"theName", []string{"a", "b"}, map[string]int{"a": 1, "b": 2},
		TestupdateUserDetails{Description: "n"},
	}
	request := &http.Request{}
	e := HTTPRequestMarshaller(s, request)
	assert.NoError(t, e)
	assert.NotNil(t, request)
	assert.True(t, request.URL.Query().Get("name") == s.Name)
}
func TestHttpMarshalerPathTemplate(t *testing.T) {
	urlTemplate := "/name/{userId}/aaa"
	s := uploadAPIKeyRequest{UserID: "111", OpcRetryToken: "token", TestcreateAPIKeyDetails: TestcreateAPIKeyDetails{Key: "thekey"}}
	request := MakeDefaultHTTPRequest(http.MethodPost, urlTemplate)
	e := HTTPRequestMarshaller(s, &request)
	assert.NoError(t, e)
	assert.Equal(t, "/name/111/aaa", request.URL.Path)
}

func TestHttpMarshalerFunnyTags(t *testing.T) {
	s := struct {
		Name  string `contributesTo:"quer" name:"name"`
		AList []string
		AMap  map[string]int
		TestupdateUserDetails
	}{
		"theName", []string{"a", "b"}, map[string]int{"a": 1, "b": 2},
		TestupdateUserDetails{Description: "n"},
	}
	request := &http.Request{}
	e := HTTPRequestMarshaller(s, request)
	assert.Error(t, e)
}

func TestHttpMarshalerUnsupportedTypes(t *testing.T) {
	s1 := struct {
		Name string         `contributesTo:"query" name:"name"`
		AMap map[string]int `contributesTo:"query" name:"theMap"`
	}{
		"theName", map[string]int{"a": 1, "b": 2},
	}
	s2 := struct {
		Name  string   `contributesTo:"query" name:"name"`
		AList []string `contributesTo:"query" name:"theList"`
	}{
		"theName", []string{"a", "b"},
	}
	s3 := struct {
		Name                  string `contributesTo:"query" name:"name"`
		TestupdateUserDetails `contributesTo:"query" name:"str"`
	}{
		"theName", TestupdateUserDetails{Description: "a"},
	}
	n := new(string)
	col := make([]int, 10)
	*n = "theName"
	s4 := struct {
		Name *string `contributesTo:"query" name:"name"`
		Coll *[]int  `contributesTo:"query" name:"coll"`
	}{
		n, &col,
	}

	lst := []interface{}{s1, s2, s3, s4}
	for _, l := range lst {
		request := &http.Request{}
		e := HTTPRequestMarshaller(l, request)
		Debugln(e)
		assert.Error(t, e)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//Response Unmarshaling
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// ListRegionsResponse wrapper for the ListRegions operation
type listRegionsResponse struct {

	// The []Region instance
	Items []int `presentIn:"body"`

	// Unique Oracle-assigned identifier for the request. If you need to contact Oracle about a
	// particular request, please provide the request ID.
	OpcRequestID string `presentIn:"header" name:"opcrequestid"`
}

type listUsersResponse struct {
	Items        []int   `presentIn:"body"`
	OpcRequestID string  `presentIn:"header" name:"opcrequestid"`
	OpcNextPage  int     `presentIn:"header" name:"opcnextpage"`
	SomeUint     uint    `presentIn:"header" name:"someuint"`
	SomeBool     bool    `presentIn:"header" name:"somebool"`
	SomeTime     SDKTime `presentIn:"header" name:"sometime"`
	SomeFloat    float64 `presentIn:"header" name:"somefloat"`
}

func TestUnmarshalResponse_StringHeader(t *testing.T) {
	header := http.Header{}
	opcID := "111"
	header.Set("OpcrequestId", opcID)
	r := http.Response{Header: header}
	s := listRegionsResponse{}
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, s.OpcRequestID, opcID)

}

func TestUnmarshalResponse_MixHeader(t *testing.T) {
	header := http.Header{}
	opcID := "111"
	nextPage := int(333)
	someuint := uint(12)
	somebool := true
	sometime := now()
	somefloat := 2.556

	header.Set("OpcrequestId", opcID)
	header.Set("opcnextpage", strconv.FormatInt(int64(nextPage), 10))
	header.Set("someuint", strconv.FormatUint(uint64(someuint), 10))
	header.Set("somebool", strconv.FormatBool(somebool))
	header.Set("sometime", formatTime(*sometime))
	header.Set("somefloat", strconv.FormatFloat(somefloat, 'f', 3, 64))

	r := http.Response{Header: header}
	s := listUsersResponse{}
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, s.OpcRequestID, opcID)
	assert.Equal(t, nextPage, s.OpcNextPage)
	assert.Equal(t, someuint, s.SomeUint)
	assert.Equal(t, somebool, s.SomeBool)
	assert.Equal(t, sometime.Format(time.RFC3339), s.SomeTime.Format(time.RFC3339))

}

type rgn struct {
	Key  string `mandatory:"false" json:"key,omitempty"`
	Name string `mandatory:"false" json:"name,omitempty"`
}

func TestUnmarshalResponse_SimpleBody(t *testing.T) {
	sampleResponse := `{"key" : "RegionFRA","name" : "eu-frankfurt-1"}`
	header := http.Header{}
	opcID := "111"
	header.Set("OpcrequestId", opcID)
	s := struct {
		Rg rgn `presentIn:"body"`
	}{}
	r := http.Response{Header: header}
	bodyBuffer := bytes.NewBufferString(sampleResponse)
	r.Body = ioutil.NopCloser(bodyBuffer)
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, "eu-frankfurt-1", s.Rg.Name)
}

func TestUnmarshalResponse_SimpleBodyList(t *testing.T) {
	sampleResponse := `[{"key" : "RegionFRA","name" : "eu-frankfurt-1"},{"key" : "RegionIAD","name" : "us-ashburn-1"}]`
	header := http.Header{}
	opcID := "111"
	header.Set("OpcrequestId", opcID)
	s := struct {
		Items []rgn `presentIn:"body"`
	}{}
	r := http.Response{Header: header}
	bodyBuffer := bytes.NewBufferString(sampleResponse)
	r.Body = ioutil.NopCloser(bodyBuffer)
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.NotEmpty(t, s.Items)
	assert.Equal(t, "eu-frankfurt-1", s.Items[0].Name)
	assert.Equal(t, "RegionIAD", s.Items[1].Key)
}

func TestUnmarshalResponse_SimpleBodyPtr(t *testing.T) {
	sampleResponse := `{"key" : "RegionFRA","name" : "eu-frankfurt-1"}`
	header := http.Header{}
	opcID := "111"
	header.Set("OpcrequestId", opcID)
	s := struct {
		Rg *rgn `presentIn:"body"`
	}{}
	r := http.Response{Header: header}
	bodyBuffer := bytes.NewBufferString(sampleResponse)
	r.Body = ioutil.NopCloser(bodyBuffer)
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, "eu-frankfurt-1", s.Rg.Name)
}

type testRnUnexported struct {
	Key  string `mandatory:"false" json:"key,omitempty"`
	Name string `mandatory:"false" json:"name,omitempty"`
}

type TestRn struct {
	Key  string `mandatory:"false" json:"key,omitempty"`
	Name string `mandatory:"false" json:"name,omitempty"`
}

type listRgRes struct {
	testRnUnexported `presentIn:"body"`
	OpcRequestID     string `presentIn:"header" name:"opcrequestid"`
}

type listRgResEx struct {
	TestRn       `presentIn:"body"`
	OpcRequestID string `presentIn:"header" name:"opcrequestid"`
}

type listRgResPtr struct {
	OpcRequestID               *string  `presentIn:"header" name:"opcrequestid"`
	NumericHeader              *int     `presentIn:"header" name:"numeric"`
	SomeTime                   *SDKTime `presentIn:"header" name:"theTime"`
	SomeBool                   *bool    `presentIn:"header" name:"aBool"`
	SomeUint                   *uint    `presentIn:"header" name:"aUint"`
	SomeFloat                  *float32 `presentIn:"header" name:"aFloat"`
	TestcreateAPIKeyDetailsPtr `presentIn:"body"`
}

func TestUnmarshalResponse_BodyAndHeaderUnex(t *testing.T) {
	sampleResponse := `{"key" : "RegionFRA","name" : "eu-frankfurt-1"}`
	header := http.Header{}
	opcID := "111"
	header.Set("OpcrequestId", opcID)
	s := listRgRes{}
	r := http.Response{Header: header}
	bodyBuffer := bytes.NewBufferString(sampleResponse)
	r.Body = ioutil.NopCloser(bodyBuffer)
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, opcID, s.OpcRequestID)
	assert.Equal(t, "", s.Name)
	assert.Equal(t, "", s.Key)
}

func TestUnmarshalResponse_BodyAndHeader(t *testing.T) {
	sampleResponse := `{"key" : "RegionFRA","name" : "eu-frankfurt-1"}`
	header := http.Header{}
	opcID := "111"
	header.Set("OpcrequestId", opcID)
	s := listRgResEx{}
	r := http.Response{Header: header}
	bodyBuffer := bytes.NewBufferString(sampleResponse)
	r.Body = ioutil.NopCloser(bodyBuffer)
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, opcID, s.OpcRequestID)
	assert.Equal(t, "eu-frankfurt-1", s.Name)
	assert.Equal(t, "RegionFRA", s.Key)
}

func TestUnmarshalResponse_PlainTextBody(t *testing.T) {
	sampleResponse := `some data not in json

isn\u0027t
some more data
and..$#04""234:: " 世界, 你好好好,  é,
 B=µH *`
	header := http.Header{}
	opcID := "111"
	header.Set("OpcrequestId", opcID)
	s := struct {
		Data *string `presentIn:"body" encoding:"plain-text"`
	}{}
	r := http.Response{Header: header}
	bodyBuffer := bytes.NewBufferString(sampleResponse)
	r.Body = ioutil.NopCloser(bodyBuffer)
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, sampleResponse, *(s.Data))
	assert.NotContains(t, sampleResponse, "isn't")
}

func TestUnmarshalResponse_BodyAndHeaderPtr(t *testing.T) {
	header := http.Header{}
	opcID := "111"
	numericHeader := "1414"
	someFloat := float32(2.332342)
	someUint := uint(33)
	theTime := SDKTime{time.Now()}
	theTimeStr := theTime.Format(sdkTimeFormat)
	sampleResponse := fmt.Sprintf(`{"key" : "RegionFRA","theTime" : "%s"}`, theTimeStr)
	header.Set("OpcrequestId", opcID)
	header.Set("numeric", numericHeader)
	header.Set("theTime", theTimeStr)
	header.Set("aBool", "true")
	header.Set("aUint", "33")
	header.Set("aFloat", "2.332342")
	s := listRgResPtr{}
	r := http.Response{Header: header}
	bodyBuffer := bytes.NewBufferString(sampleResponse)
	r.Body = ioutil.NopCloser(bodyBuffer)
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, opcID, *s.OpcRequestID)
	delta, _ := time.ParseDuration("1s")
	assert.WithinDuration(t, theTime.Time, s.SomeTime.Time, delta)
	assert.Equal(t, true, *s.SomeBool)
	assert.Equal(t, someFloat, *s.SomeFloat)
	assert.Equal(t, someUint, *s.SomeUint)
	assert.WithinDuration(t, theTime.Time, s.TheTime.Time, delta)
	assert.Equal(t, "RegionFRA", *s.Key)
}

type reqWithBinaryFiled struct {
	Content io.Reader `mandatory:"true" contributesTo:"body" encoding:"binary"`
}

func TestMarshalBinaryRequest(t *testing.T) {
	data := "some data in a file"
	buffer := bytes.NewBufferString(data)
	r := reqWithBinaryFiled{Content: ioutil.NopCloser(buffer)}
	httpRequest, err := MakeDefaultHTTPRequestWithTaggedStruct("PUT", "/obj", r)
	assert.NoError(t, err)
	all, err := ioutil.ReadAll(httpRequest.Body)
	assert.NoError(t, err)
	assert.Equal(t, data, string(all))
}

type structWithBinaryField struct {
	Content io.Reader `presentIn:"body" encoding:"binary"`
}

func TestUnmarshalResponse(t *testing.T) {
	data := "some data in a file"
	filename := writeTempFile(data)
	defer removeFileFn(filename)
	file, _ := os.Open(filename)
	header := http.Header{}
	r := http.Response{Header: header}
	r.Body = ioutil.NopCloser(file)
	s := structWithBinaryField{}
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	all, e := ioutil.ReadAll(s.Content)
	assert.NoError(t, e)
	assert.Equal(t, data, string(all))
}

type structWithHeaderCollections struct {
	Meta map[string]string `contributesTo:"header-collection" prefix:"meta-prefix-"`
}

func TestMarshalWithHeaderCollections(t *testing.T) {
	vals := make(map[string]string)
	vals["key1"] = "val1"
	vals["key2"] = "val2"
	s := structWithHeaderCollections{Meta: vals}

	request, err := MakeDefaultHTTPRequestWithTaggedStruct("GET", "/", s)
	assert.NoError(t, err)
	assert.Equal(t, s.Meta["key1"], request.Header.Get("meta-prefix-key1"))
	assert.Equal(t, s.Meta["key2"], request.Header.Get("Meta-prefix-key2"))
}

func TestMarshalWithHeaderCollections_BadCollectionType(t *testing.T) {
	vals := make(map[string]int)
	vals["key1"] = 1
	s := struct {
		Meta map[string]int `contributesTo:"header-collection" prefix:"meta-prefix-"`
	}{Meta: vals}

	_, err := MakeDefaultHTTPRequestWithTaggedStruct("GET", "/", s)
	assert.Error(t, err)
}

type responseWithHC struct {
	Meta map[string]string `presentIn:"header-collection" prefix:"meta-prefix-"`
}

func TestUnMarshalWithHeaderCollections(t *testing.T) {
	header := http.Header{}
	s := responseWithHC{}
	header.Set("meta-prefix-key1", "val1")
	header.Set("meta-prefix-key2", "val2")
	r := http.Response{Header: header}
	err := UnmarshalResponse(&r, &s)
	assert.NoError(t, err)
	assert.Equal(t, s.Meta["key1"], r.Header.Get("Meta-Prefix-Key1"))
	assert.Equal(t, s.Meta["key2"], r.Header.Get("Meta-Prefix-Key2"))
}

type responseWithEmptyQP struct {
	Meta    string `contributesTo:"query" omitEmpty:"true" name:"meta"`
	QParam  string `contributesTo:"query" omitEmpty:"false" name:"qp"`
	QParam2 string `contributesTo:"query" name:"qp2"`
}

func TestEmptyQueryParam(t *testing.T) {
	s := responseWithEmptyQP{}
	r, err := MakeDefaultHTTPRequestWithTaggedStruct("GET", "/", s)
	assert.NoError(t, err)
	assert.Contains(t, r.URL.RawQuery, "qp2")
	assert.Contains(t, r.URL.RawQuery, "qp")
	assert.NotContains(t, r.URL.RawQuery, "meta")
}

type responseWithWrongCsvType struct {
	Meta    string            `contributesTo:"query" omitEmpty:"true" name:"meta"`
	QParam  string            `contributesTo:"query" omitEmpty:"false" name:"qp"`
	QParam2 string            `contributesTo:"query" name:"qp2"`
	QParam3 map[string]string `contributesTo:"query" name:"qp2" collectionFormat:"csv"`
}

func TestWrongTypeQueryParamEncodingWrongType(t *testing.T) {
	m := make(map[string]string)
	m["one"] = "one"
	s := responseWithWrongCsvType{QParam3: m}
	_, err := MakeDefaultHTTPRequestWithTaggedStruct("GET", "/", s)
	assert.Error(t, err)
}

type responseUnsupportedQueryEncoding struct {
	Meta    string   `contributesTo:"query" omitEmpty:"true" name:"meta"`
	QParam  string   `contributesTo:"query" omitEmpty:"false" name:"qp"`
	QParam2 string   `contributesTo:"query" name:"qp2"`
	QParam3 []string `contributesTo:"query" name:"qp2" collectionFormat:"xml"`
}

func TestWrongTypeQueryParamWrongEncoding(t *testing.T) {
	s := responseUnsupportedQueryEncoding{QParam3: []string{"one ", "two"}}
	_, err := MakeDefaultHTTPRequestWithTaggedStruct("GET", "/", s)
	assert.Error(t, err)
}

func TestOmitFieldsInJson_SimpleStruct(t *testing.T) {
	type Nested struct {
		N   *string `mandatory:"false" json:"n"`
		NN  *string `mandatory:"false" json:"nn"`
		NNN string  `json:"nnn"`
	}
	val := ""
	s := Nested{NN: &val}
	sVal := reflect.ValueOf(s)
	jsonIn, _ := json.Marshal(s)
	m := make(map[string]interface{})
	json.Unmarshal(jsonIn, &m)
	mapRet, err := omitNilFieldsInJSON(m, sVal)
	assert.NoError(t, err)
	jsonRet, err := json.Marshal(mapRet)
	assert.NoError(t, err)
	assert.Equal(t, `{"nn":"","nnn":""}`, string(jsonRet))
}

func TestOmitFieldsInJson_SimpleStructWithSlice(t *testing.T) {
	type Nested struct {
		N            *string `mandatory:"false" json:"n"`
		NN           *string `mandatory:"false" json:"nn"`
		NNN          string  `json:"nnn"`
		Numbers      []int   `mandatory:"false" json:"numbers"`
		EmptyNumbers []int   `mandatory:"false" json:"enumbers"`
		NilNumbers   []int   `mandatory:"false" json:"nilnumbers"`
	}
	val := ""
	numbers := []int{1, 3}
	s := Nested{NN: &val, Numbers: numbers, EmptyNumbers: []int{}}
	sVal := reflect.ValueOf(s)
	jsonIn, _ := json.Marshal(s)
	m := make(map[string]interface{})
	json.Unmarshal(jsonIn, &m)
	mapRet, err := omitNilFieldsInJSON(m, sVal)
	assert.NoError(t, err)
	jsonRet, err := json.Marshal(mapRet)
	assert.NotContains(t, "nilnumbers", mapRet)
	assert.NoError(t, err)
	assert.Equal(t, `{"enumbers":[],"nn":"","nnn":"","numbers":[1,3]}`, string(jsonRet))
}

func TestOmitFieldsInJson_SimpleStructWithStruct(t *testing.T) {
	type InSstruct struct {
		AString      *string `mandatory:"false" json:"a"`
		ANilString   *string `mandatory:"false" json:"anil"`
		EmptyNumbers []int   `mandatory:"false" json:"aempty"`
	}

	type Nested struct {
		N        *string   `mandatory:"false" json:"n"`
		Numbers  []int     `mandatory:"false" json:"numbers"`
		ZComplex InSstruct `mandatory:"false" json:"complex"`
	}
	val := ""
	numbers := []int{1, 3}
	s := Nested{N: &val, Numbers: numbers, ZComplex: InSstruct{AString: &val, EmptyNumbers: []int{}}}
	sVal := reflect.ValueOf(s)
	jsonIn, _ := json.Marshal(s)
	m := make(map[string]interface{})
	json.Unmarshal(jsonIn, &m)
	mapRet, err := omitNilFieldsInJSON(m, sVal)
	assert.NoError(t, err)
	jsonRet, err := json.Marshal(mapRet)
	assert.NotContains(t, "nilnumbers", mapRet)
	assert.NoError(t, err)
	assert.Equal(t, `{"complex":{"a":"","aempty":[]},"n":"","numbers":[1,3]}`, string(jsonRet))
}

func TestOmitFieldsInJson_SimpleStructWithStructPtr(t *testing.T) {
	type InSstruct struct {
		AString      *string `mandatory:"false" json:"a"`
		ANilString   *string `mandatory:"false" json:"anil"`
		EmptyNumbers []int   `mandatory:"false" json:"aempty"`
	}

	type Nested struct {
		N        *string    `mandatory:"false" json:"n"`
		Numbers  []int      `mandatory:"false" json:"numbers"`
		ZComplex *InSstruct `mandatory:"false" json:"complex"`
	}
	val := ""
	numbers := []int{1, 3}
	s := Nested{N: &val, Numbers: numbers, ZComplex: &InSstruct{AString: &val, EmptyNumbers: []int{}}}
	sVal := reflect.ValueOf(s)
	jsonIn, _ := json.Marshal(s)
	m := make(map[string]interface{})
	json.Unmarshal(jsonIn, &m)
	mapRet, err := omitNilFieldsInJSON(m, sVal)
	assert.NoError(t, err)
	jsonRet, err := json.Marshal(mapRet)
	assert.NotContains(t, "nilnumbers", mapRet)
	assert.NoError(t, err)
	assert.Equal(t, `{"complex":{"a":"","aempty":[]},"n":"","numbers":[1,3]}`, string(jsonRet))
}

func TestOmitFieldsInJson_SimpleStructWithSliceStruct(t *testing.T) {
	type InSstruct struct {
		AString      *string `mandatory:"false" json:"a"`
		ANilString   *string `mandatory:"false" json:"anil"`
		EmptyNumbers []int   `mandatory:"false" json:"aempty"`
	}

	type Nested struct {
		//N *string `mandatory:"false" json:"n"`
		//Numbers []int `mandatory:"false" json:"numbers"`
		ZComplex []InSstruct `mandatory:"false" json:"complex"`
	}
	val := ""
	//numbers := []int{1, 3}
	//s := Nested{N:&val, Numbers: numbers, ZComplex:InSstruct{AString:&val, EmptyNumbers:[]int{}}}
	s := Nested{ZComplex: []InSstruct{{AString: &val, EmptyNumbers: []int{}}}}
	sVal := reflect.ValueOf(s)
	jsonIn, _ := json.Marshal(s)
	m := make(map[string]interface{})
	json.Unmarshal(jsonIn, &m)
	mapRet, err := omitNilFieldsInJSON(m, sVal)
	assert.NoError(t, err)
	jsonRet, err := json.Marshal(mapRet)
	assert.NotContains(t, "nilnumbers", mapRet)
	assert.NoError(t, err)
	assert.Equal(t, `{"complex":[{"a":"","aempty":[]}]}`, string(jsonRet))
}

func TestOmitEmptyEnumInJson_SimpleStructWithEnum(t *testing.T) {
	type TestEnum string

	const (
		TestEnumActive  TestEnum = "ACTIVE"
		TestEnumUnknown TestEnum = "UNKNOWN"
	)
	type TestStruct struct {
		MandatoryEnum TestEnum `mandatory:"true" json:"mandatoryenum"`
		OptionalEnum  TestEnum `mandatory:"false" json:"optionalenum,omitempty"`
		TestString    *string  `mandatory:"false" json:"teststring"`
	}

	type TestStruct2 struct {
		MandatoryEnum TestEnum `mandatory:"true" json:"mandatoryenum,omitempty"`
		OptionalEnum  TestEnum `mandatory:"false" json:"optionalenum"`
		TestString    *string  `mandatory:"false" json:"teststring"`
	}

	var enumTests = []struct {
		in  interface{} // input
		out string      // expected result
	}{
		{
			TestStruct{MandatoryEnum: TestEnumActive, TestString: String("teststring")},
			`{"mandatoryenum":"ACTIVE","teststring":"teststring"}`,
		},
		{
			TestStruct2{MandatoryEnum: TestEnumActive, TestString: String("teststring")},
			`{"mandatoryenum":"ACTIVE","optionalenum":"","teststring":"teststring"}`,
		},
	}

	for _, tt := range enumTests {
		b, err := json.Marshal(tt.in)
		assert.NoError(t, err)
		assert.Equal(t, tt.out, string(b))
	}
}

func TestOmitFieldsInJson_SimpleStructWithMapStruct(t *testing.T) {
	type InSstruct struct {
		AString      *string `mandatory:"false" json:"a"`
		ANilString   *string `mandatory:"false" json:"anil"`
		EmptyNumbers []int   `mandatory:"false" json:"aempty"`
	}

	type Nested struct {
		//N *string `mandatory:"false" json:"n"`
		//Numbers []int `mandatory:"false" json:"numbers"`
		ZComplex map[string]InSstruct `mandatory:"false" json:"complex"`
	}
	val := ""
	val2 := "two"
	//numbers := []int{1, 3}
	//s := Nested{N:&val, Numbers: numbers, ZComplex:InSstruct{AString:&val, EmptyNumbers:[]int{}}}
	data := make(map[string]InSstruct)
	data["one"] = InSstruct{AString: &val, EmptyNumbers: []int{}}
	data["two"] = InSstruct{AString: &val2, EmptyNumbers: []int{1}}
	data["ten"] = InSstruct{AString: &val2}

	s := Nested{ZComplex: data}
	sVal := reflect.ValueOf(s)
	jsonIn, _ := json.Marshal(s)
	m := make(map[string]interface{})
	json.Unmarshal(jsonIn, &m)
	mapRet, err := omitNilFieldsInJSON(m, sVal)
	assert.NoError(t, err)
	jsonRet, err := json.Marshal(mapRet)
	assert.NotContains(t, "nilnumbers", mapRet)
	assert.NoError(t, err)
	assert.Equal(t, `{"complex":{"one":{"a":"","aempty":[]},"ten":{"a":"two"},"two":{"a":"two","aempty":[1]}}}`, string(jsonRet))
}

func TestOmitFieldsInJson_removeFields(t *testing.T) {
	type MyEnum string
	type InSstruct struct {
		AString      *string `mandatory:"false" json:"a"`
		ANilString   *string `mandatory:"false" json:"anil"`
		ASecondEnum  MyEnum  `mandatory:"false" json:"secenum,omitempty"`
		ThirdEnum    MyEnum  `mandatory:"false" json:"tnum,omitempty"`
		EmptyNumbers []int   `mandatory:"false" json:"aempty"`
	}
	type Nested struct {
		N        *string              `mandatory:"false" json:"n"`
		AnEnum   MyEnum               `mandatory:"false" json:"anenum,omitempty"`
		AnEnum2  MyEnum               `mandatory:"false" json:"anenum2,omitempty"`
		ZComplex map[string]InSstruct `mandatory:"false" json:"complex"`
	}
	val := ""
	val2 := "two"
	//numbers := []int{1, 3}
	//s := Nested{N:&val, Numbers: numbers, ZComplex:InSstruct{AString:&val, EmptyNumbers:[]int{}}}
	data := make(map[string]InSstruct)
	data["one"] = InSstruct{AString: &val, EmptyNumbers: []int{}, ThirdEnum: MyEnum("enum")}
	data["two"] = InSstruct{AString: &val2, EmptyNumbers: []int{1}}
	data["ten"] = InSstruct{AString: &val2}

	s := Nested{ZComplex: data, AnEnum2: MyEnum("hello")}
	jsonIn, _ := json.Marshal(s)
	sVal := reflect.ValueOf(s)
	jsonRet, err := removeNilFieldsInJSONWithTaggedStruct(jsonIn, sVal)
	assert.NoError(t, err)
	assert.Equal(t, `{"anenum2":"hello","complex":{"one":{"a":"","aempty":[],"tnum":"enum"},"ten":{"a":"two"},"two":{"a":"two","aempty":[1]}}}`, string(jsonRet))
}

func TestOmitFieldsInJson_SimpleStructWithTime(t *testing.T) {
	type Nested struct {
		N       *string  `mandatory:"false" json:"n"`
		TheTime *SDKTime `mandatory:"true" json:"theTime"`
		NilTime *SDKTime `mandatory:"false" json:"nilTime"`
	}
	val := ""
	now := SDKTime{time.Now()}
	s := Nested{N: &val, TheTime: &now}
	sVal := reflect.ValueOf(s)
	jsonIn, _ := json.Marshal(s)
	m := make(map[string]interface{})
	json.Unmarshal(jsonIn, &m)
	theTime := m["theTime"]
	mapRet, err := omitNilFieldsInJSON(m, sVal)
	assert.NoError(t, err)
	assert.NotContains(t, mapRet, "nilTime")
	assert.Contains(t, mapRet, "n")
	assert.Contains(t, mapRet, "theTime")
	assert.Equal(t, theTime, mapRet.(map[string]interface{})["theTime"])
}

func TestAddRequestID(t *testing.T) {
	type testStructType struct {
		OpcRequestID *string `mandatory:"false" contributesTo:"header" name:"opc-request-id"`
	}

	inputTestDataSet := []testStructType{
		{},
		{OpcRequestID: String("testid")},
	}

	for _, testData := range inputTestDataSet {
		request := MakeDefaultHTTPRequest(http.MethodPost, "/random")
		HTTPRequestMarshaller(testData, &request)
		assert.NotEmpty(t, request.Header["Opc-Request-Id"])
		assert.Equal(t, 1, len(request.Header["Opc-Request-Id"]))

		if testData.OpcRequestID != nil {
			assert.Equal(t, "testid", request.Header["Opc-Request-Id"][0])
		}
	}
}
