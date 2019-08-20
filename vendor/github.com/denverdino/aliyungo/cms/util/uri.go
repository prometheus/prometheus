package util

import (
	"net/url"
	"strings"
)

/**
 * url编码特殊过滤，和标准的url编码有小差别
 */
func QueryEscape(str string) string {
	encodeString := url.QueryEscape(str)

	encodeString = strings.Replace(encodeString, "+", "%20", -1)
	encodeString = strings.Replace(encodeString, "*", "%2A", -1)

	return encodeString
}

/**
 * url解码特殊过滤，和标准的url编码有小差别
 */
func QueryUnEscape(str string) string {
	str = strings.Replace(str, "%20", "+", -1)
	str = strings.Replace(str, "%2A", "*", -1)

	encodeString, _ := url.QueryUnescape(str)

	return encodeString
}
