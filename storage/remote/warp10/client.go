package warp10

import (
	"bytes"
	"github.com/prometheus/common/model"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
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
		client : &http.Client{
			Timeout: time.Second * 2,
		},
	}
}

func (c *Client) Store(samples model.Samples) error {
	buffer := &bytes.Buffer{}
	for _, e := range samples {
		buffer.WriteString(strconv.FormatInt(int64(e.Timestamp)*1000, 10))
		buffer.WriteString("// ")
		buffer.WriteString(string(e.Metric[model.MetricNameLabel]))
		buffer.WriteString("{")
		i := 0
		for l, v := range e.Metric {
			if l != model.MetricNameLabel {

				buffer.WriteString(string(l))
				buffer.WriteString("=")
				buffer.WriteString(string(v))
				if i != 0 {
					buffer.WriteString(",")
				}
				i++
			}
		}
		buffer.WriteString("} ")
		buffer.WriteString(strconv.FormatFloat(float64(e.Value), 'f', -1, 64))
		buffer.WriteString("\n")
	}

	req, _ := http.NewRequest("POST", c.server, buffer)
	req.Header.Add("X-Warp10-Token", c.writeToken)
	req.Header.Add("User-Agent", "Prometheus remote v "+Version)
	resp, err := c.client.Do(req)
	if err != nil {
		log.Println("Cannot send metrics to warp10")
	} else {
		defer resp.Body.Close()
		ioutil.ReadAll(resp.Body)
	}
	return nil
}

func (c Client) Name() string {
	return "warp10"
}
