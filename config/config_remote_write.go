package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// TODO(bwplotka): Consider an util for "preference enums" as it's similar code for rw compression, proto type and scrape protocol.

// RemoteWriteProtoType represents the supported protobuf types for the remote write.
type RemoteWriteProtoType string

// Validate returns error if the given protobuf type is not supported.
func (s RemoteWriteProtoType) Validate() error {
	if _, ok := RemoteWriteContentTypeHeaders[s]; !ok {
		return fmt.Errorf("unknown remote write protobuf type %v, supported: %v",
			s, func() (ret []string) {
				for k := range RemoteWriteContentTypeHeaders {
					ret = append(ret, string(k))
				}
				sort.Strings(ret)
				return ret
			}())
	}
	return nil
}

type RemoteWriteProtoTypes []RemoteWriteProtoType

func (t RemoteWriteProtoTypes) Strings() []string {
	ret := make([]string, 0, len(t))
	for _, typ := range t {
		ret = append(ret, string(typ))
	}
	return ret
}

func (t RemoteWriteProtoTypes) String() string {
	return strings.Join(t.Strings(), ",")
}

// ServerAcceptHeaderValue returns server Accept header value for
// given list of proto types as per RFC 9110 https://www.rfc-editor.org/rfc/rfc9110.html#section-12.5.1-14
func (t RemoteWriteProtoTypes) ServerAcceptHeaderValue() string {
	// TODO(bwplotka): Consider implementing an optional quality factor.
	ret := make([]string, 0, len(t))
	for _, typ := range t {
		ret = append(ret, RemoteWriteContentTypeHeaders[typ])
	}
	return strings.Join(ret, ",")
}

var (
	RemoteWriteProtoTypeV1        RemoteWriteProtoType = "v1.WriteRequest"
	RemoteWriteProtoTypeV2        RemoteWriteProtoType = "v2.WriteRequest"
	RemoteWriteContentTypeHeaders                      = map[RemoteWriteProtoType]string{
		RemoteWriteProtoTypeV1: "application/x-protobuf", // Also application/x-protobuf; proto=prometheus.WriteRequest but simplified for compatibility with 1.x spec.
		RemoteWriteProtoTypeV2: "application/x-protobuf;proto=io.prometheus.remote.write.v2.WriteRequest",
	}

	// DefaultRemoteWriteProtoTypes is the set of remote write protobuf types that will be
	// preferred by the remote write client.
	DefaultRemoteWriteProtoTypes = RemoteWriteProtoTypes{
		RemoteWriteProtoTypeV1,
		RemoteWriteProtoTypeV2,
	}
)

// validateRemoteWriteProtoTypes return errors if we see problems with rw protobuf types in
// the Prometheus configuration.
func validateRemoteWriteProtoTypes(ts []RemoteWriteProtoType) error {
	if len(ts) == 0 {
		return errors.New("protobuf_types cannot be empty")
	}
	dups := map[string]struct{}{}
	for _, t := range ts {
		if _, ok := dups[strings.ToLower(string(t))]; ok {
			return fmt.Errorf("duplicated protobuf types in protobuf_types, got %v", ts)
		}
		if err := t.Validate(); err != nil {
			return fmt.Errorf("protobuf_types: %w", err)
		}
		dups[strings.ToLower(string(t))] = struct{}{}
	}
	return nil
}

// RemoteWriteCompression represents the supported compressions for the remote write.
type RemoteWriteCompression string

// Validate returns error if the given protobuf type is not supported.
func (s RemoteWriteCompression) Validate() error {
	if _, ok := RemoteWriteContentEncodingHeaders[s]; !ok {
		return fmt.Errorf("unknown remote write protobuf type %v, supported: %v",
			s, func() (ret []string) {
				for k := range RemoteWriteContentEncodingHeaders {
					ret = append(ret, string(k))
				}
				sort.Strings(ret)
				return ret
			}())
	}
	return nil
}

type RemoteWriteCompressions []RemoteWriteCompression

func (cs RemoteWriteCompressions) Strings() []string {
	ret := make([]string, 0, len(cs))
	for _, c := range cs {
		ret = append(ret, string(c))
	}
	return ret
}

func (cs RemoteWriteCompressions) String() string {
	return strings.Join(cs.Strings(), ",")
}

// ServerAcceptEncodingHeaderValue returns server Accept-Encoding header value for
// given list of compressions as per RFC 9110 https://www.rfc-editor.org/rfc/rfc9110.html#name-accept-encoding
func (cs RemoteWriteCompressions) ServerAcceptEncodingHeaderValue() string {
	// TODO(bwplotka): Consider implementing an optional quality factor.
	ret := make([]string, 0, len(cs))
	for _, typ := range cs {
		ret = append(ret, RemoteWriteContentEncodingHeaders[typ])
	}
	return strings.Join(ret, ",")
}

// validateRemoteWriteCompressions return errors if we see problems with rw compressions in
// the Prometheus configuration.
func validateRemoteWriteCompressions(cs []RemoteWriteCompression) error {
	if len(cs) == 0 {
		return errors.New("compressions cannot be empty")
	}
	dups := map[string]struct{}{}
	for _, c := range cs {
		if _, ok := dups[strings.ToLower(string(c))]; ok {
			return fmt.Errorf("duplicated compression in compressions, got %v", cs)
		}
		if err := c.Validate(); err != nil {
			return fmt.Errorf("compressions: %w", err)
		}
		dups[strings.ToLower(string(c))] = struct{}{}
	}
	return nil
}

var (
	RemoteWriteCompressionSnappy RemoteWriteCompression = "snappy"

	RemoteWriteContentEncodingHeaders = map[RemoteWriteCompression]string{
		RemoteWriteCompressionSnappy: "snappy",
	}

	DefaultRemoteWriteCompressions = RemoteWriteCompressions{
		RemoteWriteCompressionSnappy,
	}
)
