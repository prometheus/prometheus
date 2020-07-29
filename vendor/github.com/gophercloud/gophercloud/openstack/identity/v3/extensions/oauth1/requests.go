package oauth1

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/identity/v3/tokens"
	"github.com/gophercloud/gophercloud/pagination"
)

// Type SignatureMethod is a OAuth1 SignatureMethod type.
type SignatureMethod string

const (
	// HMACSHA1 is a recommended OAuth1 signature method.
	HMACSHA1 SignatureMethod = "HMAC-SHA1"

	// PLAINTEXT signature method is not recommended to be used in
	// production environment.
	PLAINTEXT SignatureMethod = "PLAINTEXT"

	// OAuth1TokenContentType is a supported content type for an OAuth1
	// token.
	OAuth1TokenContentType = "application/x-www-form-urlencoded"
)

// AuthOptions represents options for authenticating a user using OAuth1 tokens.
type AuthOptions struct {
	// OAuthConsumerKey is the OAuth1 Consumer Key.
	OAuthConsumerKey string `q:"oauth_consumer_key" required:"true"`

	// OAuthConsumerSecret is the OAuth1 Consumer Secret. Used to generate
	// an OAuth1 request signature.
	OAuthConsumerSecret string `required:"true"`

	// OAuthToken is the OAuth1 Request Token.
	OAuthToken string `q:"oauth_token" required:"true"`

	// OAuthTokenSecret is the OAuth1 Request Token Secret. Used to generate
	// an OAuth1 request signature.
	OAuthTokenSecret string `required:"true"`

	// OAuthSignatureMethod is the OAuth1 signature method the Consumer used
	// to sign the request. Supported values are "HMAC-SHA1" or "PLAINTEXT".
	// "PLAINTEXT" is not recommended for production usage.
	OAuthSignatureMethod SignatureMethod `q:"oauth_signature_method" required:"true"`

	// OAuthTimestamp is an OAuth1 request timestamp. If nil, current Unix
	// timestamp will be used.
	OAuthTimestamp *time.Time

	// OAuthNonce is an OAuth1 request nonce. Nonce must be a random string,
	// uniquely generated for each request. Will be generated automatically
	// when it is not set.
	OAuthNonce string `q:"oauth_nonce"`

	// AllowReauth allows Gophercloud to re-authenticate automatically
	// if/when your token expires.
	AllowReauth bool
}

// ToTokenV3HeadersMap builds the headers required for an OAuth1-based create
// request.
func (opts AuthOptions) ToTokenV3HeadersMap(headerOpts map[string]interface{}) (map[string]string, error) {
	q, err := buildOAuth1QueryString(opts, opts.OAuthTimestamp, "")
	if err != nil {
		return nil, err
	}

	signatureKeys := []string{opts.OAuthConsumerSecret, opts.OAuthTokenSecret}

	method := headerOpts["method"].(string)
	u := headerOpts["url"].(string)
	stringToSign := buildStringToSign(method, u, q.Query())
	signature := url.QueryEscape(signString(opts.OAuthSignatureMethod, stringToSign, signatureKeys))

	authHeader := buildAuthHeader(q.Query(), signature)

	headers := map[string]string{
		"Authorization": authHeader,
		"X-Auth-Token":  "",
	}

	return headers, nil
}

// ToTokenV3ScopeMap allows AuthOptions to satisfy the tokens.AuthOptionsBuilder
// interface.
func (opts AuthOptions) ToTokenV3ScopeMap() (map[string]interface{}, error) {
	return nil, nil
}

// CanReauth allows AuthOptions to satisfy the tokens.AuthOptionsBuilder
// interface.
func (opts AuthOptions) CanReauth() bool {
	return opts.AllowReauth
}

// ToTokenV3CreateMap builds a create request body.
func (opts AuthOptions) ToTokenV3CreateMap(map[string]interface{}) (map[string]interface{}, error) {
	// identityReq defines the "identity" portion of an OAuth1-based authentication
	// create request body.
	type identityReq struct {
		Methods []string `json:"methods"`
		OAuth1  struct{} `json:"oauth1"`
	}

	// authReq defines the "auth" portion of an OAuth1-based authentication
	// create request body.
	type authReq struct {
		Identity identityReq `json:"identity"`
	}

	// oauth1Request defines how  an OAuth1-based authentication create
	// request body looks.
	type oauth1Request struct {
		Auth authReq `json:"auth"`
	}

	var req oauth1Request

	req.Auth.Identity.Methods = []string{"oauth1"}
	return gophercloud.BuildRequestBody(req, "")
}

// Create authenticates and either generates a new OpenStack token from an
// OAuth1 token.
func Create(client *gophercloud.ServiceClient, opts tokens.AuthOptionsBuilder) (r tokens.CreateResult) {
	b, err := opts.ToTokenV3CreateMap(nil)
	if err != nil {
		r.Err = err
		return
	}

	headerOpts := map[string]interface{}{
		"method": "POST",
		"url":    authURL(client),
	}

	h, err := opts.ToTokenV3HeadersMap(headerOpts)
	if err != nil {
		r.Err = err
		return
	}

	resp, err := client.Post(authURL(client), b, &r.Body, &gophercloud.RequestOpts{
		MoreHeaders: h,
		OkCodes:     []int{201},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// CreateConsumerOptsBuilder allows extensions to add additional parameters to
// the CreateConsumer request.
type CreateConsumerOptsBuilder interface {
	ToOAuth1CreateConsumerMap() (map[string]interface{}, error)
}

// CreateConsumerOpts provides options used to create a new Consumer.
type CreateConsumerOpts struct {
	// Description is the consumer description.
	Description string `json:"description"`
}

// ToOAuth1CreateConsumerMap formats a CreateConsumerOpts into a create request.
func (opts CreateConsumerOpts) ToOAuth1CreateConsumerMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "consumer")
}

// Create creates a new Consumer.
func CreateConsumer(client *gophercloud.ServiceClient, opts CreateConsumerOptsBuilder) (r CreateConsumerResult) {
	b, err := opts.ToOAuth1CreateConsumerMap()
	if err != nil {
		r.Err = err
		return
	}
	resp, err := client.Post(consumersURL(client), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{201},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// Delete deletes a Consumer.
func DeleteConsumer(client *gophercloud.ServiceClient, id string) (r DeleteConsumerResult) {
	resp, err := client.Delete(consumerURL(client, id), nil)
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// List enumerates Consumers.
func ListConsumers(client *gophercloud.ServiceClient) pagination.Pager {
	return pagination.NewPager(client, consumersURL(client), func(r pagination.PageResult) pagination.Page {
		return ConsumersPage{pagination.LinkedPageBase{PageResult: r}}
	})
}

// GetConsumer retrieves details on a single Consumer by ID.
func GetConsumer(client *gophercloud.ServiceClient, id string) (r GetConsumerResult) {
	resp, err := client.Get(consumerURL(client, id), &r.Body, nil)
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// UpdateConsumerOpts provides options used to update a consumer.
type UpdateConsumerOpts struct {
	// Description is the consumer description.
	Description string `json:"description"`
}

// ToOAuth1UpdateConsumerMap formats an UpdateConsumerOpts into a consumer update
// request.
func (opts UpdateConsumerOpts) ToOAuth1UpdateConsumerMap() (map[string]interface{}, error) {
	return gophercloud.BuildRequestBody(opts, "consumer")
}

// UpdateConsumer updates an existing Consumer.
func UpdateConsumer(client *gophercloud.ServiceClient, id string, opts UpdateConsumerOpts) (r UpdateConsumerResult) {
	b, err := opts.ToOAuth1UpdateConsumerMap()
	if err != nil {
		r.Err = err
		return
	}
	resp, err := client.Patch(consumerURL(client, id), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// RequestTokenOptsBuilder allows extensions to add additional parameters to the
// RequestToken request.
type RequestTokenOptsBuilder interface {
	ToOAuth1RequestTokenHeaders(string, string) (map[string]string, error)
}

// RequestTokenOpts provides options used to get a consumer unauthorized
// request token.
type RequestTokenOpts struct {
	// OAuthConsumerKey is the OAuth1 Consumer Key.
	OAuthConsumerKey string `q:"oauth_consumer_key" required:"true"`

	// OAuthConsumerSecret is the OAuth1 Consumer Secret. Used to generate
	// an OAuth1 request signature.
	OAuthConsumerSecret string `required:"true"`

	// OAuthSignatureMethod is the OAuth1 signature method the Consumer used
	// to sign the request. Supported values are "HMAC-SHA1" or "PLAINTEXT".
	// "PLAINTEXT" is not recommended for production usage.
	OAuthSignatureMethod SignatureMethod `q:"oauth_signature_method" required:"true"`

	// OAuthTimestamp is an OAuth1 request timestamp. If nil, current Unix
	// timestamp will be used.
	OAuthTimestamp *time.Time

	// OAuthNonce is an OAuth1 request nonce. Nonce must be a random string,
	// uniquely generated for each request. Will be generated automatically
	// when it is not set.
	OAuthNonce string `q:"oauth_nonce"`

	// RequestedProjectID is a Project ID a consumer user requested an
	// access to.
	RequestedProjectID string `h:"Requested-Project-Id"`
}

// ToOAuth1RequestTokenHeaders formats a RequestTokenOpts into a map of request
// headers.
func (opts RequestTokenOpts) ToOAuth1RequestTokenHeaders(method, u string) (map[string]string, error) {
	q, err := buildOAuth1QueryString(opts, opts.OAuthTimestamp, "oob")
	if err != nil {
		return nil, err
	}

	h, err := gophercloud.BuildHeaders(opts)
	if err != nil {
		return nil, err
	}

	signatureKeys := []string{opts.OAuthConsumerSecret}
	stringToSign := buildStringToSign(method, u, q.Query())
	signature := url.QueryEscape(signString(opts.OAuthSignatureMethod, stringToSign, signatureKeys))
	authHeader := buildAuthHeader(q.Query(), signature)

	h["Authorization"] = authHeader

	return h, nil
}

// RequestToken requests an unauthorized OAuth1 Token.
func RequestToken(client *gophercloud.ServiceClient, opts RequestTokenOptsBuilder) (r TokenResult) {
	h, err := opts.ToOAuth1RequestTokenHeaders("POST", requestTokenURL(client))
	if err != nil {
		r.Err = err
		return
	}

	resp, err := client.Post(requestTokenURL(client), nil, nil, &gophercloud.RequestOpts{
		MoreHeaders:      h,
		OkCodes:          []int{201},
		KeepResponseBody: true,
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	if r.Err != nil {
		return
	}
	defer resp.Body.Close()
	if v := r.Header.Get("Content-Type"); v != OAuth1TokenContentType {
		r.Err = fmt.Errorf("unsupported Content-Type: %q", v)
		return
	}
	r.Body, r.Err = ioutil.ReadAll(resp.Body)
	return
}

// AuthorizeTokenOptsBuilder allows extensions to add additional parameters to
// the AuthorizeToken request.
type AuthorizeTokenOptsBuilder interface {
	ToOAuth1AuthorizeTokenMap() (map[string]interface{}, error)
}

// AuthorizeTokenOpts provides options used to authorize a request token.
type AuthorizeTokenOpts struct {
	Roles []Role `json:"roles"`
}

// Role is a struct representing a role object in a AuthorizeTokenOpts struct.
type Role struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ToOAuth1AuthorizeTokenMap formats an AuthorizeTokenOpts into an authorize token
// request.
func (opts AuthorizeTokenOpts) ToOAuth1AuthorizeTokenMap() (map[string]interface{}, error) {
	for _, r := range opts.Roles {
		if r == (Role{}) {
			return nil, fmt.Errorf("role must not be empty")
		}
	}
	return gophercloud.BuildRequestBody(opts, "")
}

// AuthorizeToken authorizes an unauthorized consumer token.
func AuthorizeToken(client *gophercloud.ServiceClient, id string, opts AuthorizeTokenOptsBuilder) (r AuthorizeTokenResult) {
	b, err := opts.ToOAuth1AuthorizeTokenMap()
	if err != nil {
		r.Err = err
		return
	}
	resp, err := client.Put(authorizeTokenURL(client, id), b, &r.Body, &gophercloud.RequestOpts{
		OkCodes: []int{200},
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// CreateAccessTokenOptsBuilder allows extensions to add additional parameters
// to the CreateAccessToken request.
type CreateAccessTokenOptsBuilder interface {
	ToOAuth1CreateAccessTokenHeaders(string, string) (map[string]string, error)
}

// CreateAccessTokenOpts provides options used to create an OAuth1 token.
type CreateAccessTokenOpts struct {
	// OAuthConsumerKey is the OAuth1 Consumer Key.
	OAuthConsumerKey string `q:"oauth_consumer_key" required:"true"`

	// OAuthConsumerSecret is the OAuth1 Consumer Secret. Used to generate
	// an OAuth1 request signature.
	OAuthConsumerSecret string `required:"true"`

	// OAuthToken is the OAuth1 Request Token.
	OAuthToken string `q:"oauth_token" required:"true"`

	// OAuthTokenSecret is the OAuth1 Request Token Secret. Used to generate
	// an OAuth1 request signature.
	OAuthTokenSecret string `required:"true"`

	// OAuthVerifier is the OAuth1 verification code.
	OAuthVerifier string `q:"oauth_verifier" required:"true"`

	// OAuthSignatureMethod is the OAuth1 signature method the Consumer used
	// to sign the request. Supported values are "HMAC-SHA1" or "PLAINTEXT".
	// "PLAINTEXT" is not recommended for production usage.
	OAuthSignatureMethod SignatureMethod `q:"oauth_signature_method" required:"true"`

	// OAuthTimestamp is an OAuth1 request timestamp. If nil, current Unix
	// timestamp will be used.
	OAuthTimestamp *time.Time

	// OAuthNonce is an OAuth1 request nonce. Nonce must be a random string,
	// uniquely generated for each request. Will be generated automatically
	// when it is not set.
	OAuthNonce string `q:"oauth_nonce"`
}

// ToOAuth1CreateAccessTokenHeaders formats a CreateAccessTokenOpts into a map of
// request headers.
func (opts CreateAccessTokenOpts) ToOAuth1CreateAccessTokenHeaders(method, u string) (map[string]string, error) {
	q, err := buildOAuth1QueryString(opts, opts.OAuthTimestamp, "")
	if err != nil {
		return nil, err
	}

	signatureKeys := []string{opts.OAuthConsumerSecret, opts.OAuthTokenSecret}
	stringToSign := buildStringToSign(method, u, q.Query())
	signature := url.QueryEscape(signString(opts.OAuthSignatureMethod, stringToSign, signatureKeys))
	authHeader := buildAuthHeader(q.Query(), signature)

	headers := map[string]string{
		"Authorization": authHeader,
	}

	return headers, nil
}

// CreateAccessToken creates a new OAuth1 Access Token
func CreateAccessToken(client *gophercloud.ServiceClient, opts CreateAccessTokenOptsBuilder) (r TokenResult) {
	h, err := opts.ToOAuth1CreateAccessTokenHeaders("POST", createAccessTokenURL(client))
	if err != nil {
		r.Err = err
		return
	}

	resp, err := client.Post(createAccessTokenURL(client), nil, nil, &gophercloud.RequestOpts{
		MoreHeaders:      h,
		OkCodes:          []int{201},
		KeepResponseBody: true,
	})
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	if r.Err != nil {
		return
	}
	defer resp.Body.Close()
	if v := r.Header.Get("Content-Type"); v != OAuth1TokenContentType {
		r.Err = fmt.Errorf("unsupported Content-Type: %q", v)
		return
	}
	r.Body, r.Err = ioutil.ReadAll(resp.Body)
	return
}

// GetAccessToken retrieves details on a single OAuth1 access token by an ID.
func GetAccessToken(client *gophercloud.ServiceClient, userID string, id string) (r GetAccessTokenResult) {
	resp, err := client.Get(userAccessTokenURL(client, userID, id), &r.Body, nil)
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// RevokeAccessToken revokes an OAuth1 access token.
func RevokeAccessToken(client *gophercloud.ServiceClient, userID string, id string) (r RevokeAccessTokenResult) {
	resp, err := client.Delete(userAccessTokenURL(client, userID, id), nil)
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// ListAccessTokens enumerates authorized access tokens.
func ListAccessTokens(client *gophercloud.ServiceClient, userID string) pagination.Pager {
	url := userAccessTokensURL(client, userID)
	return pagination.NewPager(client, url, func(r pagination.PageResult) pagination.Page {
		return AccessTokensPage{pagination.LinkedPageBase{PageResult: r}}
	})
}

// ListAccessTokenRoles enumerates authorized access token roles.
func ListAccessTokenRoles(client *gophercloud.ServiceClient, userID string, id string) pagination.Pager {
	url := userAccessTokenRolesURL(client, userID, id)
	return pagination.NewPager(client, url, func(r pagination.PageResult) pagination.Page {
		return AccessTokenRolesPage{pagination.LinkedPageBase{PageResult: r}}
	})
}

// GetAccessTokenRole retrieves details on a single OAuth1 access token role by
// an ID.
func GetAccessTokenRole(client *gophercloud.ServiceClient, userID string, id string, roleID string) (r GetAccessTokenRoleResult) {
	resp, err := client.Get(userAccessTokenRoleURL(client, userID, id, roleID), &r.Body, nil)
	_, r.Header, r.Err = gophercloud.ParseResponse(resp, err)
	return
}

// The following are small helper functions used to help build the signature.

// buildOAuth1QueryString builds a URLEncoded parameters string specific for
// OAuth1-based requests.
func buildOAuth1QueryString(opts interface{}, timestamp *time.Time, callback string) (*url.URL, error) {
	q, err := gophercloud.BuildQueryString(opts)
	if err != nil {
		return nil, err
	}

	query := q.Query()

	if timestamp != nil {
		// use provided timestamp
		query.Set("oauth_timestamp", strconv.FormatInt(timestamp.Unix(), 10))
	} else {
		// use current timestamp
		query.Set("oauth_timestamp", strconv.FormatInt(time.Now().UTC().Unix(), 10))
	}

	if query.Get("oauth_nonce") == "" {
		// when nonce is not set, generate a random one
		query.Set("oauth_nonce", strconv.FormatInt(rand.Int63(), 10)+query.Get("oauth_timestamp"))
	}

	if callback != "" {
		query.Set("oauth_callback", callback)
	}
	query.Set("oauth_version", "1.0")

	return &url.URL{RawQuery: query.Encode()}, nil
}

// buildStringToSign builds a string to be signed.
func buildStringToSign(method string, u string, query url.Values) []byte {
	parsedURL, _ := url.Parse(u)
	p := parsedURL.Port()
	s := parsedURL.Scheme

	// Default scheme port must be stripped
	if s == "http" && p == "80" || s == "https" && p == "443" {
		parsedURL.Host = strings.TrimSuffix(parsedURL.Host, ":"+p)
	}

	// Ensure that URL doesn't contain queries
	parsedURL.RawQuery = ""

	v := strings.Join(
		[]string{method, url.QueryEscape(parsedURL.String()), url.QueryEscape(query.Encode())}, "&")

	return []byte(v)
}

// signString signs a string using an OAuth1 signature method.
func signString(signatureMethod SignatureMethod, strToSign []byte, signatureKeys []string) string {
	var key []byte
	for i, k := range signatureKeys {
		key = append(key, []byte(url.QueryEscape(k))...)
		if i == 0 {
			key = append(key, '&')
		}
	}

	var signedString string
	switch signatureMethod {
	case PLAINTEXT:
		signedString = string(key)
	default:
		h := hmac.New(sha1.New, key)
		h.Write(strToSign)
		signedString = base64.StdEncoding.EncodeToString(h.Sum(nil))
	}

	return signedString
}

// buildAuthHeader generates an OAuth1 Authorization header with a signature
// calculated using an OAuth1 signature method.
func buildAuthHeader(query url.Values, signature string) string {
	var authHeader []string
	var keys []string
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		for _, v := range query[k] {
			authHeader = append(authHeader, fmt.Sprintf("%s=%q", k, url.QueryEscape(v)))
		}
	}

	authHeader = append(authHeader, fmt.Sprintf("oauth_signature=%q", signature))

	return "OAuth " + strings.Join(authHeader, ", ")
}
