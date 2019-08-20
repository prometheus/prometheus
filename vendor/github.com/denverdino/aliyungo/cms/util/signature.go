package util

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/denverdino/aliyungo/cms/bytesbuffer"
)

/**
 对字符串进行md5运算
 @param
	data: 签名的内容
**/
func Md5Signature(data string) string {
	t := md5.New()
	io.WriteString(t, data)

	return fmt.Sprintf("%x", t.Sum(nil))
}

/**
 ** 对字符串进行md5，并对md5的结果进行base64编码
 * @param
 *  data: 签名的内容
 */
func Md5Base64_32(data string) string {
	md5String := Md5Signature(data)

	return base64.StdEncoding.EncodeToString([]byte(md5String))
}

/**
 * 对字符串进行md5编码，并且每2位转换为字符，然后进行base64
 * @param
 *  data:要编码的串
 **/
func Md5Base64_16(data string) string {
	md5String := Md5Signature(data)
	md5CharString, _ := bytesbuffer.Hex_to_str(md5String)

	return base64.StdEncoding.EncodeToString([]byte(md5CharString))
}

/**
 hmac_sha1签名
 @param:
	secret string:签名的key
	message string：签名的内容
**/
func HmacSha1(secret string, message string) string {
	key := []byte(secret)
	h := hmac.New(sha1.New, key)
	h.Write([]byte(message))

	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
