package ext

import opentracing "github.com/opentracing/opentracing-go"

// These constants define common tag names recommended for better portability across
// tracing systems and languages/platforms.
//
// The tag names are defined as typed strings, so that in addition to the usual use
//
//     span.setTag(TagName, value)
//
// they also support value type validation via this additional syntax:
//
//    TagName.Set(span, value)
//
var (
	//////////////////////////////////////////////////////////////////////
	// SpanKind (client/server)
	//////////////////////////////////////////////////////////////////////

	// SpanKind hints at relationship between spans, e.g. client/server
	SpanKind = spanKindTagName("span.kind")

	// SpanKindRPCClient marks a span representing the client-side of an RPC
	// or other remote call
	SpanKindRPCClientEnum = SpanKindEnum("client")
	SpanKindRPCClient     = opentracing.Tag{Key: string(SpanKind), Value: SpanKindRPCClientEnum}

	// SpanKindRPCServer marks a span representing the server-side of an RPC
	// or other remote call
	SpanKindRPCServerEnum = SpanKindEnum("server")
	SpanKindRPCServer     = opentracing.Tag{Key: string(SpanKind), Value: SpanKindRPCServerEnum}

	//////////////////////////////////////////////////////////////////////
	// Component name
	//////////////////////////////////////////////////////////////////////

	// Component is a low-cardinality identifier of the module, library,
	// or package that is generating a span.
	Component = stringTagName("component")

	//////////////////////////////////////////////////////////////////////
	// Sampling hint
	//////////////////////////////////////////////////////////////////////

	// SamplingPriority determines the priority of sampling this Span.
	SamplingPriority = uint16TagName("sampling.priority")

	//////////////////////////////////////////////////////////////////////
	// Peer tags. These tags can be emitted by either client-side of
	// server-side to describe the other side/service in a peer-to-peer
	// communications, like an RPC call.
	//////////////////////////////////////////////////////////////////////

	// PeerService records the service name of the peer
	PeerService = stringTagName("peer.service")

	// PeerHostname records the host name of the peer
	PeerHostname = stringTagName("peer.hostname")

	// PeerHostIPv4 records IP v4 host address of the peer
	PeerHostIPv4 = uint32TagName("peer.ipv4")

	// PeerHostIPv6 records IP v6 host address of the peer
	PeerHostIPv6 = stringTagName("peer.ipv6")

	// PeerPort records port number of the peer
	PeerPort = uint16TagName("peer.port")

	//////////////////////////////////////////////////////////////////////
	// HTTP Tags
	//////////////////////////////////////////////////////////////////////

	// HTTPUrl should be the URL of the request being handled in this segment
	// of the trace, in standard URI format. The protocol is optional.
	HTTPUrl = stringTagName("http.url")

	// HTTPMethod is the HTTP method of the request, and is case-insensitive.
	HTTPMethod = stringTagName("http.method")

	// HTTPStatusCode is the numeric HTTP status code (200, 404, etc) of the
	// HTTP response.
	HTTPStatusCode = uint16TagName("http.status_code")

	//////////////////////////////////////////////////////////////////////
	// Error Tag
	//////////////////////////////////////////////////////////////////////

	// Error indicates that operation represented by the span resulted in an error.
	Error = boolTagName("error")
)

// ---

// SpanKindEnum represents common span types
type SpanKindEnum string

type spanKindTagName string

// Set adds a string tag to the `span`
func (tag spanKindTagName) Set(span opentracing.Span, value SpanKindEnum) {
	span.SetTag(string(tag), value)
}

type rpcServerOption struct {
	clientContext opentracing.SpanContext
}

func (r rpcServerOption) Apply(o *opentracing.StartSpanOptions) {
	if r.clientContext != nil {
		opentracing.ChildOf(r.clientContext).Apply(o)
	}
	SpanKindRPCServer.Apply(o)
}

// RPCServerOption returns a StartSpanOption appropriate for an RPC server span
// with `client` representing the metadata for the remote peer Span if available.
// In case client == nil, due to the client not being instrumented, this RPC
// server span will be a root span.
func RPCServerOption(client opentracing.SpanContext) opentracing.StartSpanOption {
	return rpcServerOption{client}
}

// ---

type stringTagName string

// Set adds a string tag to the `span`
func (tag stringTagName) Set(span opentracing.Span, value string) {
	span.SetTag(string(tag), value)
}

// ---

type uint32TagName string

// Set adds a uint32 tag to the `span`
func (tag uint32TagName) Set(span opentracing.Span, value uint32) {
	span.SetTag(string(tag), value)
}

// ---

type uint16TagName string

// Set adds a uint16 tag to the `span`
func (tag uint16TagName) Set(span opentracing.Span, value uint16) {
	span.SetTag(string(tag), value)
}

// ---

type boolTagName string

// Add adds a bool tag to the `span`
func (tag boolTagName) Set(span opentracing.Span, value bool) {
	span.SetTag(string(tag), value)
}
