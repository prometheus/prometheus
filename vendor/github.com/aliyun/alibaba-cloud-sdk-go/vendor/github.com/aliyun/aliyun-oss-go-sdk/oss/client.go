// Package oss implements functions for access oss service.
// It has two main struct Client and Bucket.
package oss

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client SDK's entry point. It's for bucket related options such as create/delete/set bucket (such as set/get ACL/lifecycle/referer/logging/website).
// Object related operations are done by Bucket class.
// Users use oss.New to create Client instance.
//
type (
	// Client OSS client
	Client struct {
		Config     *Config      // OSS client configuration
		Conn       *Conn        // Send HTTP request
		HTTPClient *http.Client //http.Client to use - if nil will make its own
	}

	// ClientOption client option such as UseCname, Timeout, SecurityToken.
	ClientOption func(*Client)
)

// New creates a new client.
//
// endpoint    the OSS datacenter endpoint such as http://oss-cn-hangzhou.aliyuncs.com .
// accessKeyId    access key Id.
// accessKeySecret    access key secret.
//
// Client    creates the new client instance, the returned value is valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func New(endpoint, accessKeyID, accessKeySecret string, options ...ClientOption) (*Client, error) {
	// Configuration
	config := getDefaultOssConfig()
	config.Endpoint = endpoint
	config.AccessKeyID = accessKeyID
	config.AccessKeySecret = accessKeySecret

	// URL parse
	url := &urlMaker{}
	url.Init(config.Endpoint, config.IsCname, config.IsUseProxy)

	// HTTP connect
	conn := &Conn{config: config, url: url}

	// OSS client
	client := &Client{
		Config: config,
		Conn:   conn,
	}

	// Client options parse
	for _, option := range options {
		option(client)
	}

	// Create HTTP connection
	err := conn.init(config, url, client.HTTPClient)

	return client, err
}

// Bucket gets the bucket instance.
//
// bucketName    the bucket name.
// Bucket    the bucket object, when error is nil.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) Bucket(bucketName string) (*Bucket, error) {
	return &Bucket{
		client,
		bucketName,
	}, nil
}

// CreateBucket creates a bucket.
//
// bucketName    the bucket name, it's globably unique and immutable. The bucket name can only consist of lowercase letters, numbers and dash ('-').
//               It must start with lowercase letter or number and the length can only be between 3 and 255.
// options    options for creating the bucket, with optional ACL. The ACL could be ACLPrivate, ACLPublicRead, and ACLPublicReadWrite. By default it's ACLPrivate.
//            It could also be specified with StorageClass option, which supports StorageStandard, StorageIA(infrequent access), StorageArchive.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) CreateBucket(bucketName string, options ...Option) error {
	headers := make(map[string]string)
	handleOptions(headers, options)

	buffer := new(bytes.Buffer)

	isOptSet, val, _ := isOptionSet(options, storageClass)
	if isOptSet {
		cbConfig := createBucketConfiguration{StorageClass: val.(StorageClassType)}
		bs, err := xml.Marshal(cbConfig)
		if err != nil {
			return err
		}
		buffer.Write(bs)

		contentType := http.DetectContentType(buffer.Bytes())
		headers[HTTPHeaderContentType] = contentType
	}

	params := map[string]interface{}{}
	resp, err := client.do("PUT", bucketName, params, headers, buffer)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// ListBuckets lists buckets of the current account under the given endpoint, with optional filters.
//
// options    specifies the filters such as Prefix, Marker and MaxKeys. Prefix is the bucket name's prefix filter.
//            And marker makes sure the returned buckets' name are greater than it in lexicographic order.
//            Maxkeys limits the max keys to return, and by default it's 100 and up to 1000.
//            For the common usage scenario, please check out list_bucket.go in the sample.
// ListBucketsResponse    the response object if error is nil.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) ListBuckets(options ...Option) (ListBucketsResult, error) {
	var out ListBucketsResult

	params, err := getRawParams(options)
	if err != nil {
		return out, err
	}

	resp, err := client.do("GET", "", params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// IsBucketExist checks if the bucket exists
//
// bucketName    the bucket name.
//
// bool    true if it exists, and it's only valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) IsBucketExist(bucketName string) (bool, error) {
	listRes, err := client.ListBuckets(Prefix(bucketName), MaxKeys(1))
	if err != nil {
		return false, err
	}

	if len(listRes.Buckets) == 1 && listRes.Buckets[0].Name == bucketName {
		return true, nil
	}
	return false, nil
}

// DeleteBucket deletes the bucket. Only empty bucket can be deleted (no object and parts).
//
// bucketName    the bucket name.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) DeleteBucket(bucketName string) error {
	params := map[string]interface{}{}
	resp, err := client.do("DELETE", bucketName, params, nil, nil)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

// GetBucketLocation gets the bucket location.
//
// Checks out the following link for more information :
// https://help.aliyun.com/document_detail/oss/user_guide/oss_concept/endpoint.html
//
// bucketName    the bucket name
//
// string    bucket's datacenter location
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketLocation(bucketName string) (string, error) {
	params := map[string]interface{}{}
	params["location"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var LocationConstraint string
	err = xmlUnmarshal(resp.Body, &LocationConstraint)
	return LocationConstraint, err
}

// SetBucketACL sets bucket's ACL.
//
// bucketName    the bucket name
// bucketAcl    the bucket ACL: ACLPrivate, ACLPublicRead and ACLPublicReadWrite.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) SetBucketACL(bucketName string, bucketACL ACLType) error {
	headers := map[string]string{HTTPHeaderOssACL: string(bucketACL)}
	params := map[string]interface{}{}
	resp, err := client.do("PUT", bucketName, params, headers, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// GetBucketACL gets the bucket ACL.
//
// bucketName    the bucket name.
//
// GetBucketAclResponse    the result object, and it's only valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketACL(bucketName string) (GetBucketACLResult, error) {
	var out GetBucketACLResult
	params := map[string]interface{}{}
	params["acl"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// SetBucketLifecycle sets the bucket's lifecycle.
//
// For more information, checks out following link:
// https://help.aliyun.com/document_detail/oss/user_guide/manage_object/object_lifecycle.html
//
// bucketName    the bucket name.
// rules    the lifecycle rules. There're two kind of rules: absolute time expiration and relative time expiration in days and day/month/year respectively.
//          Check out sample/bucket_lifecycle.go for more details.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) SetBucketLifecycle(bucketName string, rules []LifecycleRule) error {
	lxml := lifecycleXML{Rules: convLifecycleRule(rules)}
	bs, err := xml.Marshal(lxml)
	if err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	contentType := http.DetectContentType(buffer.Bytes())
	headers := map[string]string{}
	headers[HTTPHeaderContentType] = contentType

	params := map[string]interface{}{}
	params["lifecycle"] = nil
	resp, err := client.do("PUT", bucketName, params, headers, buffer)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// DeleteBucketLifecycle deletes the bucket's lifecycle.
//
//
// bucketName    the bucket name.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) DeleteBucketLifecycle(bucketName string) error {
	params := map[string]interface{}{}
	params["lifecycle"] = nil
	resp, err := client.do("DELETE", bucketName, params, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

// GetBucketLifecycle gets the bucket's lifecycle settings.
//
// bucketName    the bucket name.
//
// GetBucketLifecycleResponse    the result object upon successful request. It's only valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketLifecycle(bucketName string) (GetBucketLifecycleResult, error) {
	var out GetBucketLifecycleResult
	params := map[string]interface{}{}
	params["lifecycle"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// SetBucketReferer sets the bucket's referer whitelist and the flag if allowing empty referrer.
//
// To avoid stealing link on OSS data, OSS supports the HTTP referrer header. A whitelist referrer could be set either by API or web console, as well as
// the allowing empty referrer flag. Note that this applies to requests from webbrowser only.
// For example, for a bucket os-example and its referrer http://www.aliyun.com, all requests from this URL could access the bucket.
// For more information, please check out this link :
// https://help.aliyun.com/document_detail/oss/user_guide/security_management/referer.html
//
// bucketName    the bucket name.
// referers    the referrer white list. A bucket could have a referrer list and each referrer supports one '*' and multiple '?' as wildcards.
//             The sample could be found in sample/bucket_referer.go
// allowEmptyReferer    the flag of allowing empty referrer. By default it's true.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) SetBucketReferer(bucketName string, referers []string, allowEmptyReferer bool) error {
	rxml := RefererXML{}
	rxml.AllowEmptyReferer = allowEmptyReferer
	if referers == nil {
		rxml.RefererList = append(rxml.RefererList, "")
	} else {
		for _, referer := range referers {
			rxml.RefererList = append(rxml.RefererList, referer)
		}
	}

	bs, err := xml.Marshal(rxml)
	if err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	contentType := http.DetectContentType(buffer.Bytes())
	headers := map[string]string{}
	headers[HTTPHeaderContentType] = contentType

	params := map[string]interface{}{}
	params["referer"] = nil
	resp, err := client.do("PUT", bucketName, params, headers, buffer)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// GetBucketReferer gets the bucket's referrer white list.
//
// bucketName    the bucket name.
//
// GetBucketRefererResponse    the result object upon successful request. It's only valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketReferer(bucketName string) (GetBucketRefererResult, error) {
	var out GetBucketRefererResult
	params := map[string]interface{}{}
	params["referer"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// SetBucketLogging sets the bucket logging settings.
//
// OSS could automatically store the access log. Only the bucket owner could enable the logging.
// Once enabled, OSS would save all the access log into hourly log files in a specified bucket.
// For more information, please check out https://help.aliyun.com/document_detail/oss/user_guide/security_management/logging.html
//
// bucketName    bucket name to enable the log.
// targetBucket    the target bucket name to store the log files.
// targetPrefix    the log files' prefix.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) SetBucketLogging(bucketName, targetBucket, targetPrefix string,
	isEnable bool) error {
	var err error
	var bs []byte
	if isEnable {
		lxml := LoggingXML{}
		lxml.LoggingEnabled.TargetBucket = targetBucket
		lxml.LoggingEnabled.TargetPrefix = targetPrefix
		bs, err = xml.Marshal(lxml)
	} else {
		lxml := loggingXMLEmpty{}
		bs, err = xml.Marshal(lxml)
	}

	if err != nil {
		return err
	}

	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	contentType := http.DetectContentType(buffer.Bytes())
	headers := map[string]string{}
	headers[HTTPHeaderContentType] = contentType

	params := map[string]interface{}{}
	params["logging"] = nil
	resp, err := client.do("PUT", bucketName, params, headers, buffer)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// DeleteBucketLogging deletes the logging configuration to disable the logging on the bucket.
//
// bucketName    the bucket name to disable the logging.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) DeleteBucketLogging(bucketName string) error {
	params := map[string]interface{}{}
	params["logging"] = nil
	resp, err := client.do("DELETE", bucketName, params, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

// GetBucketLogging gets the bucket's logging settings
//
// bucketName    the bucket name
// GetBucketLoggingResponse    the result object upon successful request. It's only valid when error is nil.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketLogging(bucketName string) (GetBucketLoggingResult, error) {
	var out GetBucketLoggingResult
	params := map[string]interface{}{}
	params["logging"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// SetBucketWebsite sets the bucket's static website's index and error page.
//
// OSS supports static web site hosting for the bucket data. When the bucket is enabled with that, you can access the file in the bucket like the way to access a static website.
// For more information, please check out: https://help.aliyun.com/document_detail/oss/user_guide/static_host_website.html
//
// bucketName    the bucket name to enable static web site.
// indexDocument    index page.
// errorDocument    error page.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) SetBucketWebsite(bucketName, indexDocument, errorDocument string) error {
	wxml := WebsiteXML{}
	wxml.IndexDocument.Suffix = indexDocument
	wxml.ErrorDocument.Key = errorDocument

	bs, err := xml.Marshal(wxml)
	if err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	contentType := http.DetectContentType(buffer.Bytes())
	headers := make(map[string]string)
	headers[HTTPHeaderContentType] = contentType

	params := map[string]interface{}{}
	params["website"] = nil
	resp, err := client.do("PUT", bucketName, params, headers, buffer)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// DeleteBucketWebsite deletes the bucket's static web site settings.
//
// bucketName    the bucket name.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) DeleteBucketWebsite(bucketName string) error {
	params := map[string]interface{}{}
	params["website"] = nil
	resp, err := client.do("DELETE", bucketName, params, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

// GetBucketWebsite gets the bucket's default page (index page) and the error page.
//
// bucketName    the bucket name
//
// GetBucketWebsiteResponse    the result object upon successful request. It's only valid when error is nil.
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketWebsite(bucketName string) (GetBucketWebsiteResult, error) {
	var out GetBucketWebsiteResult
	params := map[string]interface{}{}
	params["website"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// SetBucketCORS sets the bucket's CORS rules
//
// For more information, please check out https://help.aliyun.com/document_detail/oss/user_guide/security_management/cors.html
//
// bucketName    the bucket name
// corsRules    the CORS rules to set. The related sample code is in sample/bucket_cors.go.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) SetBucketCORS(bucketName string, corsRules []CORSRule) error {
	corsxml := CORSXML{}
	for _, v := range corsRules {
		cr := CORSRule{}
		cr.AllowedMethod = v.AllowedMethod
		cr.AllowedOrigin = v.AllowedOrigin
		cr.AllowedHeader = v.AllowedHeader
		cr.ExposeHeader = v.ExposeHeader
		cr.MaxAgeSeconds = v.MaxAgeSeconds
		corsxml.CORSRules = append(corsxml.CORSRules, cr)
	}

	bs, err := xml.Marshal(corsxml)
	if err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	buffer.Write(bs)

	contentType := http.DetectContentType(buffer.Bytes())
	headers := map[string]string{}
	headers[HTTPHeaderContentType] = contentType

	params := map[string]interface{}{}
	params["cors"] = nil
	resp, err := client.do("PUT", bucketName, params, headers, buffer)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusOK})
}

// DeleteBucketCORS deletes the bucket's static website settings.
//
// bucketName    the bucket name.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) DeleteBucketCORS(bucketName string) error {
	params := map[string]interface{}{}
	params["cors"] = nil
	resp, err := client.do("DELETE", bucketName, params, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkRespCode(resp.StatusCode, []int{http.StatusNoContent})
}

// GetBucketCORS gets the bucket's CORS settings.
//
// bucketName    the bucket name.
// GetBucketCORSResult    the result object upon successful request. It's only valid when error is nil.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketCORS(bucketName string) (GetBucketCORSResult, error) {
	var out GetBucketCORSResult
	params := map[string]interface{}{}
	params["cors"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// GetBucketInfo gets the bucket information.
//
// bucketName    the bucket name.
// GetBucketInfoResult    the result object upon successful request. It's only valid when error is nil.
//
// error    it's nil if no error, otherwise it's an error object.
//
func (client Client) GetBucketInfo(bucketName string) (GetBucketInfoResult, error) {
	var out GetBucketInfoResult
	params := map[string]interface{}{}
	params["bucketInfo"] = nil
	resp, err := client.do("GET", bucketName, params, nil, nil)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()

	err = xmlUnmarshal(resp.Body, &out)
	return out, err
}

// LimitUploadSpeed: set upload bandwidth limit speed,default is 0,unlimited
// upSpeed: KB/s, 0 is unlimited,default is 0
// error:it's nil if success, otherwise failure
func (client Client) LimitUploadSpeed(upSpeed int) error {
	if client.Config == nil {
		return fmt.Errorf("client config is nil")
	}
	return client.Config.LimitUploadSpeed(upSpeed)
}

// UseCname sets the flag of using CName. By default it's false.
//
// isUseCname    true: the endpoint has the CName, false: the endpoint does not have cname. Default is false.
//
func UseCname(isUseCname bool) ClientOption {
	return func(client *Client) {
		client.Config.IsCname = isUseCname
		client.Conn.url.Init(client.Config.Endpoint, client.Config.IsCname, client.Config.IsUseProxy)
	}
}

// Timeout sets the HTTP timeout in seconds.
//
// connectTimeoutSec    HTTP timeout in seconds. Default is 10 seconds. 0 means infinite (not recommended)
// readWriteTimeout    HTTP read or write's timeout in seconds. Default is 20 seconds. 0 means infinite.
//
func Timeout(connectTimeoutSec, readWriteTimeout int64) ClientOption {
	return func(client *Client) {
		client.Config.HTTPTimeout.ConnectTimeout =
			time.Second * time.Duration(connectTimeoutSec)
		client.Config.HTTPTimeout.ReadWriteTimeout =
			time.Second * time.Duration(readWriteTimeout)
		client.Config.HTTPTimeout.HeaderTimeout =
			time.Second * time.Duration(readWriteTimeout)
		client.Config.HTTPTimeout.IdleConnTimeout =
			time.Second * time.Duration(readWriteTimeout)
		client.Config.HTTPTimeout.LongTimeout =
			time.Second * time.Duration(readWriteTimeout*10)
	}
}

// SecurityToken sets the temporary user's SecurityToken.
//
// token    STS token
//
func SecurityToken(token string) ClientOption {
	return func(client *Client) {
		client.Config.SecurityToken = strings.TrimSpace(token)
	}
}

// EnableMD5 enables MD5 validation.
//
// isEnableMD5    true: enable MD5 validation; false: disable MD5 validation.
//
func EnableMD5(isEnableMD5 bool) ClientOption {
	return func(client *Client) {
		client.Config.IsEnableMD5 = isEnableMD5
	}
}

// MD5ThresholdCalcInMemory sets the memory usage threshold for computing the MD5, default is 16MB.
//
// threshold    the memory threshold in bytes. When the uploaded content is more than 16MB, the temp file is used for computing the MD5.
//
func MD5ThresholdCalcInMemory(threshold int64) ClientOption {
	return func(client *Client) {
		client.Config.MD5Threshold = threshold
	}
}

// EnableCRC enables the CRC checksum. Default is true.
//
// isEnableCRC    true: enable CRC checksum; false: disable the CRC checksum.
//
func EnableCRC(isEnableCRC bool) ClientOption {
	return func(client *Client) {
		client.Config.IsEnableCRC = isEnableCRC
	}
}

// UserAgent specifies UserAgent. The default is aliyun-sdk-go/1.2.0 (windows/-/amd64;go1.5.2).
//
// userAgent    the user agent string.
//
func UserAgent(userAgent string) ClientOption {
	return func(client *Client) {
		client.Config.UserAgent = userAgent
	}
}

// Proxy sets the proxy (optional). The default is not using proxy.
//
// proxyHost    the proxy host in the format "host:port". For example, proxy.com:80 .
//
func Proxy(proxyHost string) ClientOption {
	return func(client *Client) {
		client.Config.IsUseProxy = true
		client.Config.ProxyHost = proxyHost
		client.Conn.url.Init(client.Config.Endpoint, client.Config.IsCname, client.Config.IsUseProxy)
	}
}

// AuthProxy sets the proxy information with user name and password.
//
// proxyHost    the proxy host in the format "host:port". For example, proxy.com:80 .
// proxyUser    the proxy user name.
// proxyPassword    the proxy password.
//
func AuthProxy(proxyHost, proxyUser, proxyPassword string) ClientOption {
	return func(client *Client) {
		client.Config.IsUseProxy = true
		client.Config.ProxyHost = proxyHost
		client.Config.IsAuthProxy = true
		client.Config.ProxyUser = proxyUser
		client.Config.ProxyPassword = proxyPassword
		client.Conn.url.Init(client.Config.Endpoint, client.Config.IsCname, client.Config.IsUseProxy)
	}
}

//
// HTTPClient sets the http.Client in use to the one passed in
//
func HTTPClient(HTTPClient *http.Client) ClientOption {
	return func(client *Client) {
		client.HTTPClient = HTTPClient
	}
}

//
// SetLogLevel sets the oss sdk log level
//
func SetLogLevel(LogLevel int) ClientOption {
	return func(client *Client) {
		client.Config.LogLevel = LogLevel
	}
}

//
// SetLogLevel sets the oss sdk log level
//
func SetLogger(Logger *log.Logger) ClientOption {
	return func(client *Client) {
		client.Config.Logger = Logger
	}
}

// Private
func (client Client) do(method, bucketName string, params map[string]interface{},
	headers map[string]string, data io.Reader) (*Response, error) {
	return client.Conn.Do(method, bucketName, "", params,
		headers, data, 0, nil)
}
