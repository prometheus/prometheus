package ext_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
)

func TestPeerTags(t *testing.T) {
	if ext.PeerService != "peer.service" {
		t.Fatalf("Invalid PeerService %v", ext.PeerService)
	}
	tracer := mocktracer.New()
	span := tracer.StartSpan("my-trace")
	ext.PeerService.Set(span, "my-service")
	ext.PeerAddress.Set(span, "my-hostname:8080")
	ext.PeerHostname.Set(span, "my-hostname")
	ext.PeerHostIPv4.Set(span, uint32(127<<24|1))
	ext.PeerHostIPv6.Set(span, "::")
	ext.PeerPort.Set(span, uint16(8080))
	ext.SamplingPriority.Set(span, uint16(1))
	ext.SpanKind.Set(span, ext.SpanKindRPCServerEnum)
	ext.SpanKindRPCClient.Set(span)
	span.Finish()

	rawSpan := tracer.FinishedSpans()[0]
	assert.Equal(t, map[string]interface{}{
		"peer.service":  "my-service",
		"peer.address":  "my-hostname:8080",
		"peer.hostname": "my-hostname",
		"peer.ipv4":     uint32(127<<24 | 1),
		"peer.ipv6":     "::",
		"peer.port":     uint16(8080),
		"span.kind":     ext.SpanKindRPCClientEnum,
	}, rawSpan.Tags())
	assert.True(t, span.Context().(mocktracer.MockSpanContext).Sampled)
	ext.SamplingPriority.Set(span, uint16(0))
	assert.False(t, span.Context().(mocktracer.MockSpanContext).Sampled)
}

func TestHTTPTags(t *testing.T) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("my-trace", ext.SpanKindRPCServer)
	ext.HTTPUrl.Set(span, "test.biz/uri?protocol=false")
	ext.HTTPMethod.Set(span, "GET")
	ext.HTTPStatusCode.Set(span, 301)
	span.Finish()

	rawSpan := tracer.FinishedSpans()[0]
	assert.Equal(t, map[string]interface{}{
		"http.url":         "test.biz/uri?protocol=false",
		"http.method":      "GET",
		"http.status_code": uint16(301),
		"span.kind":        ext.SpanKindRPCServerEnum,
	}, rawSpan.Tags())
}

func TestDBTags(t *testing.T) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("my-trace", ext.SpanKindRPCClient)
	ext.DBInstance.Set(span, "127.0.0.1:3306/customers")
	ext.DBStatement.Set(span, "SELECT * FROM user_table")
	ext.DBType.Set(span, "sql")
	ext.DBUser.Set(span, "customer_user")
	span.Finish()

	rawSpan := tracer.FinishedSpans()[0]
	assert.Equal(t, map[string]interface{}{
		"db.instance":  "127.0.0.1:3306/customers",
		"db.statement": "SELECT * FROM user_table",
		"db.type":      "sql",
		"db.user":      "customer_user",
		"span.kind":    ext.SpanKindRPCClientEnum,
	}, rawSpan.Tags())
}

func TestMiscTags(t *testing.T) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("my-trace")
	ext.Component.Set(span, "my-awesome-library")
	ext.SamplingPriority.Set(span, 1)
	ext.Error.Set(span, true)

	span.Finish()

	rawSpan := tracer.FinishedSpans()[0]
	assert.Equal(t, map[string]interface{}{
		"component": "my-awesome-library",
		"error":     true,
	}, rawSpan.Tags())
}

func TestRPCServerOption(t *testing.T) {
	tracer := mocktracer.New()
	parent := tracer.StartSpan("my-trace")
	parent.SetBaggageItem("bag", "gage")

	carrier := opentracing.HTTPHeadersCarrier{}
	err := tracer.Inject(parent.Context(), opentracing.HTTPHeaders, carrier)
	if err != nil {
		t.Fatal(err)
	}

	parCtx, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
	if err != nil {
		t.Fatal(err)
	}

	tracer.StartSpan("my-child", ext.RPCServerOption(parCtx)).Finish()

	rawSpan := tracer.FinishedSpans()[0]
	assert.Equal(t, map[string]interface{}{
		"span.kind": ext.SpanKindRPCServerEnum,
	}, rawSpan.Tags())
	assert.Equal(t, map[string]string{
		"bag": "gage",
	}, rawSpan.Context().(mocktracer.MockSpanContext).Baggage)
}

func TestMessageBusProducerTags(t *testing.T) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("my-trace", ext.SpanKindProducer)
	ext.MessageBusDestination.Set(span, "topic name")
	span.Finish()

	rawSpan := tracer.FinishedSpans()[0]
	assert.Equal(t, map[string]interface{}{
		"message_bus.destination": "topic name",
		"span.kind":               ext.SpanKindProducerEnum,
	}, rawSpan.Tags())
}

func TestMessageBusConsumerTags(t *testing.T) {
	tracer := mocktracer.New()
	span := tracer.StartSpan("my-trace", ext.SpanKindConsumer)
	ext.MessageBusDestination.Set(span, "topic name")
	span.Finish()

	rawSpan := tracer.FinishedSpans()[0]
	assert.Equal(t, map[string]interface{}{
		"message_bus.destination": "topic name",
		"span.kind":               ext.SpanKindConsumerEnum,
	}, rawSpan.Tags())
}
