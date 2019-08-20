package oss

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/denverdino/aliyungo/common"
	"github.com/denverdino/aliyungo/util"
)

const DefaultContentType = "application/octet-stream"

// The Client type encapsulates operations with an OSS region.
type Client struct {
	AccessKeyId     string
	AccessKeySecret string
	SecurityToken   string
	Region          Region
	Internal        bool
	Secure          bool
	ConnectTimeout  time.Duration

	endpoint string
	debug    bool
}

// The Bucket type encapsulates operations with an bucket.
type Bucket struct {
	*Client
	Name string
}

// The Owner type represents the owner of the object in an bucket.
type Owner struct {
	ID          string
	DisplayName string
}

// Options struct
//
type Options struct {
	ServerSideEncryption bool
	Meta                 map[string][]string
	ContentEncoding      string
	CacheControl         string
	ContentMD5           string
	ContentDisposition   string
	//Range              string
	//Expires int
}

type CopyOptions struct {
	Headers           http.Header
	CopySourceOptions string
	MetadataDirective string
	//ContentType       string
}

// CopyObjectResult is the output from a Copy request
type CopyObjectResult struct {
	ETag         string
	LastModified string
}

var attempts = util.AttemptStrategy{
	Min:   5,
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// NewOSSClient creates a new OSS.

func NewOSSClientForAssumeRole(region Region, internal bool, accessKeyId string, accessKeySecret string, securityToken string, secure bool) *Client {
	return &Client{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		SecurityToken:   securityToken,
		Region:          region,
		Internal:        internal,
		debug:           false,
		Secure:          secure,
	}
}

func NewOSSClient(region Region, internal bool, accessKeyId string, accessKeySecret string, secure bool) *Client {
	return &Client{
		AccessKeyId:     accessKeyId,
		AccessKeySecret: accessKeySecret,
		Region:          region,
		Internal:        internal,
		debug:           false,
		Secure:          secure,
	}
}

// SetDebug sets debug mode to log the request/response message
func (client *Client) SetDebug(debug bool) {
	client.debug = debug
}

// Bucket returns a Bucket with the given name.
func (client *Client) Bucket(name string) *Bucket {
	name = strings.ToLower(name)
	return &Bucket{
		Client: client,
		Name:   name,
	}
}

type BucketInfo struct {
	Name             string
	CreationDate     string
	ExtranetEndpoint string
	IntranetEndpoint string
	Location         string
	Grant            string `xml:"AccessControlList>Grant"`
}

type GetServiceResp struct {
	Owner   Owner
	Buckets []BucketInfo `xml:">Bucket"`
}

type GetBucketInfoResp struct {
	Bucket BucketInfo
}

// GetService gets a list of all buckets owned by an account.
func (client *Client) GetService() (*GetServiceResp, error) {
	bucket := client.Bucket("")

	r, err := bucket.Get("")
	if err != nil {
		return nil, err
	}

	// Parse the XML response.
	var resp GetServiceResp
	if err = xml.Unmarshal(r, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

type ACL string

const (
	Private           = ACL("private")
	PublicRead        = ACL("public-read")
	PublicReadWrite   = ACL("public-read-write")
	AuthenticatedRead = ACL("authenticated-read")
	BucketOwnerRead   = ACL("bucket-owner-read")
	BucketOwnerFull   = ACL("bucket-owner-full-control")
)

var createBucketConfiguration = `<CreateBucketConfiguration>
  <LocationConstraint>%s</LocationConstraint>
</CreateBucketConfiguration>`

// locationConstraint returns an io.Reader specifying a LocationConstraint if
// required for the region.
func (client *Client) locationConstraint() io.Reader {
	constraint := fmt.Sprintf(createBucketConfiguration, client.Region)
	return strings.NewReader(constraint)
}

// override default endpoint
func (client *Client) SetEndpoint(endpoint string) {
	// TODO check endpoint
	client.endpoint = endpoint
}

// Info query basic information about the bucket
//
// You can read doc at https://help.aliyun.com/document_detail/31968.html
func (b *Bucket) Info() (BucketInfo, error) {
	params := make(url.Values)
	params.Set("bucketInfo", "")
	r, err := b.GetWithParams("/", params)

	if err != nil {
		return BucketInfo{}, err
	}

	// Parse the XML response.
	var resp GetBucketInfoResp
	if err = xml.Unmarshal(r, &resp); err != nil {
		return BucketInfo{}, err
	}

	return resp.Bucket, nil
}

// PutBucket creates a new bucket.
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/bucket&PutBucket
func (b *Bucket) PutBucket(perm ACL) error {
	headers := make(http.Header)
	if perm != "" {
		headers.Set("x-oss-acl", string(perm))
	}
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    "/",
		headers: headers,
		payload: b.Client.locationConstraint(),
	}
	return b.Client.query(req, nil)
}

// DelBucket removes an existing bucket. All objects in the bucket must
// be removed before the bucket itself can be removed.
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/bucket&DeleteBucket
func (b *Bucket) DelBucket() (err error) {
	for attempt := attempts.Start(); attempt.Next(); {
		req := &request{
			method: "DELETE",
			bucket: b.Name,
			path:   "/",
		}

		err = b.Client.query(req, nil)
		if !shouldRetry(err) {
			break
		}
	}
	return err
}

// Get retrieves an object from an bucket.
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/object&GetObject
func (b *Bucket) Get(path string) (data []byte, err error) {
	body, err := b.GetReader(path)
	if err != nil {
		return nil, err
	}
	data, err = ioutil.ReadAll(body)
	body.Close()
	return data, err
}

// GetReader retrieves an object from an bucket,
// returning the body of the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading.
func (b *Bucket) GetReader(path string) (rc io.ReadCloser, err error) {
	resp, err := b.GetResponse(path)
	if resp != nil {
		return resp.Body, err
	}
	return nil, err
}

// GetResponse retrieves an object from an bucket,
// returning the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading
func (b *Bucket) GetResponse(path string) (resp *http.Response, err error) {
	return b.GetResponseWithHeaders(path, make(http.Header))
}

// GetResponseWithHeaders retrieves an object from an bucket
// Accepts custom headers to be sent as the second parameter
// returning the body of the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading
func (b *Bucket) GetResponseWithHeaders(path string, headers http.Header) (resp *http.Response, err error) {
	for attempt := attempts.Start(); attempt.Next(); {
		req := &request{
			bucket:  b.Name,
			path:    path,
			headers: headers,
		}
		err = b.Client.prepare(req)
		if err != nil {
			return nil, err
		}

		resp, err := b.Client.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	panic("unreachable")
}

// Get retrieves an object from an bucket.
func (b *Bucket) GetWithParams(path string, params url.Values) (data []byte, err error) {
	resp, err := b.GetResponseWithParamsAndHeaders(path, params, nil)
	if err != nil {
		return nil, err
	}
	data, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	return data, err
}

func (b *Bucket) GetResponseWithParamsAndHeaders(path string, params url.Values, headers http.Header) (resp *http.Response, err error) {
	for attempt := attempts.Start(); attempt.Next(); {
		req := &request{
			bucket:  b.Name,
			path:    path,
			params:  params,
			headers: headers,
		}
		err = b.Client.prepare(req)
		if err != nil {
			return nil, err
		}

		resp, err := b.Client.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	panic("unreachable")
}

// Exists checks whether or not an object exists on an bucket using a HEAD request.
func (b *Bucket) Exists(path string) (exists bool, err error) {
	for attempt := attempts.Start(); attempt.Next(); {
		req := &request{
			method: "HEAD",
			bucket: b.Name,
			path:   path,
		}
		err = b.Client.prepare(req)
		if err != nil {
			return
		}

		resp, err := b.Client.run(req, nil)

		if shouldRetry(err) && attempt.HasNext() {
			continue
		}

		if err != nil {
			// We can treat a 403 or 404 as non existence
			if e, ok := err.(*Error); ok && (e.StatusCode == 403 || e.StatusCode == 404) {
				return false, nil
			}
			return false, err
		}

		if resp.StatusCode/100 == 2 {
			exists = true
		}
		if resp.Body != nil {
			resp.Body.Close()
		}
		return exists, err
	}
	return false, fmt.Errorf("OSS Currently Unreachable")
}

// Head HEADs an object in the bucket, returns the response with
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/object&HeadObject
func (b *Bucket) Head(path string, headers http.Header) (*http.Response, error) {

	for attempt := attempts.Start(); attempt.Next(); {
		req := &request{
			method:  "HEAD",
			bucket:  b.Name,
			path:    path,
			headers: headers,
		}
		err := b.Client.prepare(req)
		if err != nil {
			return nil, err
		}

		resp, err := b.Client.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return resp, err
	}
	return nil, fmt.Errorf("OSS Currently Unreachable")
}

// Put inserts an object into the bucket.
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/object&PutObject
func (b *Bucket) Put(path string, data []byte, contType string, perm ACL, options Options) error {
	body := bytes.NewBuffer(data)
	return b.PutReader(path, body, int64(len(data)), contType, perm, options)
}

// PutCopy puts a copy of an object given by the key path into bucket b using b.Path as the target key
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/object&CopyObject
func (b *Bucket) PutCopy(path string, perm ACL, options CopyOptions, source string) (*CopyObjectResult, error) {
	headers := make(http.Header)

	headers.Set("x-oss-object-acl", string(perm))
	headers.Set("x-oss-copy-source", source)

	options.addHeaders(headers)
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
		timeout: 5 * time.Minute,
	}
	resp := &CopyObjectResult{}
	err := b.Client.query(req, resp)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// PutReader inserts an object into the bucket by consuming data
// from r until EOF.
func (b *Bucket) PutReader(path string, r io.Reader, length int64, contType string, perm ACL, options Options) error {
	headers := make(http.Header)
	headers.Set("Content-Length", strconv.FormatInt(length, 10))
	headers.Set("Content-Type", contType)
	headers.Set("x-oss-object-acl", string(perm))

	options.addHeaders(headers)
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
		payload: r,
	}
	return b.Client.query(req, nil)
}

// PutFile creates/updates object with file
func (b *Bucket) PutFile(path string, file *os.File, perm ACL, options Options) error {
	var contentType string
	if dotPos := strings.LastIndex(file.Name(), "."); dotPos == -1 {
		contentType = DefaultContentType
	} else {
		if mimeType := mime.TypeByExtension(file.Name()[dotPos:]); mimeType == "" {
			contentType = DefaultContentType
		} else {
			contentType = mimeType
		}
	}
	stats, err := file.Stat()
	if err != nil {
		log.Printf("Unable to read file %s stats.\n", file.Name())
		return err
	}

	return b.PutReader(path, file, stats.Size(), contentType, perm, options)
}

// addHeaders adds o's specified fields to headers
func (o Options) addHeaders(headers http.Header) {
	if o.ServerSideEncryption {
		headers.Set("x-oss-server-side-encryption", "AES256")
	}
	if len(o.ContentEncoding) != 0 {
		headers.Set("Content-Encoding", o.ContentEncoding)
	}
	if len(o.CacheControl) != 0 {
		headers.Set("Cache-Control", o.CacheControl)
	}
	if len(o.ContentMD5) != 0 {
		headers.Set("Content-MD5", o.ContentMD5)
	}
	if len(o.ContentDisposition) != 0 {
		headers.Set("Content-Disposition", o.ContentDisposition)
	}

	for k, v := range o.Meta {
		for _, mv := range v {
			headers.Add("x-oss-meta-"+k, mv)
		}
	}
}

// addHeaders adds o's specified fields to headers
func (o CopyOptions) addHeaders(headers http.Header) {
	if len(o.MetadataDirective) != 0 {
		headers.Set("x-oss-metadata-directive", o.MetadataDirective)
	}
	if len(o.CopySourceOptions) != 0 {
		headers.Set("x-oss-copy-source-range", o.CopySourceOptions)
	}
	if o.Headers != nil {
		for k, v := range o.Headers {
			newSlice := make([]string, len(v))
			copy(newSlice, v)
			headers[k] = newSlice
		}
	}
}

func makeXMLBuffer(doc []byte) *bytes.Buffer {
	buf := new(bytes.Buffer)
	buf.WriteString(xml.Header)
	buf.Write(doc)
	return buf
}

type IndexDocument struct {
	Suffix string `xml:"Suffix"`
}

type ErrorDocument struct {
	Key string `xml:"Key"`
}

type RoutingRule struct {
	ConditionKeyPrefixEquals     string `xml:"Condition>KeyPrefixEquals"`
	RedirectReplaceKeyPrefixWith string `xml:"Redirect>ReplaceKeyPrefixWith,omitempty"`
	RedirectReplaceKeyWith       string `xml:"Redirect>ReplaceKeyWith,omitempty"`
}

type RedirectAllRequestsTo struct {
	HostName string `xml:"HostName"`
	Protocol string `xml:"Protocol,omitempty"`
}

type WebsiteConfiguration struct {
	XMLName               xml.Name               `xml:"http://doc.oss-cn-hangzhou.aliyuncs.com WebsiteConfiguration"`
	IndexDocument         *IndexDocument         `xml:"IndexDocument,omitempty"`
	ErrorDocument         *ErrorDocument         `xml:"ErrorDocument,omitempty"`
	RoutingRules          *[]RoutingRule         `xml:"RoutingRules>RoutingRule,omitempty"`
	RedirectAllRequestsTo *RedirectAllRequestsTo `xml:"RedirectAllRequestsTo,omitempty"`
}

// PutBucketWebsite configures a bucket as a website.
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/bucket&PutBucketWebsite
func (b *Bucket) PutBucketWebsite(configuration WebsiteConfiguration) error {
	doc, err := xml.Marshal(configuration)
	if err != nil {
		return err
	}

	buf := makeXMLBuffer(doc)

	return b.PutBucketSubresource("website", buf, int64(buf.Len()))
}

func (b *Bucket) PutBucketSubresource(subresource string, r io.Reader, length int64) error {
	headers := make(http.Header)
	headers.Set("Content-Length", strconv.FormatInt(length, 10))

	req := &request{
		path:    "/",
		method:  "PUT",
		bucket:  b.Name,
		headers: headers,
		payload: r,
		params:  url.Values{subresource: {""}},
	}

	return b.Client.query(req, nil)
}

// Del removes an object from the bucket.
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/object&DeleteObject
func (b *Bucket) Del(path string) error {
	req := &request{
		method: "DELETE",
		bucket: b.Name,
		path:   path,
	}
	return b.Client.query(req, nil)
}

type Delete struct {
	Quiet   bool     `xml:"Quiet,omitempty"`
	Objects []Object `xml:"Object"`
}

type Object struct {
	Key       string `xml:"Key"`
	VersionId string `xml:"VersionId,omitempty"`
}

// DelMulti removes up to 1000 objects from the bucket.
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/object&DeleteMultipleObjects
func (b *Bucket) DelMulti(objects Delete) error {
	doc, err := xml.Marshal(objects)
	if err != nil {
		return err
	}

	buf := makeXMLBuffer(doc)
	digest := md5.New()
	size, err := digest.Write(buf.Bytes())
	if err != nil {
		return err
	}

	headers := make(http.Header)
	headers.Set("Content-Length", strconv.FormatInt(int64(size), 10))
	headers.Set("Content-MD5", base64.StdEncoding.EncodeToString(digest.Sum(nil)))
	headers.Set("Content-Type", "text/xml")

	req := &request{
		path:    "/",
		method:  "POST",
		params:  url.Values{"delete": {""}},
		bucket:  b.Name,
		headers: headers,
		payload: buf,
	}

	return b.Client.query(req, nil)
}

// The ListResp type holds the results of a List bucket operation.
type ListResp struct {
	Name      string
	Prefix    string
	Delimiter string
	Marker    string
	MaxKeys   int
	// IsTruncated is true if the results have been truncated because
	// there are more keys and prefixes than can fit in MaxKeys.
	// N.B. this is the opposite sense to that documented (incorrectly) in
	// http://goo.gl/YjQTc
	IsTruncated    bool
	Contents       []Key
	CommonPrefixes []string `xml:">Prefix"`
	// if IsTruncated is true, pass NextMarker as marker argument to List()
	// to get the next set of keys
	NextMarker string
}

// The Key type represents an item stored in an bucket.
type Key struct {
	Key          string
	LastModified string
	Type         string
	Size         int64
	// ETag gives the hex-encoded MD5 sum of the contents,
	// surrounded with double-quotes.
	ETag         string
	StorageClass string
	Owner        Owner
}

// List returns information about objects in an bucket.
//
// The prefix parameter limits the response to keys that begin with the
// specified prefix.
//
// The delim parameter causes the response to group all of the keys that
// share a common prefix up to the next delimiter in a single entry within
// the CommonPrefixes field. You can use delimiters to separate a bucket
// into different groupings of keys, similar to how folders would work.
//
// The marker parameter specifies the key to start with when listing objects
// in a bucket. OSS lists objects in alphabetical order and
// will return keys alphabetically greater than the marker.
//
// The max parameter specifies how many keys + common prefixes to return in
// the response, at most 1000. The default is 100.
//
// For example, given these keys in a bucket:
//
//     index.html
//     index2.html
//     photos/2006/January/sample.jpg
//     photos/2006/February/sample2.jpg
//     photos/2006/February/sample3.jpg
//     photos/2006/February/sample4.jpg
//
// Listing this bucket with delimiter set to "/" would yield the
// following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Contents:  []Key{
//             {Key: "index.html", "index2.html"},
//         },
//         CommonPrefixes: []string{
//             "photos/",
//         },
//     }
//
// Listing the same bucket with delimiter set to "/" and prefix set to
// "photos/2006/" would yield the following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Prefix:    "photos/2006/",
//         CommonPrefixes: []string{
//             "photos/2006/February/",
//             "photos/2006/January/",
//         },
//     }
//
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/bucket&GetBucket
func (b *Bucket) List(prefix, delim, marker string, max int) (result *ListResp, err error) {
	params := make(url.Values)
	params.Set("prefix", prefix)
	params.Set("delimiter", delim)
	params.Set("marker", marker)
	if max != 0 {
		params.Set("max-keys", strconv.FormatInt(int64(max), 10))
	}
	result = &ListResp{}
	for attempt := attempts.Start(); attempt.Next(); {
		req := &request{
			bucket: b.Name,
			params: params,
		}
		err = b.Client.query(req, result)
		if !shouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	// if NextMarker is not returned, it should be set to the name of last key,
	// so let's do it so that each caller doesn't have to
	if result.IsTruncated && result.NextMarker == "" {
		n := len(result.Contents)
		if n > 0 {
			result.NextMarker = result.Contents[n-1].Key
		}
	}
	return result, nil
}

type GetLocationResp struct {
	Location string `xml:",innerxml"`
}

func (b *Bucket) Location() (string, error) {
	params := make(url.Values)
	params.Set("location", "")
	r, err := b.GetWithParams("/", params)

	if err != nil {
		return "", err
	}

	// Parse the XML response.
	var resp GetLocationResp
	if err = xml.Unmarshal(r, &resp); err != nil {
		return "", err
	}

	if resp.Location == "" {
		return string(Hangzhou), nil
	}
	return resp.Location, nil
}

func (b *Bucket) Path(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "/" + b.Name + path
}

// URL returns a non-signed URL that allows retriving the
// object at path. It only works if the object is publicly
// readable (see SignedURL).
func (b *Bucket) URL(path string) string {
	req := &request{
		bucket: b.Name,
		path:   path,
	}
	err := b.Client.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url()
	if err != nil {
		panic(err)
	}
	u.RawQuery = ""
	return u.String()
}

// SignedURL returns a signed URL that allows anyone holding the URL
// to retrieve the object at path. The signature is valid until expires.
func (b *Bucket) SignedURL(path string, expires time.Time) string {
	return b.SignedURLWithArgs(path, expires, nil, nil)
}

// SignedURLWithArgs returns a signed URL that allows anyone holding the URL
// to retrieve the object at path. The signature is valid until expires.
func (b *Bucket) SignedURLWithArgs(path string, expires time.Time, params url.Values, headers http.Header) string {
	return b.SignedURLWithMethod("GET", path, expires, params, headers)
}

// SignedURLWithMethod returns a signed URL that allows anyone holding the URL
// to either retrieve the object at path or make a HEAD request against it. The signature is valid until expires.
func (b *Bucket) SignedURLWithMethod(method, path string, expires time.Time, params url.Values, headers http.Header) string {
	var uv = url.Values{}

	if params != nil {
		uv = params
	}

	uv.Set("Expires", strconv.FormatInt(expires.Unix(), 10))
	uv.Set("OSSAccessKeyId", b.AccessKeyId)

	req := &request{
		method:  method,
		bucket:  b.Name,
		path:    path,
		params:  uv,
		headers: headers,
	}
	err := b.Client.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url()
	if err != nil {
		panic(err)
	}

	return u.String()
}

// UploadSignedURL returns a signed URL that allows anyone holding the URL
// to upload the object at path. The signature is valid until expires.
// contenttype is a string like image/png
// name is the resource name in OSS terminology like images/ali.png [obviously excluding the bucket name itself]
func (b *Bucket) UploadSignedURL(name, method, contentType string, expires time.Time) string {
	//TODO TESTING
	expireDate := expires.Unix()
	if method != "POST" {
		method = "PUT"
	}

	tokenData := ""

	stringToSign := method + "\n\n" + contentType + "\n" + strconv.FormatInt(expireDate, 10) + "\n" + tokenData + "/" + path.Join(b.Name, name)
	secretKey := b.AccessKeySecret
	accessId := b.AccessKeyId
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(stringToSign))
	macsum := mac.Sum(nil)
	signature := base64.StdEncoding.EncodeToString(macsum)
	signature = strings.TrimSpace(signature)

	signedurl, err := url.Parse(b.Region.GetEndpoint(b.Internal, b.Name, b.Secure))
	if err != nil {
		log.Println("ERROR sining url for OSS upload", err)
		return ""
	}
	signedurl.Path = name
	params := url.Values{}
	params.Add("OSSAccessKeyId", accessId)
	params.Add("Expires", strconv.FormatInt(expireDate, 10))
	params.Add("Signature", signature)

	signedurl.RawQuery = params.Encode()
	return signedurl.String()
}

// PostFormArgsEx returns the action and input fields needed to allow anonymous
// uploads to a bucket within the expiration limit
// Additional conditions can be specified with conds
func (b *Bucket) PostFormArgsEx(path string, expires time.Time, redirect string, conds []string) (action string, fields map[string]string) {
	conditions := []string{}
	fields = map[string]string{
		"AWSAccessKeyId": b.AccessKeyId,
		"key":            path,
	}

	if conds != nil {
		conditions = append(conditions, conds...)
	}

	conditions = append(conditions, fmt.Sprintf("{\"key\": \"%s\"}", path))
	conditions = append(conditions, fmt.Sprintf("{\"bucket\": \"%s\"}", b.Name))
	if redirect != "" {
		conditions = append(conditions, fmt.Sprintf("{\"success_action_redirect\": \"%s\"}", redirect))
		fields["success_action_redirect"] = redirect
	}

	vExpiration := expires.Format("2006-01-02T15:04:05Z")
	vConditions := strings.Join(conditions, ",")
	policy := fmt.Sprintf("{\"expiration\": \"%s\", \"conditions\": [%s]}", vExpiration, vConditions)
	policy64 := base64.StdEncoding.EncodeToString([]byte(policy))
	fields["policy"] = policy64

	signer := hmac.New(sha1.New, []byte(b.AccessKeySecret))
	signer.Write([]byte(policy64))
	fields["signature"] = base64.StdEncoding.EncodeToString(signer.Sum(nil))

	action = fmt.Sprintf("%s/%s/", b.Client.Region, b.Name)
	return
}

// PostFormArgs returns the action and input fields needed to allow anonymous
// uploads to a bucket within the expiration limit
func (b *Bucket) PostFormArgs(path string, expires time.Time, redirect string) (action string, fields map[string]string) {
	return b.PostFormArgsEx(path, expires, redirect, nil)
}

type request struct {
	method   string
	bucket   string
	path     string
	params   url.Values
	headers  http.Header
	baseurl  string
	payload  io.Reader
	prepared bool
	timeout  time.Duration
}

func (req *request) url() (*url.URL, error) {
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return nil, fmt.Errorf("bad OSS endpoint URL %q: %v", req.baseurl, err)
	}
	u.RawQuery = req.params.Encode()
	u.Path = req.path
	return u, nil
}

// query prepares and runs the req request.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (client *Client) query(req *request, resp interface{}) error {
	err := client.prepare(req)
	if err != nil {
		return err
	}
	r, err := client.run(req, resp)
	if r != nil && r.Body != nil {
		r.Body.Close()
	}
	return err
}

// Sets baseurl on req from bucket name and the region endpoint
func (client *Client) setBaseURL(req *request) error {

	if client.endpoint == "" {
		req.baseurl = client.Region.GetEndpoint(client.Internal, req.bucket, client.Secure)
	} else {
		req.baseurl = fmt.Sprintf("%s://%s", getProtocol(client.Secure), client.endpoint)
	}

	return nil
}

// partiallyEscapedPath partially escapes the OSS path allowing for all OSS REST API calls.
//
// Some commands including:
//      GET Bucket acl              http://goo.gl/aoXflF
//      GET Bucket cors             http://goo.gl/UlmBdx
//      GET Bucket lifecycle        http://goo.gl/8Fme7M
//      GET Bucket policy           http://goo.gl/ClXIo3
//      GET Bucket location         http://goo.gl/5lh8RD
//      GET Bucket Logging          http://goo.gl/sZ5ckF
//      GET Bucket notification     http://goo.gl/qSSZKD
//      GET Bucket tagging          http://goo.gl/QRvxnM
// require the first character after the bucket name in the path to be a literal '?' and
// not the escaped hex representation '%3F'.
func partiallyEscapedPath(path string) string {
	pathEscapedAndSplit := strings.Split((&url.URL{Path: path}).String(), "/")
	if len(pathEscapedAndSplit) >= 3 {
		if len(pathEscapedAndSplit[2]) >= 3 {
			// Check for the one "?" that should not be escaped.
			if pathEscapedAndSplit[2][0:3] == "%3F" {
				pathEscapedAndSplit[2] = "?" + pathEscapedAndSplit[2][3:]
			}
		}
	}
	return strings.Replace(strings.Join(pathEscapedAndSplit, "/"), "+", "%2B", -1)
}

// prepare sets up req to be delivered to OSS.
func (client *Client) prepare(req *request) error {
	// Copy so they can be mutated without affecting on retries.
	headers := copyHeader(req.headers)
	if len(client.SecurityToken) != 0 {
		headers.Set("x-oss-security-token", client.SecurityToken)
	}

	params := make(url.Values)

	for k, v := range req.params {
		params[k] = v
	}

	req.params = params
	req.headers = headers

	if !req.prepared {
		req.prepared = true
		if req.method == "" {
			req.method = "GET"
		}

		if !strings.HasPrefix(req.path, "/") {
			req.path = "/" + req.path
		}

		err := client.setBaseURL(req)
		if err != nil {
			return err
		}
	}

	req.headers.Set("Date", util.GetGMTime())
	client.signRequest(req)

	return nil
}

// Prepares an *http.Request for doHttpRequest
func (client *Client) setupHttpRequest(req *request) (*http.Request, error) {
	// Copy so that signing the http request will not mutate it

	u, err := req.url()
	if err != nil {
		return nil, err
	}
	u.Opaque = fmt.Sprintf("//%s%s", u.Host, partiallyEscapedPath(u.Path))

	hreq := http.Request{
		URL:        u,
		Method:     req.method,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Close:      true,
		Header:     req.headers,
		Form:       req.params,
	}

	hreq.Header.Set("X-SDK-Client", `AliyunGO/`+common.Version)

	contentLength := req.headers.Get("Content-Length")

	if contentLength != "" {
		hreq.ContentLength, _ = strconv.ParseInt(contentLength, 10, 64)
		req.headers.Del("Content-Length")
	}

	if req.payload != nil {
		hreq.Body = ioutil.NopCloser(req.payload)
	}

	return &hreq, nil
}

// doHttpRequest sends hreq and returns the http response from the server.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (client *Client) doHttpRequest(c *http.Client, hreq *http.Request, resp interface{}) (*http.Response, error) {

	if client.debug {
		log.Printf("%s %s ...\n", hreq.Method, hreq.URL.String())
	}
	hresp, err := c.Do(hreq)
	if err != nil {
		return nil, err
	}
	if client.debug {
		log.Printf("%s %s %d\n", hreq.Method, hreq.URL.String(), hresp.StatusCode)
		contentType := hresp.Header.Get("Content-Type")
		if contentType == "application/xml" || contentType == "text/xml" {
			dump, _ := httputil.DumpResponse(hresp, true)
			log.Printf("%s\n", dump)
		} else {
			log.Printf("Response Content-Type: %s\n", contentType)
		}
	}
	if hresp.StatusCode != 200 && hresp.StatusCode != 204 && hresp.StatusCode != 206 {
		return nil, client.buildError(hresp)
	}
	if resp != nil {
		err = xml.NewDecoder(hresp.Body).Decode(resp)
		hresp.Body.Close()

		if client.debug {
			log.Printf("aliyungo.oss> decoded xml into %#v", resp)
		}

	}
	return hresp, err
}

// run sends req and returns the http response from the server.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (client *Client) run(req *request, resp interface{}) (*http.Response, error) {
	if client.debug {
		log.Printf("Running OSS request: %#v", req)
	}

	hreq, err := client.setupHttpRequest(req)
	if err != nil {
		return nil, err
	}

	c := &http.Client{
		Transport: &http.Transport{
			Dial: func(netw, addr string) (c net.Conn, err error) {
				if client.ConnectTimeout > 0 {
					c, err = net.DialTimeout(netw, addr, client.ConnectTimeout)
				} else {
					c, err = net.Dial(netw, addr)
				}
				if err != nil {
					return
				}
				return
			},
			Proxy: http.ProxyFromEnvironment,
		},
		Timeout: req.timeout,
	}

	return client.doHttpRequest(c, hreq, resp)
}

// Error represents an error in an operation with OSS.
type Error struct {
	StatusCode int    // HTTP status code (200, 403, ...)
	Code       string // OSS error code ("UnsupportedOperation", ...)
	Message    string // The human-oriented error message
	BucketName string
	RequestId  string
	HostId     string
}

func (e *Error) Error() string {
	return fmt.Sprintf("Aliyun API Error: RequestId: %s Status Code: %d Code: %s Message: %s", e.RequestId, e.StatusCode, e.Code, e.Message)
}

func (client *Client) buildError(r *http.Response) error {
	if client.debug {
		log.Printf("got error (status code %v)", r.StatusCode)
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("\tread error: %v", err)
		} else {
			log.Printf("\tdata:\n%s\n\n", data)
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	}

	err := Error{}
	// TODO return error if Unmarshal fails?
	xml.NewDecoder(r.Body).Decode(&err)
	r.Body.Close()
	err.StatusCode = r.StatusCode
	if err.Message == "" {
		err.Message = r.Status
	}
	if client.debug {
		log.Printf("err: %#v\n", err)
	}
	return &err
}

type TimeoutError interface {
	error
	Timeout() bool // Is the error a timeout?
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	_, ok := err.(TimeoutError)
	if ok {
		return true
	}

	switch err {
	case io.ErrUnexpectedEOF, io.EOF:
		return true
	}
	switch e := err.(type) {
	case *net.DNSError:
		return true
	case *net.OpError:
		switch e.Op {
		case "read", "write":
			return true
		}
	case *url.Error:
		// url.Error can be returned either by net/url if a URL cannot be
		// parsed, or by net/http if the response is closed before the headers
		// are received or parsed correctly. In that later case, e.Op is set to
		// the HTTP method name with the first letter uppercased. We don't want
		// to retry on POST operations, since those are not idempotent, all the
		// other ones should be safe to retry.
		switch e.Op {
		case "Get", "Put", "Delete", "Head":
			return shouldRetry(e.Err)
		default:
			return false
		}
	case *Error:
		switch e.Code {
		case "InternalError", "NoSuchUpload", "NoSuchBucket":
			return true
		}
	}
	return false
}

func hasCode(err error, code string) bool {
	e, ok := err.(*Error)
	return ok && e.Code == code
}

func copyHeader(header http.Header) (newHeader http.Header) {
	newHeader = make(http.Header)
	for k, v := range header {
		newSlice := make([]string, len(v))
		copy(newSlice, v)
		newHeader[k] = newSlice
	}
	return
}

type AccessControlPolicy struct {
	Owner  Owner
	Grants []string `xml:"AccessControlList>Grant"`
}

// ACL returns ACL of bucket
//
// You can read doc at http://docs.aliyun.com/#/pub/oss/api-reference/bucket&GetBucketAcl
func (b *Bucket) ACL() (result *AccessControlPolicy, err error) {

	params := make(url.Values)
	params.Set("acl", "")

	r, err := b.GetWithParams("/", params)
	if err != nil {
		return nil, err
	}

	// Parse the XML response.
	var resp AccessControlPolicy
	if err = xml.Unmarshal(r, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (b *Bucket) GetContentLength(sourcePath string) (int64, error) {
	resp, err := b.Head(sourcePath, nil)
	if err != nil {
		return 0, err
	}

	currentLength := resp.ContentLength

	return currentLength, err
}

func (b *Bucket) CopyLargeFile(sourcePath string, destPath string, contentType string, perm ACL, options Options) error {
	return b.CopyLargeFileInParallel(sourcePath, destPath, contentType, perm, options, 1)
}

const defaultChunkSize = int64(128 * 1024 * 1024) //128MB
const maxCopytSize = int64(128 * 1024 * 1024)     //128MB

// Copy large file in the same bucket
func (b *Bucket) CopyLargeFileInParallel(sourcePath string, destPath string, contentType string, perm ACL, options Options, maxConcurrency int) error {

	if maxConcurrency < 1 {
		maxConcurrency = 1
	}

	currentLength, err := b.GetContentLength(sourcePath)

	log.Printf("Parallel Copy large file[size: %d] from %s to %s\n", currentLength, sourcePath, destPath)

	if err != nil {
		return err
	}

	if currentLength < maxCopytSize {
		_, err := b.PutCopy(destPath, perm,
			CopyOptions{},
			b.Path(sourcePath))
		return err
	}

	multi, err := b.InitMulti(destPath, contentType, perm, options)
	if err != nil {
		return err
	}

	numParts := (currentLength + defaultChunkSize - 1) / defaultChunkSize
	completedParts := make([]Part, numParts)

	errChan := make(chan error, numParts)
	limiter := make(chan struct{}, maxConcurrency)

	var start int64 = 0
	var to int64 = 0
	var partNumber = 0
	sourcePathForCopy := b.Path(sourcePath)

	for start = 0; start < currentLength; start = to {
		to = start + defaultChunkSize
		if to > currentLength {
			to = currentLength
		}
		partNumber++

		rangeStr := fmt.Sprintf("bytes=%d-%d", start, to-1)
		limiter <- struct{}{}
		go func(partNumber int, rangeStr string) {
			_, part, err := multi.PutPartCopyWithContentLength(partNumber,
				CopyOptions{CopySourceOptions: rangeStr},
				sourcePathForCopy, currentLength)
			if err == nil {
				completedParts[partNumber-1] = part
			} else {
				log.Printf("Unable in PutPartCopy of part %d for %s: %v\n", partNumber, sourcePathForCopy, err)
			}
			errChan <- err
			<-limiter
		}(partNumber, rangeStr)
	}

	fullyCompleted := true
	for range completedParts {
		err := <-errChan
		if err != nil {
			fullyCompleted = false
		}
	}

	if fullyCompleted {
		err = multi.Complete(completedParts)
	} else {
		err = multi.Abort()
	}

	return err
}
