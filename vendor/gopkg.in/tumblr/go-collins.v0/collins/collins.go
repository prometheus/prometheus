package collins

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

const (
	maxHTTPCode = 299
)

var (
	VERSION = "0.1.0"
)

// Client represents a client connection to a collins server. Requests to the
// various APIs are done by calling functions on the various services.
type Client struct {
	client   *http.Client
	BaseURL  *url.URL
	User     string
	Password string

	Assets     *AssetService
	AssetTypes *AssetTypeService
	Logs       *LogService
	States     *StateService
	Tags       *TagService
	Management *ManagementService
	IPAM       *IPAMService
	Firehose   *FirehoseService
}

// Error represents an error returned from collins. Collins returns
// errors in JSON format, which we marshal in to this struct.
type Error struct {
	Status string `json:"status"`
	Data   struct {
		Message string `json:"message"`
	} `json:"data"`
}

// Container is used to deserialize the JSON reponse from the API.
type Container struct {
	CollinsStatus string      `json:"status"`
	Data          interface{} `json:"data"`
}

// Response is our custom response type. It has the HTTP response embedded for
// debugging purposes. It also has embedded the `container` that the JSON
// response gets decoded into (if the caller to `Do` passes in a struct
// to decode into). Finally it contains all necessary data for pagination.
type Response struct {
	*http.Response

	*Container

	PreviousPage int
	CurrentPage  int
	NextPage     int
	TotalResults int
}

// PageOpts allows the caller to specify pagination options. Since Collins takes
// in pagination options via URL parameters we can use google/go-querystring to
// describe our pagination opts as structs. This also allows embedding of
// pagination options directly into other request option structs.
type PageOpts struct {
	Page int    `url:"page,omitempty"`
	Size int    `url:"size,omitempty"`
	Sort string `url:"sort,omitempty"`
}

// PaginationResponse is used to represent the pagination information coming
// back from the collins server.
type PaginationResponse struct {
	PreviousPage int `json:"PreviousPage"`
	CurrentPage  int `json:"CurrentPage"`
	NextPage     int `json:"NextPage"`
	TotalResults int `json:"TotalResults"`
}

func (e *Error) Error() string {
	return e.Data.Message
}

// NewClient creates a Client struct and returns a point to it. This client is
// then used to query the various APIs collins provides.
func NewClient(username, password, baseurl string) (*Client, error) {
	u, err := url.Parse(baseurl)
	if err != nil {
		return nil, err
	}

	c := &Client{
		client:   &http.Client{},
		User:     username,
		Password: password,
		BaseURL:  u,
	}

	c.Assets = &AssetService{client: c}
	c.AssetTypes = &AssetTypeService{client: c}
	c.Logs = &LogService{client: c}
	c.States = &StateService{client: c}
	c.Tags = &TagService{client: c}
	c.Management = &ManagementService{client: c}
	c.IPAM = &IPAMService{client: c}
	c.Firehose = &FirehoseService{client: c}

	return c, nil
}

// NewClientFromYaml sets up a new Client, but reads the credentials and host
// from a yaml file on disk. The following paths are searched:
//
// * Path in COLLINS_CLIENT_CONFIG environment variable
// * ~/.collins.yml
// * /etc/collins.yml
// * /var/db/collins.yml
func NewClientFromYaml() (*Client, error) {
	yamlPaths := []string{
		os.Getenv("COLLINS_CLIENT_CONFIG"),
		path.Join(os.Getenv("HOME"), ".collins.yml"),
		"/etc/collins.yml",
		"/var/db/collins.yml",
	}
	return NewClientFromFiles(yamlPaths...)
}

// NewClientFromFiles takes an array of paths to look for credentials, and
// returns a Client based on the first config file that exists and parses
// correctly. Otherwise, it returns nil and an error.
func NewClientFromFiles(paths ...string) (*Client, error) {
	f, err := openYamlFiles(paths...)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var creds struct {
		Host     string
		Username string
		Password string
	}

	err = yaml.Unmarshal(data, &creds)
	if err != nil {
		return nil, err
	}

	return NewClient(creds.Username, creds.Password, creds.Host)
}

func openYamlFiles(paths ...string) (io.Reader, error) {
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		} else {
			return f, nil
		}
	}

	errStr := fmt.Sprintf("Could not load collins credentials from file. (Searched: %s)", strings.Join(paths, ", "))
	return nil, errors.New(errStr)
}

// NewRequest creates a new HTTP request which can then be performed by Do.
func (c *Client) NewRequest(method, path string) (*http.Request, error) {
	rel, err := url.Parse(path)
	if err != nil {
		return nil, err
	}

	reqURL := c.BaseURL.ResolveReference(rel)
	req, err := http.NewRequest(method, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.User, c.Password)
	req.Header.Set("User-Agent", "go-collins "+VERSION)
	req.Header.Set("Accept", "application/json")

	return req, nil
}

// Create our custom response object that we will pass back to caller
func newResponse(r *http.Response) *Response {
	resp := &Response{Response: r}
	resp.populatePagination()
	return resp
}

// Read in data from headers and use that to populate our response struct
func (r *Response) populatePagination() {
	h := r.Header
	if prev := h.Get("X-Pagination-PreviousPage"); prev != "" {
		n, _ := strconv.Atoi(prev)
		r.PreviousPage = n
	}

	if cur := h.Get("X-Pagination-CurrentPage"); cur != "" {
		n, _ := strconv.Atoi(cur)
		r.CurrentPage = n
	}

	if next := h.Get("X-Pagination-NextPage"); next != "" {
		n, _ := strconv.Atoi(next)
		r.NextPage = n
	}

	if total := h.Get("X-Pagination-TotalResults"); total != "" {
		n, _ := strconv.Atoi(total)
		r.TotalResults = n
	}
}

// Do performs a given request that was built with `NewRequest`. We return the
// response object as well so that callers can have access to pagination info.
func (c *Client) Do(req *http.Request, v interface{}) (*Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	response := newResponse(resp)

	if resp.StatusCode > maxHTTPCode {
		collinsError := new(Error)
		if strings.Contains(resp.Header.Get("Content-Type"), "application/json;") {
			err = json.NewDecoder(resp.Body).Decode(collinsError)
			if err != nil {
				return response, err
			}
		} else if strings.Contains(resp.Header.Get("Content-Type"), "text/plain;") {
			errbuf := &bytes.Buffer{}
			bufio.NewReader(resp.Body).WriteTo(errbuf)
			collinsError.Data.Message = errbuf.String()
		} else {
			errstr := fmt.Sprintf("Response with unexpected Content-Type - `%s' received.", resp.Header.Get("Content-Type"))
			return response, errors.New(errstr)
		}
		collinsError.Data.Message = resp.Status + " returned from collins: " + collinsError.Data.Message
		return response, collinsError
	}

	// This looks kind of weird but it works. This allows callers that pass in
	// an interface to have response JSON decoded into the interface they pass.
	// It also allows accessing `response.container.Status` etc. to get helpful
	// response info from Collins.
	if v != nil {
		response.Container = &Container{
			Data: v,
		}
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "application/json;") {
		err = json.NewDecoder(resp.Body).Decode(response)
		if err != nil {
			return response, err
		}
	} else {
		errstr := fmt.Sprintf("Response with unexpected Content-Type - `%s' received. Erroring out.", resp.Header.Get("Content-Type"))
		return response, errors.New(errstr)
	}

	return response, nil
}
