package kafka

import (
	"encoding/json"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Shopify/sarama"
	"github.com/golang/protobuf/proto"
	"gopkg.in/yaml.v2"

	dto "github.com/prometheus/client_model/go"
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

	Topic string

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

	topic := u.Query().Get("topic")

	if topic == "" {
		topic = strings.Trim(strings.Replace(u.Path, "/", ".", -1), ". ")

		if topic == "" {
			if hostname, err := os.Hostname(); err != nil {
				return nil, err
			} else {
				topic = hostname
			}
		}
	}

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
		Topic:    topic,
		uri:      u,
		encoding: encoding,
	}, nil
}

func (c *Client) Name() string {
	return "kafka"
}

// tagsFromMetric translates Prometheus metric into Kakfa/ProtoBuf tags.
func labelsFromMetric(m model.Metric) []*dto.LabelPair {
	labels := make([]*dto.LabelPair, 0, len(m)-1)

	for l, v := range m {
		if l == model.MetricNameLabel {
			continue
		}

		labels = append(labels, &dto.LabelPair{
			Name:  proto.String(string(l)),
			Value: proto.String(string(v)),
		})
	}

	return labels
}

func (c *Client) Store(samples model.Samples) error {
	if err := c.Validate(); err != nil {
		return err
	}

	msgs := make([]*sarama.ProducerMessage, 0, len(samples))

	for _, s := range samples {
		v := float64(s.Value)

		if math.IsNaN(v) || math.IsInf(v, 0) {
			log.Debugf("cannot send value %f to Kafka, skipping sample %#v", v, s)
			continue
		}

		data, err := c.encoding.Encode(&dto.Metric{
			Label: labelsFromMetric(s.Metric),
			Untyped: &dto.Untyped{
				Value: proto.Float64(v),
			},
			TimestampMs: proto.Int64(int64(s.Timestamp)),
		})

		if err != nil {
			return err
		}

		msgs = append(msgs, &sarama.ProducerMessage{
			Topic: c.Topic,
			Key:   sarama.StringEncoder(s.Metric[model.MetricNameLabel]),
			Value: sarama.ByteEncoder(data),
		})
	}

	producer := c.producer

	if producer == nil {
		if c.client == nil {
			if client, err := sarama.NewClient([]string{c.uri.Host}, c.Config); err != nil {
				return err
			} else {
				c.client = client
			}
		}

		if p, err := sarama.NewAsyncProducerFromClient(c.client); err != nil {
			return err
		} else {
			producer = p
		}
	}

	var errors sarama.ProducerErrors

	for _, msg := range msgs {
		log.Infof("sending: %v", msg)

		select {
		case producer.Input() <- msg:
		case perr := <-producer.Errors():
			log.Warnf("received: %v", perr)

			errors = append(errors, perr)

			break
		}
	}

	if c.producer == nil {
		if perrs := producer.Close(); perrs != nil {
			errors = append(errors, perrs.(sarama.ProducerErrors)...)
		}
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}
