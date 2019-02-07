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

import "time"

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
