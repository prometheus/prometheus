// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ocagent

import (
	"time"

	"google.golang.org/grpc/credentials"
)

const (
	DefaultAgentPort uint16 = 55678
	DefaultAgentHost string = "localhost"
)

type ExporterOption interface {
	withExporter(e *Exporter)
}

type insecureGrpcConnection int

var _ ExporterOption = (*insecureGrpcConnection)(nil)

func (igc *insecureGrpcConnection) withExporter(e *Exporter) {
	e.canDialInsecure = true
}

// WithInsecure disables client transport security for the exporter's gRPC connection
// just like grpc.WithInsecure() https://godoc.org/google.golang.org/grpc#WithInsecure
// does. Note, by default, client security is required unless WithInsecure is used.
func WithInsecure() ExporterOption { return new(insecureGrpcConnection) }

type addressSetter string

func (as addressSetter) withExporter(e *Exporter) {
	e.agentAddress = string(as)
}

var _ ExporterOption = (*addressSetter)(nil)

// WithAddress allows one to set the address that the exporter will
// connect to the agent on. If unset, it will instead try to use
// connect to DefaultAgentHost:DefaultAgentPort
func WithAddress(addr string) ExporterOption {
	return addressSetter(addr)
}

type serviceNameSetter string

func (sns serviceNameSetter) withExporter(e *Exporter) {
	e.serviceName = string(sns)
}

var _ ExporterOption = (*serviceNameSetter)(nil)

// WithServiceName allows one to set/override the service name
// that the exporter will report to the agent.
func WithServiceName(serviceName string) ExporterOption {
	return serviceNameSetter(serviceName)
}

type reconnectionPeriod time.Duration

func (rp reconnectionPeriod) withExporter(e *Exporter) {
	e.reconnectionPeriod = time.Duration(rp)
}

func WithReconnectionPeriod(rp time.Duration) ExporterOption {
	return reconnectionPeriod(rp)
}

type compressorSetter string

func (c compressorSetter) withExporter(e *Exporter) {
	e.compressor = string(c)
}

// UseCompressor will set the compressor for the gRPC client to use when sending requests.
// It is the responsibility of the caller to ensure that the compressor set has been registered
// with google.golang.org/grpc/encoding. This can be done by encoding.RegisterCompressor. Some
// compressors auto-register on import, such as gzip, which can be registered by calling
// `import _ "google.golang.org/grpc/encoding/gzip"`
func UseCompressor(compressorName string) ExporterOption {
	return compressorSetter(compressorName)
}

type headerSetter map[string]string

func (h headerSetter) withExporter(e *Exporter) {
	e.headers = map[string]string(h)
}

// WithHeaders will send the provided headers when the gRPC stream connection
// is instantiated
func WithHeaders(headers map[string]string) ExporterOption {
	return headerSetter(headers)
}

type clientCredentials struct {
	credentials.TransportCredentials
}

var _ ExporterOption = (*clientCredentials)(nil)

// WithTLSCredentials allows the connection to use TLS credentials
// when talking to the server. It takes in grpc.TransportCredentials instead
// of say a Certificate file or a tls.Certificate, because the retrieving
// these credentials can be done in many ways e.g. plain file, in code tls.Config
// or by certificate rotation, so it is up to the caller to decide what to use.
func WithTLSCredentials(creds credentials.TransportCredentials) ExporterOption {
	return &clientCredentials{TransportCredentials: creds}
}

func (cc *clientCredentials) withExporter(e *Exporter) {
	e.clientTransportCredentials = cc.TransportCredentials
}
