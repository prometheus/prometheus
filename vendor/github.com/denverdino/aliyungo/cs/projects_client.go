package cs

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"strings"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

type ProjectClient struct {
	clusterId  string
	endpoint   string
	debug      bool
	userAgent  string
	httpClient *http.Client
}

func NewProjectClient(clusterId, endpoint string, clusterCerts ClusterCerts) (client *ProjectClient, err error) {

	certs, err := tls.X509KeyPair([]byte(clusterCerts.Cert), []byte(clusterCerts.Key))

	if err != nil {
		return
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(clusterCerts.CA))

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				Certificates:       []tls.Certificate{certs},
				ClientCAs:          caCertPool,
				ClientAuth:         tls.RequireAndVerifyClientCert,
			},
		},
	}

	client = &ProjectClient{
		clusterId:  clusterId,
		endpoint:   endpoint,
		httpClient: httpClient,
	}

	return
}

// SetDebug sets debug mode to log the request/response message
func (client *ProjectClient) SetDebug(debug bool) {
	client.debug = debug
}

// SetUserAgent sets user agent to log the request/response message
func (client *ProjectClient) SetUserAgent(userAgent string) {
	client.userAgent = userAgent
}

func (client *ProjectClient) ClusterId() string {
	return client.clusterId
}

func (client *ProjectClient) Endpoint() string {
	return client.endpoint
}

func (client *ProjectClient) Invoke(method string, path string, query url.Values, args interface{}, response interface{}) error {
	var reqBody []byte
	var err error
	var contentType string

	if args != nil {
		reqBody, err = json.Marshal(args)
		if err != nil {
			return err
		}
		contentType = "application/json"
	}

	requestURL := client.endpoint + path
	if query != nil && len(query) > 0 {
		requestURL = requestURL + "?" + util.Encode(query)
	}
	var bodyReader io.Reader
	if reqBody != nil {
		bodyReader = bytes.NewReader(reqBody)
	}

	httpReq, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		return common.GetClientError(err)
	}

	httpReq.Header.Set("Date", util.GetGMTime())
	httpReq.Header.Set("Accept", "application/json")

	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	if client.userAgent != "" {
		httpReq.Header.Set("User-Agent", client.userAgent)
	}

	t0 := time.Now()
	httpResp, err := client.httpClient.Do(httpReq)
	t1 := time.Now()
	if err != nil {
		return common.GetClientError(err)
	}
	statusCode := httpResp.StatusCode

	if client.debug {
		log.Printf("Invoke %s %s %d (%v)", method, requestURL, statusCode, t1.Sub(t0))
	}

	defer httpResp.Body.Close()
	body, err := ioutil.ReadAll(httpResp.Body)

	if err != nil {
		return common.GetClientError(err)
	}

	if client.debug {
		var prettyJSON bytes.Buffer
		err = json.Indent(&prettyJSON, body, "", "    ")
		log.Println(string(prettyJSON.Bytes()))
	}

	if statusCode >= 400 && statusCode <= 599 {
		errorResponse := common.ErrorResponse{}
		err = json.Unmarshal(body, &errorResponse)
		ecsError := &common.Error{
			ErrorResponse: errorResponse,
			StatusCode:    statusCode,
		}

		// Project error is not standard ErrorResponse and its body only contains error message
		if len(strings.TrimSpace(ecsError.Message)) <= 0 {
			ecsError.Message = strings.Trim(string(body[:]), "\n")
		}
		return ecsError
	}

	if response != nil {
		err = json.Unmarshal(body, response)
		if err != nil {
			return common.GetClientError(err)
		}
	}

	return nil
}
