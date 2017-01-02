// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kafka

import (
	"encoding/json"
	"github.com/Shopify/sarama"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"time"
)

// Client allows sending batches of Prometheus samples to Kafka.
type Client struct {
	client       sarama.AsyncProducer
	defaultTopic string
}

// NewClient creates a new Client.
func NewClient(brokerList string, newTopic string) *Client {

	return &Client{
		client:       newAccessLogProducer(brokerList),
		defaultTopic: newTopic,
	}
}

func newAccessLogProducer(brokerList string) sarama.AsyncProducer {
	config := sarama.NewConfig()
	config.Net.TLS.Enable = false
	config.Producer.RequiredAcks = sarama.WaitForLocal     // Only wait for the leader to ack
	config.Producer.Compression = sarama.CompressionSnappy // Compress messages
	config.Producer.Flush.Frequency = 1 * time.Millisecond // Flush batches every 500ms
	var a []string
	a = append(a, brokerList)
	producer, err := sarama.NewAsyncProducer(a, config)
	if err != nil {
		log.Fatalln("Failed to start Sarama producer:", err)
	}

	// We will just log to STDOUT if we're not able to produce messages.
	// Note: messages will only be returned here after all retry attempts are exhausted.
	go func() {
		for err := range producer.Errors() {
			log.Debugln("Failed to write access log entry:", err)
		}
	}()

	return producer
}

// Store sends a batch of samples to Kafka via its go client
func (c *Client) Store(samples model.Samples) error {

	for _, s := range samples {
		b, err := json.Marshal(&s)
		if err != nil {
			return err
		}
		c.client.Input() <- &sarama.ProducerMessage{
			Topic: c.defaultTopic,
			Key:   sarama.StringEncoder(s.Metric[model.MetricNameLabel]),
			Value: sarama.StringEncoder(b),
		}
	}

	return nil
}

// Name identifies the client as an Kafka client.
func (c Client) Name() string {
	return "kafka"
}
