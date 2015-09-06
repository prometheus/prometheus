package kafka

import (
	"encoding/json"
	"math"
	"net/url"
	"time"

	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
)

type Encoding interface {
	Name() string

	Encode(v interface{}) ([]byte, error)

	Decode(data []byte, v interface{}) error
}

const (
	ENCODING_PB   = "protobuf"
	ENCODING_JSON = "json"
	ENCODING_YAML = "yaml"
)

type ProtobufEncoding struct{}

func (e *ProtobufEncoding) Name() string { return ENCODING_PB }
func (e *ProtobufEncoding) Encode(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}
func (e *ProtobufEncoding) Decode(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}

type JsonEncoding struct{}

func (e *JsonEncoding) Name() string                            { return ENCODING_JSON }
func (e *JsonEncoding) Encode(v interface{}) ([]byte, error)    { return json.Marshal(v) }
func (e *JsonEncoding) Decode(data []byte, v interface{}) error { return json.Unmarshal(data, v) }

type YamlEncoding struct{}

func (e *YamlEncoding) Name() string                            { return ENCODING_YAML }
func (e *YamlEncoding) Encode(v interface{}) ([]byte, error)    { return yaml.Marshal(v) }
func (e *YamlEncoding) Decode(data []byte, v interface{}) error { return yaml.Unmarshal(data, v) }

type Client struct {
	*sarama.Config

	uri      *url.URL
	encoding Encoding
	client   sarama.Client
	producer sarama.AsyncProducer
}

func NewClient(uri string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	cfg := sarama.NewConfig()
	cfg.Producer.Timeout = timeout

	var encoding Encoding

	switch u.Query().Get("format") {
	case ENCODING_JSON:
		encoding = &JsonEncoding{}
	case ENCODING_YAML:
		encoding = &YamlEncoding{}
	default:
		encoding = &ProtobufEncoding{}
	}

	switch u.Query().Get("compression") {
	case "gzip":
		cfg.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		cfg.Producer.Compression = sarama.CompressionSnappy
	default:
		cfg.Producer.Compression = sarama.CompressionNone
	}

	return &Client{
		Config:   cfg,
		uri:      u,
		encoding: encoding,
	}, nil
}

func (c *Client) Name() string {
	return "kafka"
}

// tagsFromMetric translates Prometheus metric into Kakfa/ProtoBuf tags.
func tagsFromMetric(m model.Metric) []*Sample_Tag {
	tags := make([]*Sample_Tag, 0, len(m)-1)

	for l, v := range m {
		if l == model.MetricNameLabel {
			continue
		}

		tags = append(tags, &Sample_Tag{
			Name:  proto.String(string(l)),
			Value: proto.String(string(v)),
		})
	}

	return tags
}

func (c *Client) Store(samples model.Samples) error {
	if err := c.Validate(); err != nil {
		return err
	}

	msgs := make([]*sarama.ProducerMessage, 0, len(samples))

	for _, s := range samples {
		v := float64(s.Value)

		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.Warnf("cannot send value %f to Kafka, skipping sample %#v", v, s)
			continue
		}

		data, err := c.encoding.Encode(&Sample{
			Value:     proto.Float64(v),
			Timestamp: proto.Int64(s.Timestamp.Unix()),
			Tags:      tagsFromMetric(s.Metric),
		})

		if err != nil {
			return err
		}

		msgs = append(msgs, &sarama.ProducerMessage{
			Topic: c.uri.Path,
			Key:   sarama.StringEncoder(s.Metric[model.MetricNameLabel]),
			Value: sarama.ByteEncoder(data),
		})
	}

	if c.producer == nil {
		if c.client == nil {
			if client, err := sarama.NewClient([]string{c.uri.Host}, c.Config); err != nil {
				return err
			} else {
				c.client = client
			}
		}

		if producer, err := sarama.NewAsyncProducerFromClient(c.client); err != nil {
			return err
		} else {
			c.producer = producer
		}
	}

	var errors sarama.ProducerErrors

	for _, msg := range msgs {
		log.Infof("sending: %s", msg)

		select {
		case c.producer.Input() <- msg:
		case perr := <-c.producer.Errors():
			log.Warnf("received: %s", perr)

			errors = append(errors, perr)

			break
		}
	}

	if perrs := c.producer.Close(); perrs != nil {
		errors = append(errors, perrs.(sarama.ProducerErrors)...)
	}

	c.producer = nil

	if len(errors) > 0 {
		return errors
	}

	return nil
}
