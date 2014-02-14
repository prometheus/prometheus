package opentsdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

const (
	putEndpoint     = "/api/put"
	contentTypeJSON = "application/json"
)

var (
	illegalCharsRE = regexp.MustCompile(`[^a-zA-Z0-9_\-./]`)
)

// Client allows sending batches of Prometheus samples to OpenTSDB.
type Client struct {
	url        string
	httpClient *http.Client
}

// NewClient creates a new Client.
func NewClient(url string, timeout time.Duration) *Client {
	return &Client{
		url:        url,
		httpClient: utility.NewDeadlineClient(timeout),
	}
}

// StoreSamplesRequest is used for building a JSON request for storing samples
// via the OpenTSDB.
type StoreSamplesRequest struct {
	Metric    string                  `json:"metric"`
	Timestamp int64                   `json:"timestamp"`
	Value     clientmodel.SampleValue `json:"value"`
	Tags      map[string]string       `json:"tags"`
}

// escapeTagValue escapes Prometheus label values to valid tag values for
// OpenTSDB.
func escapeTagValue(l clientmodel.LabelValue) string {
	return illegalCharsRE.ReplaceAllString(string(l), "_")
}

// tagsFromMetric translates Prometheus metric into OpenTSDB tags.
func tagsFromMetric(m clientmodel.Metric) map[string]string {
	tags := make(map[string]string, len(m)-1)
	for l, v := range m {
		if l == clientmodel.MetricNameLabel {
			continue
		}
		tags[string(l)] = escapeTagValue(v)
	}
	return tags
}

// Store sends a batch of samples to OpenTSDB via its HTTP API.
func (c *Client) Store(samples clientmodel.Samples) error {
	reqs := make([]StoreSamplesRequest, 0, len(samples))
	for _, s := range samples {
		metric := escapeTagValue(s.Metric[clientmodel.MetricNameLabel])
		reqs = append(reqs, StoreSamplesRequest{
			Metric:    metric,
			Timestamp: s.Timestamp.Unix(),
			Value:     s.Value,
			Tags:      tagsFromMetric(s.Metric),
		})
	}

	u, err := url.Parse(c.url)
	if err != nil {
		return err
	}

	u.Path = putEndpoint

	buf, err := json.Marshal(reqs)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(
		u.String(),
		contentTypeJSON,
		bytes.NewBuffer(buf),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// API returns status code 204 for successful writes.
	// http://opentsdb.net/docs/build/html/api_http/put.html
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	// API returns status code 400 on error, encoding error details in the
	// response content in JSON.
	buf, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var r map[string]int
	if err := json.Unmarshal(buf, &r); err != nil {
		return err
	}
	return fmt.Errorf("failed to write %d samples to OpenTSDB, %d succeeded", r["failed"], r["success"])
}
