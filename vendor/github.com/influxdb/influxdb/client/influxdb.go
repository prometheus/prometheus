package client

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/influxdb/influxdb/tsdb"
)

const (
	// DefaultTimeout is the default connection timeout used to connect to an InfluxDB instance
	DefaultTimeout = 0
)

// Config is used to specify what server to connect to.
// URL: The URL of the server connecting to.
// Username/Password are optional.  They will be passed via basic auth if provided.
// UserAgent: If not provided, will default "InfluxDBClient",
// Timeout: If not provided, will default to 0 (no timeout)
type Config struct {
	URL       url.URL
	Username  string
	Password  string
	UserAgent string
	Timeout   time.Duration
	Precision string
}

// NewConfig will create a config to be used in connecting to the client
func NewConfig() Config {
	return Config{
		Timeout: DefaultTimeout,
	}
}

// Client is used to make calls to the server.
type Client struct {
	url        url.URL
	username   string
	password   string
	httpClient *http.Client
	userAgent  string
	precision  string
}

const (
	ConsistencyOne    = "one"
	ConsistencyAll    = "all"
	ConsistencyQuorum = "quorum"
	ConsistencyAny    = "any"
)

// NewClient will instantiate and return a connected client to issue commands to the server.
func NewClient(c Config) (*Client, error) {
	client := Client{
		url:        c.URL,
		username:   c.Username,
		password:   c.Password,
		httpClient: &http.Client{Timeout: c.Timeout},
		userAgent:  c.UserAgent,
		precision:  c.Precision,
	}
	if client.userAgent == "" {
		client.userAgent = "InfluxDBClient"
	}
	return &client, nil
}

// Write takes BatchPoints and allows for writing of multiple points with defaults
// If successful, error is nil and Response is nil
// If an error occurs, Response may contain additional information if populated.
func (c *Client) Write(bp BatchPoints) (*Response, error) {
	u := c.url
	u.Path = "write"

	var b bytes.Buffer
	for _, p := range bp.Points {
		if p.Raw != "" {
			if _, err := b.WriteString(p.Raw); err != nil {
				return nil, err
			}
		} else {
			for k, v := range bp.Tags {
				if p.Tags == nil {
					p.Tags = make(map[string]string, len(bp.Tags))
				}
				p.Tags[k] = v
			}

			if _, err := b.WriteString(p.MarshalString()); err != nil {
				return nil, err
			}
		}

		if err := b.WriteByte('\n'); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest("POST", u.String(), &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "")
	req.Header.Set("User-Agent", c.userAgent)
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	params := req.URL.Query()
	params.Set("db", bp.Database)
	params.Set("rp", bp.RetentionPolicy)
	params.Set("precision", bp.Precision)
	params.Set("consistency", bp.WriteConsistency)
	req.URL.RawQuery = params.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var response Response
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		var err = fmt.Errorf(string(body))
		response.Err = err
		return &response, err
	}

	return nil, nil
}

// Structs

// Response represents a list of statement results.
type Response struct {
	Err error
}

// Point defines the fields that will be written to the database
// Measurement, Time, and Fields are required
// Precision can be specified if the time is in epoch format (integer).
// Valid values for Precision are n, u, ms, s, m, and h
type Point struct {
	Measurement string
	Tags        map[string]string
	Time        time.Time
	Fields      map[string]interface{}
	Precision   string
	Raw         string
}

func (p *Point) MarshalString() string {
	return tsdb.NewPoint(p.Measurement, p.Tags, p.Fields, p.Time).String()
}

// BatchPoints is used to send batched data in a single write.
// Database and Points are required
// If no retention policy is specified, it will use the databases default retention policy.
// If tags are specified, they will be "merged" with all points.  If a point already has that tag, it is ignored.
// If time is specified, it will be applied to any point with an empty time.
// Precision can be specified if the time is in epoch format (integer).
// Valid values for Precision are n, u, ms, s, m, and h
type BatchPoints struct {
	Points           []Point           `json:"points,omitempty"`
	Database         string            `json:"database,omitempty"`
	RetentionPolicy  string            `json:"retentionPolicy,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	Time             time.Time         `json:"time,omitempty"`
	Precision        string            `json:"precision,omitempty"`
	WriteConsistency string            `json:"-"`
}
