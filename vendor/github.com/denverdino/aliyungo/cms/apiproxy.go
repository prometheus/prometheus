package cms

import (
	"bytes"
	"log"
	"time"

	"github.com/denverdino/aliyungo/cms/util"
	//	"fmt"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
)

const (
	HEADER_SEPERATER = "\n"
	ACS_PREFIX       = "x-acs"
)

/**
 *  这个方法是先POP签名,并写入到协议头中
 * @params
 *  method http方法，GET POST DELETE PUT 等
 *  url 请求的url
 *  header http请求头
 *  querys GET请求参数，则需要设置querys参数
 * POP签名
 */
func (client *Client) Sign(method string, url string, req *http.Request, querys string) {

	header := req.Header

	var buf bytes.Buffer
	keys := make([]string, 0, len(header))
	for k := range header {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf.WriteString(method + HEADER_SEPERATER)
	for _, k := range keys {
		vs := header[k]
		//		prefix := strings.ToLower(k + ":")
		prefix := strings.ToLower(k + ":")
		for _, v := range vs {

			if v != "" {
				lowerKey := strings.ToLower(k)
				if strings.Contains(lowerKey, ACS_PREFIX) {
					buf.WriteString(prefix)
					buf.WriteString(v)
				} else {
					buf.WriteString(v)
				}

			}

			buf.WriteString("\n")
		}
	}

	//	写入url
	if querys != "" {
		url = url + "?" + querys
	}
	buf.WriteString(url)

	//fmt.Printf("SignString = %s ", buf.String())

	signiture := util.HmacSha1(client.GetAccessSecret(), buf.String())

	header.Add("Authorization", "acs "+client.GetAccessKey()+":"+signiture)
}

/**
 * 取得上下文请求路径
 */
func GetRequestPath(entity string, project string, id string) string {
	urlPath := ""

	if entity == "projects" {
		urlPath = urlPath + "/" + entity
	} else {
		urlPath = urlPath + "/projects/" + project + "/" + entity
	}

	if id != "" {
		if strings.HasPrefix(id, "?") {
			urlPath = urlPath + id
		} else {
			urlPath = urlPath + "/" + id
		}
	}

	return urlPath
}

/**
 * 去的要请求的url
 */
func (client *Client) GetUrl(entity string, project string, id string) string {
	var url = client.GetApiUri()
	url += GetRequestPath(entity, project, id)
	//	var pageStr = "",
	//	  filterStr = "";
	//	if (filter) {
	//	  for (var kk in filter) {
	//	    filterStr += "&" + kk + "=" + filter[kk];
	//	  }
	//	}
	//	if (pagination) {
	//	  pageStr = "&page=" + (Math.floor(pagination.index / pagination.size) + 1) + "&pageSize=" + pagination.size;
	//	}
	//	if (filterStr || pageStr) {
	//	  url += "?resource" + filterStr + pageStr;
	//	}
	//	return url;

	return url
}

/**
 * 取得公共的头部参数,初始化的时候先将Content
 */
func InitBaseHeader(v *http.Request) {

	v.Header["Accept"] = []string{"application/json"}
	v.Header.Set("Content-MD5", "")
	v.Header["Content-Type"] = []string{"application/json"}
	v.Header["Date"] = []string{util.GetRFCDate()}
	v.Header["x-acs-signature-method"] = []string{"HMAC-SHA1"}
	v.Header["x-acs-signature-version"] = []string{"1.0"}
	v.Header["x-acs-version"] = []string{"2015-08-15"}
}

/**
 * 对json进行md5 base64
 */
func BodyMd5(jsonstring string) string {
	return util.Md5Base64_16(jsonstring)
}

/**
 * 发送http请求，去的响应字符串
 */
func (c *Client) GetResponseJson(method string, requestUrl string, requestPath string, body string) (responseBody string, err error) {

	//	fmt.Println("method %s, requestPath: %s", requestUrl, requestPath)
	reqest, err := http.NewRequest(method, requestUrl, strings.NewReader(body))
	if err != nil {
		return responseBody, err
	}

	InitBaseHeader(reqest)

	//	如果是post请求，并且有post请求内容则加上Content-MD5头
	if body != "" && method == "POST" {
		reqest.Header.Set("Content-MD5", BodyMd5(body))
	}

	if c.securityToken != "" {
		reqest.Header["x-acs-security-token"] = []string{c.securityToken}
	}

	c.Sign(method, requestPath, reqest, "")

	if method != "POST" {
		reqest.Header.Del("Content-MD5")

	}
	//reqest.Header.Del("Accept-Encoding")
	reqest.Header.Set("Accept-Encoding", "deflate,sdch")

	log.Printf("RequestUrl = %++v", reqest)
	client := &http.Client{}
	t0 := time.Now()
	response, err := client.Do(reqest)
	t1 := time.Now()
	if err != nil {
		return body, err
	}

	if c.debug {
		log.Printf("Invoke %s %s %d (%v)", method, requestUrl, response.StatusCode, t1.Sub(t0))
	}

	rsBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	//如果状态吗是非200则进行返回值的过滤,使得pop返回的错误和程序的错误输出一致
	if response.StatusCode != 200 {
		err = errors.New("Response status code faild" + string(rsBody))
		type ResultError struct {
			RequestId string `json:"requestId"`
			HostId    string `json:"hostId"`
			Code      string `json:"code"`
			Message   string `json:"Message"`
		}
		var rsError ResultError
		rsBodyString := string(rsBody)
		_ = json.Unmarshal([]byte(rsBodyString), &rsError)

		newResult := ResultModel{
			rsError.Code,
			rsError.Message,
			false,
		}

		resultJson, _ := json.Marshal(newResult)

		return string(resultJson), err

	}

	return string(rsBody), err
}
