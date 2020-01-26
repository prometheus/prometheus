// +build go1.8

package jsonrpc

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
)

func TestUnmarshalError_SerializationError(t *testing.T) {
	cases := map[string]struct {
		Request     *request.Request
		ExpectMsg   string
		ExpectBytes []byte
	}{
		"empty body": {
			Request: &request.Request{
				Data: &struct{}{},
				HTTPResponse: &http.Response{
					StatusCode: 400,
					Header: http.Header{
						"X-Amzn-Requestid": []string{"abc123"},
					},
					Body: ioutil.NopCloser(
						bytes.NewReader([]byte{}),
					),
				},
			},
			ExpectMsg: "error message missing",
		},
		"HTML body": {
			Request: &request.Request{
				Data: &struct{}{},
				HTTPResponse: &http.Response{
					StatusCode: 400,
					Header: http.Header{
						"X-Amzn-Requestid": []string{"abc123"},
					},
					Body: ioutil.NopCloser(
						bytes.NewReader([]byte(`<html></html>`)),
					),
				},
			},
			ExpectBytes: []byte(`<html></html>`),
			ExpectMsg:   "failed decoding",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			req := c.Request

			UnmarshalError(req)
			if req.Error == nil {
				t.Fatal("expect error, got none")
			}

			aerr := req.Error.(awserr.RequestFailure)
			if e, a := request.ErrCodeSerialization, aerr.Code(); e != a {
				t.Errorf("expect %v, got %v", e, a)
			}

			uerr := aerr.OrigErr().(awserr.UnmarshalError)
			if e, a := c.ExpectMsg, uerr.Message(); !strings.Contains(a, e) {
				t.Errorf("Expect %q, in %q", e, a)
			}
			if e, a := c.ExpectBytes, uerr.Bytes(); !bytes.Equal(e, a) {
				t.Errorf("expect:\n%v\nactual:\n%v", hex.Dump(e), hex.Dump(a))
			}
		})
	}
}
