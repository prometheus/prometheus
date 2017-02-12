package warp10

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

const Version string = "0.1.0"

type Client struct {
	writeToken string
	server     string
	client     *http.Client
}

func NewClient(server, writeToken string) *Client {
	return &Client{
		server:     server,
		writeToken: writeToken,
		client: &http.Client{
			Timeout: time.Second * 5,
		},
	}
}

func (c *Client) Store(samples model.Samples) error {
	buffer := &bytes.Buffer{}
	for _, e := range samples {
		fmt.Fprintf(buffer, "%d// %s{", int64(e.Timestamp)*1000, url.QueryEscape(string(e.Metric[model.MetricNameLabel])))
		i := 0
		for l, v := range e.Metric {
			if l != model.MetricNameLabel {
				fmt.Fprintf(buffer, "%s=%s", url.QueryEscape(string(l)), url.QueryEscape(string(v)))
				if i != 0 {
					buffer.WriteRune(',')
				}
				i++
			}
		}
		fmt.Fprintf(buffer, "} %f\n", float64(e.Value))
	}
	req, err := http.NewRequest("POST", c.server, buffer)
	if err != nil {
		log.Errorf("Cannot create request to %s", c.server)
		return err
	}
	req.Header.Add("X-Warp10-Token", c.writeToken)
	req.Header.Add("User-Agent", "Prometheus remote "+Version)
	resp, err := c.client.Do(req)
	if err != nil {
		log.Errorf("Cannot send metrics to warp10 %s", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		content, _ := ioutil.ReadAll(resp.Body)
		log.Errorf("%s", content)
		return errors.New("Warp10 ingress errors")
	}
	return nil
}

func (c Client) Name() string {
	return "warp10"
}
