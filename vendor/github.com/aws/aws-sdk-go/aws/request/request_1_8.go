// +build go1.8

package request

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

// NoBody is a http.NoBody reader instructing Go HTTP client to not include
// and body in the HTTP request.
var NoBody = http.NoBody

// ResetBody rewinds the request body back to its starting position, and
// sets the HTTP Request body reference. When the body is read prior
// to being sent in the HTTP request it will need to be rewound.
//
// ResetBody will automatically be called by the SDK's build handler, but if
// the request is being used directly ResetBody must be called before the request
// is Sent.  SetStringBody, SetBufferBody, and SetReaderBody will automatically
// call ResetBody.
//
// Will also set the Go 1.8's http.Request.GetBody member to allow retrying
// PUT/POST redirects.
func (r *Request) ResetBody() {
	body, err := r.getNextRequestBody()
	if err != nil {
		r.Error = awserr.New(ErrCodeSerialization,
			"failed to reset request body", err)
		return
	}

	r.HTTPRequest.Body = body
	r.HTTPRequest.GetBody = r.getNextRequestBody
}
