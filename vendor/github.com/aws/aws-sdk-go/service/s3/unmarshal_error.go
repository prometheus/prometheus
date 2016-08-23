package s3

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
)

type xmlErrorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

func unmarshalError(r *request.Request) {
	defer r.HTTPResponse.Body.Close()
	defer io.Copy(ioutil.Discard, r.HTTPResponse.Body)

	if r.HTTPResponse.StatusCode == http.StatusMovedPermanently {
		r.Error = awserr.NewRequestFailure(
			awserr.New("BucketRegionError",
				fmt.Sprintf("incorrect region, the bucket is not in '%s' region",
					aws.StringValue(r.Config.Region)),
				nil),
			r.HTTPResponse.StatusCode,
			r.RequestID,
		)
		return
	}

	if r.HTTPResponse.ContentLength == 0 {
		// No body, use status code to generate an awserr.Error
		r.Error = awserr.NewRequestFailure(
			awserr.New(strings.Replace(r.HTTPResponse.Status, " ", "", -1), r.HTTPResponse.Status, nil),
			r.HTTPResponse.StatusCode,
			r.RequestID,
		)
		return
	}

	resp := &xmlErrorResponse{}
	err := xml.NewDecoder(r.HTTPResponse.Body).Decode(resp)
	if err != nil && err != io.EOF {
		r.Error = awserr.New("SerializationError", "failed to decode S3 XML error response", nil)
	} else {
		r.Error = awserr.NewRequestFailure(
			awserr.New(resp.Code, resp.Message, nil),
			r.HTTPResponse.StatusCode,
			r.RequestID,
		)
	}
}
