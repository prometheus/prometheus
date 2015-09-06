package kafka

import (
	"math"
	"testing"
	"time"

	"github.com/Shopify/sarama/mocks"

	"github.com/prometheus/common/model"
)

var (
	samples = model.Samples{
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"test_label":          "test_label_value1",
			},
			Timestamp: model.Time(123456789123),
			Value:     1.23,
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "testmetric",
				"test_label":          "test_label_value2",
			},
			Timestamp: model.Time(123456789123),
			Value:     5.1234,
		},
		{
			Metric: model.Metric{
				model.MetricNameLabel: "special_float_value",
			},
			Timestamp: model.Time(123456789123),
			Value:     model.SampleValue(math.NaN()),
		},
	}
)

func doTestClient(t *testing.T, format string) {
	c, err := NewClient("kafka://localhost/topic?format="+format, time.Minute)
	c.Producer.Return.Successes = true

	if err != nil {
		t.Fatalf("Error creating client: %s", err)
	}

	producer := mocks.NewAsyncProducer(t, c.Config)

	for i := 0; i < len(samples)-1; i++ {
		producer.ExpectInputAndSucceed()
	}

	c.producer = producer

	go func() {
		if err := c.Store(samples); err != nil {
			t.Fatalf("Error sending samples: %s", err)
		}

		if err := c.producer.Close(); err != nil {
			t.Fatalf("Error closing producer: %s", err)
		}
	}()

	var msgs []*Sample

	for msg := range c.producer.Successes() {
		var sample Sample

		if data, err := msg.Value.Encode(); err != nil {
			t.Fatalf("Error encoding value: %s", err)
		} else if c.encoding.Decode(data, &sample); err != nil {
			t.Fatalf("Error decoding value: %s", err)
		} else {
			msgs = append(msgs, &sample)
		}
	}

	if len(msgs) != len(samples)-1 {
		t.Fatalf("Error missed %d messages", len(samples)-len(msgs)-1)
	}
}

func TestClientProtobuf(t *testing.T) {
	doTestClient(t, ENCODING_PB)
}

func TestClientJson(t *testing.T) {
	doTestClient(t, ENCODING_JSON)
}

func TestClientYaml(t *testing.T) {
	doTestClient(t, ENCODING_YAML)
}
