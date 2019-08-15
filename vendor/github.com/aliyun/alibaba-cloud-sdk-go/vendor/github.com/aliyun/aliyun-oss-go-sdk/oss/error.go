package oss

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// ServiceError contains fields of the error response from Oss Service REST API.
type ServiceError struct {
	XMLName    xml.Name `xml:"Error"`
	Code       string   `xml:"Code"`      // The error code returned from OSS to the caller
	Message    string   `xml:"Message"`   // The detail error message from OSS
	RequestID  string   `xml:"RequestId"` // The UUID used to uniquely identify the request
	HostID     string   `xml:"HostId"`    // The OSS server cluster's Id
	Endpoint   string   `xml:"Endpoint"`
	RawMessage string   // The raw messages from OSS
	StatusCode int      // HTTP status code
}

// Error implements interface error
func (e ServiceError) Error() string {
	if e.Endpoint == "" {
		return fmt.Sprintf("oss: service returned error: StatusCode=%d, ErrorCode=%s, ErrorMessage=\"%s\", RequestId=%s",
			e.StatusCode, e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("oss: service returned error: StatusCode=%d, ErrorCode=%s, ErrorMessage=\"%s\", RequestId=%s, Endpoint=%s",
		e.StatusCode, e.Code, e.Message, e.RequestID, e.Endpoint)
}

// UnexpectedStatusCodeError is returned when a storage service responds with neither an error
// nor with an HTTP status code indicating success.
type UnexpectedStatusCodeError struct {
	allowed []int // The expected HTTP stats code returned from OSS
	got     int   // The actual HTTP status code from OSS
}

// Error implements interface error
func (e UnexpectedStatusCodeError) Error() string {
	s := func(i int) string { return fmt.Sprintf("%d %s", i, http.StatusText(i)) }

	got := s(e.got)
	expected := []string{}
	for _, v := range e.allowed {
		expected = append(expected, s(v))
	}
	return fmt.Sprintf("oss: status code from service response is %s; was expecting %s",
		got, strings.Join(expected, " or "))
}

// Got is the actual status code returned by oss.
func (e UnexpectedStatusCodeError) Got() int {
	return e.got
}

// checkRespCode returns UnexpectedStatusError if the given response code is not
// one of the allowed status codes; otherwise nil.
func checkRespCode(respCode int, allowed []int) error {
	for _, v := range allowed {
		if respCode == v {
			return nil
		}
	}
	return UnexpectedStatusCodeError{allowed, respCode}
}

// CRCCheckError is returned when crc check is inconsistent between client and server
type CRCCheckError struct {
	clientCRC uint64 // Calculated CRC64 in client
	serverCRC uint64 // Calculated CRC64 in server
	operation string // Upload operations such as PutObject/AppendObject/UploadPart, etc
	requestID string // The request id of this operation
}

// Error implements interface error
func (e CRCCheckError) Error() string {
	return fmt.Sprintf("oss: the crc of %s is inconsistent, client %d but server %d; request id is %s",
		e.operation, e.clientCRC, e.serverCRC, e.requestID)
}

func checkDownloadCRC(clientCRC, serverCRC uint64) error {
	if clientCRC == serverCRC {
		return nil
	}
	return CRCCheckError{clientCRC, serverCRC, "DownloadFile", ""}
}

func checkCRC(resp *Response, operation string) error {
	if resp.Headers.Get(HTTPHeaderOssCRC64) == "" || resp.ClientCRC == resp.ServerCRC {
		return nil
	}
	return CRCCheckError{resp.ClientCRC, resp.ServerCRC, operation, resp.Headers.Get(HTTPHeaderOssRequestID)}
}
