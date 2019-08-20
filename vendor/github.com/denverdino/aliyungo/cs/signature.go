package cs

import (
	"net/http"
	"sort"
	"strings"

	"log"

	"github.com/denverdino/aliyungo/util"
)

func (client *Client) signRequest(request *http.Request) {

	headers := request.Header
	contentMd5 := headers.Get("Content-Md5")
	contentType := headers.Get("Content-Type")
	accept := headers.Get("Accept")
	date := headers.Get("Date")

	canonicalizedResource := request.URL.RequestURI()

	_, canonicalizedHeader := canonicalizeHeader(headers)

	stringToSign := request.Method + "\n" + accept + "\n" + contentMd5 + "\n" + contentType + "\n" + date + "\n" + canonicalizedHeader + canonicalizedResource

	if client.debug {
		log.Printf("stringToSign = %s: ", stringToSign)
	}

	signature := util.CreateSignature(stringToSign, client.AccessKeySecret)
	headers.Set("Authorization", "acs "+client.AccessKeyId+":"+signature)
}

const headerOSSPrefix = "x-acs-"

//Have to break the abstraction to append keys with lower case.
func canonicalizeHeader(headers http.Header) (newHeaders http.Header, result string) {
	var canonicalizedHeaders []string
	newHeaders = http.Header{}

	for k, v := range headers {
		if lower := strings.ToLower(k); strings.HasPrefix(lower, headerOSSPrefix) {
			newHeaders[lower] = v
			canonicalizedHeaders = append(canonicalizedHeaders, lower)
		} else {
			newHeaders[k] = v
		}
	}

	sort.Strings(canonicalizedHeaders)

	var canonicalizedHeader string

	for _, k := range canonicalizedHeaders {
		v := ""
		if len(headers[k]) > 0 {
			v = headers[k][0]
		}
		canonicalizedHeader += k + ":" + v + "\n"
	}

	return newHeaders, canonicalizedHeader
}
