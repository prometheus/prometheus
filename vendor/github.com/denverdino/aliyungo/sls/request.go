package sls

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/denverdino/aliyungo/util"
)

type request struct {
	endpoint    string
	method      string
	path        string
	contentType string
	params      map[string]string
	headers     map[string]string
	payload     []byte
}

func (req *request) url() string {
	params := &url.Values{}

	if req.params != nil {
		for k, v := range req.params {
			params.Set(k, v)
		}
	}

	u := url.URL{
		Scheme:   "http",
		Host:     req.endpoint,
		Path:     req.path,
		RawQuery: params.Encode(),
	}
	return u.String()
}

func (client *Client) doRequest(req *request) (*http.Response, error) {

	payload := req.payload

	if req.headers == nil {
		req.headers = make(map[string]string)
	}

	if req.endpoint == "" {
		req.endpoint = client.endpoint
	}

	contentLength := "0"

	if payload != nil {
		contentLength = strconv.Itoa(len(payload))
	}

	req.headers["Content-Type"] = req.contentType
	req.headers["Content-Length"] = contentLength
	req.headers["x-log-bodyrawsize"] = contentLength
	req.headers["Date"] = util.GetGMTime()
	req.headers["Host"] = req.endpoint
	req.headers["x-log-apiversion"] = client.version
	req.headers["x-log-signaturemethod"] = "hmac-sha1"
	if client.securityToken != "" {
		req.headers["x-acs-security-token"] = client.securityToken
	}

	client.signRequest(req, payload)

	var reader io.Reader

	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	hreq, err := http.NewRequest(req.method, req.url(), reader)

	for k, v := range req.headers {
		if v != "" {
			hreq.Header.Set(k, v)
		}
	}

	if err != nil {
		return nil, err
	}
	t0 := time.Now()
	resp, err := client.httpClient.Do(hreq)
	t1 := time.Now()
	if err != nil {
		return nil, err
	}

	if client.debug {
		log.Printf("Invoke %s %s %d (%v)", req.method, req.url(), resp.StatusCode, t1.Sub(t0))
	}

	if resp.StatusCode != 200 && resp.StatusCode != 204 && resp.StatusCode != 206 {
		return nil, buildError(resp)
	}
	return resp, nil
}

func (client *Client) requestWithJsonResponse(req *request, v interface{}) error {
	resp, err := client.doRequest(req)

	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

func (client *Client) requestWithClose(req *request) error {
	resp, err := client.doRequest(req)
	if err != nil {
		return err
	}

	resp.Body.Close()
	return nil
}

type Error struct {
	StatusCode int
	Code       string `json:"errorCode,omitempty"`
	Message    string `json:"errorMessage,omitempty"`
}

func (err *Error) Error() string {
	return fmt.Sprintf("Aliyun API Error: Status Code: %d Code: %s Message: %s", err.StatusCode, err.Code, err.Message)
}

func buildError(resp *http.Response) error {
	defer resp.Body.Close()
	err := &Error{}
	json.NewDecoder(resp.Body).Decode(err)
	err.StatusCode = resp.StatusCode
	if err.Message == "" {
		err.Message = resp.Status
	}
	return err
}
