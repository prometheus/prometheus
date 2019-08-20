package sls

import (
	"crypto/md5"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"

	"github.com/denverdino/aliyungo/util"
)

const HeaderSLSPrefix1 = "x-log-"
const HeaderSLSPrefix2 = "x-acs-"

func (client *Client) signRequest(req *request, payload []byte) {

	//	SignString = VERB + "\n"
	//	+ CONTENT-MD5 + "\n"
	//	+ CONTENT-TYPE + "\n"
	//	+ DATE + "\n"
	//	+ CanonicalizedLOGHeaders + "\n"
	//	+ CanonicalizedResource

	if _, ok := req.headers["Authorization"]; ok {
		return
	}

	contentMd5 := ""
	contentType := req.headers["Content-Type"]
	if req.payload != nil {
		hasher := md5.New()
		hasher.Write(payload)
		contentMd5 = strings.ToUpper(hex.EncodeToString(hasher.Sum(nil))) //string(md5.Sum(contentMd5))
		req.headers["Content-MD5"] = contentMd5

	}
	date := req.headers["Date"]
	//date := util.GetGMTime()
	canonicalizedHeader := canonicalizeHeader(req.headers)

	canonicalizedResource := canonicalizeResource(req)

	signString := req.method + "\n" + contentMd5 + "\n" + contentType + "\n" + date + "\n" + canonicalizedHeader + "\n" + canonicalizedResource

	signature := util.CreateSignature(signString, client.accessKeySecret)
	req.headers["Authorization"] = "LOG " + client.accessKeyId + ":" + signature
}

func canonicalizeResource(req *request) string {
	canonicalizedResource := req.path
	var paramNames []string
	if req.params != nil && len(req.params) > 0 {
		for k, _ := range req.params {
			paramNames = append(paramNames, k)
		}
		sort.Strings(paramNames)

		var query []string
		for _, k := range paramNames {
			query = append(query, url.QueryEscape(k)+"="+url.QueryEscape(req.params[k]))
		}
		canonicalizedResource = canonicalizedResource + "?" + strings.Join(query, "&")
	}
	return canonicalizedResource
}

//Have to break the abstraction to append keys with lower case.
func canonicalizeHeader(headers map[string]string) string {
	var canonicalizedHeaders []string

	for k, _ := range headers {
		if lower := strings.ToLower(k); strings.HasPrefix(lower, HeaderSLSPrefix1) || strings.HasPrefix(lower, HeaderSLSPrefix2) {
			canonicalizedHeaders = append(canonicalizedHeaders, lower)
		}
	}

	sort.Strings(canonicalizedHeaders)

	var headersWithValue []string

	for _, k := range canonicalizedHeaders {
		headersWithValue = append(headersWithValue, k+":"+headers[k])
	}
	return strings.Join(headersWithValue, "\n")
}
