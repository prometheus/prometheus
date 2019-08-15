package responses

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeHTTPResponse(statusCode int, content string) (res *http.Response) {
	header := make(http.Header)
	status := strconv.Itoa(statusCode)
	res = &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		Header:     header,
		StatusCode: statusCode,
		Status:     status + " " + http.StatusText(statusCode),
	}
	res.Header = make(http.Header)
	res.Body = ioutil.NopCloser(bytes.NewReader([]byte(content)))
	return
}

func Test_CommonResponse(t *testing.T) {
	r := NewCommonResponse()
	assert.NotNil(t, r)

	assert.Equal(t, 0, r.GetHttpStatus())
	// assert.Equal(t, nil, r.GetHttpHeaders())
	assert.Equal(t, "", r.GetHttpContentString())
	assert.Equal(t, 0, len(r.GetHttpContentBytes()))
	assert.Nil(t, r.GetOriginHttpResponse())
	assert.False(t, r.IsSuccess())
}

func Test_CommonResponse_parseFromHttpResponse(t *testing.T) {
	r := NewCommonResponse()
	res := makeHTTPResponse(200, "")
	res.Header.Add("Server", "GitHub.com")
	r.parseFromHttpResponse(res)
	expected := `HTTP/1.1 200 OK
Server: GitHub.com


`

	assert.True(t, r.IsSuccess())
	assert.Equal(t, "GitHub.com", r.GetHttpHeaders()["Server"][0])
	assert.Equal(t, expected, r.String())
}

func Test_CommonResponse_Unmarshal(t *testing.T) {
	r := NewCommonResponse()
	res := makeHTTPResponse(400, "")
	err := Unmarshal(r, res, "JSON")
	assert.NotNil(t, err)
	assert.Equal(t, "SDK.ServerError\nErrorCode: \nRecommend: \nRequestId: \nMessage: ", err.Error())
}

func Test_CommonResponse_Unmarshal_CommonResponse(t *testing.T) {
	r := NewCommonResponse()
	res := makeHTTPResponse(200, "")
	err := Unmarshal(r, res, "JSON")
	assert.Nil(t, err)
}

func Test_CommonResponse_Unmarshal_XML(t *testing.T) {
	r := &MyResponse{
		BaseResponse: &BaseResponse{},
	}
	res := makeHTTPResponse(200, `{"RequestId": "the request id"}`)
	err := Unmarshal(r, res, "XML")
	assert.NotNil(t, err)
}

type MyResponse struct {
	*BaseResponse
	RequestId string
}

func Test_CommonResponse_Unmarshal_EmptyContent(t *testing.T) {
	r := &MyResponse{
		BaseResponse: &BaseResponse{},
	}
	res := makeHTTPResponse(200, "")
	err := Unmarshal(r, res, "JSON")
	assert.Nil(t, err)
	assert.Equal(t, "", r.RequestId)
}

func Test_CommonResponse_Unmarshal_MyResponse(t *testing.T) {
	r := &MyResponse{
		BaseResponse: &BaseResponse{},
	}
	res := makeHTTPResponse(200, `{"RequestId": "the request id"}`)
	err := Unmarshal(r, res, "JSON")
	assert.Nil(t, err)
	assert.Equal(t, "the request id", r.RequestId)
}

func Test_CommonResponse_Unmarshal_Error(t *testing.T) {
	r := &MyResponse{
		BaseResponse: &BaseResponse{},
	}
	res := makeHTTPResponse(200, `{`)
	err := Unmarshal(r, res, "JSON")
	assert.NotNil(t, err)
	assert.Equal(t, "[SDK.JsonUnmarshalError] Failed to unmarshal response, but you can get the data via response.GetHttpStatusCode() and response.GetHttpContentString()\ncaused by:\nresponses.MyResponse.readFieldHash: expect \", but found \x00, error found in #1 byte of ...|{|..., bigger context ...|{|...", err.Error())
}
