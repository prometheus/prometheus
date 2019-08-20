package mns

import (
	"net/url"
	"sort"
	"strings"

	"github.com/denverdino/aliyungo/util"
)

const HeaderMNSPrefix = "x-mns-"

//授权签名
func (client *Client) SignRequest(req *request, payload []byte) {

	//	SignString = VERB + "\n"
	//	+ CONTENT-MD5 + "\n"
	//	+ CONTENT-TYPE + "\n"
	//	+ DATE + "\n"
	//	+ CanonicalizedLOGHeaders + "\n"
	//	+ CanonicalizedResource

	if _, ok := req.headers["Authorization"]; ok {
		return
	}

	contentType := req.headers["Content-Type"]
	contentMd5 := ""
	//Header中未加Content-MD5
	//if payload != nil {
	//	contentMd5 = Md5(payload)
	//	req.headers["Content-MD5"] = contentMd5
	//}
	date := req.headers["Date"]
	canonicalizedHeader := canonicalizeHeader(req.headers)
	canonicalizedResource := canonicalizeResource(req)

	signString := req.method + "\n" + contentMd5 + "\n" + contentType + "\n" + date + "\n" + canonicalizedHeader + "\n" + canonicalizedResource
	signature := util.CreateSignature(signString, client.AccessKeySecret)
	req.headers["Authorization"] = "MNS " + client.AccessKeyId + ":" + signature
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
		if lower := strings.ToLower(k); strings.HasPrefix(lower, HeaderMNSPrefix) {
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
