// Copyright 2021 The Prometheus Authors
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

package xds

import (
	"context"
	"time"

	v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	// Constants for instrumentation.
	namespace = "prometheus"
)

// ProtocolVersion is the xDS protocol version.
type ProtocolVersion string

const (
	ProtocolV3 = ProtocolVersion("v3")
)

type HTTPConfig struct {
	config.HTTPClientConfig `yaml:",inline"`
}

// SDConfig is a base config for xDS-based SD mechanisms.
type SDConfig struct {
	HTTPClientConfig config.HTTPClientConfig `yaml:",inline"`
	RefreshInterval  model.Duration          `yaml:"refresh_interval,omitempty"`
	FetchTimeout     model.Duration          `yaml:"fetch_timeout,omitempty"`
	Server           string                  `yaml:"server,omitempty"`
	ClientID         string                  `yaml:"client_id,omitempty"`
}

// mustRegisterMessage registers the provided message type in the typeRegistry, and panics
// if there is an error.
func mustRegisterMessage(typeRegistry *protoregistry.Types, mt protoreflect.MessageType) {
	if err := typeRegistry.RegisterMessage(mt); err != nil {
		panic(err)
	}
}

func init() {
	// Register top-level SD Configs.
	discovery.RegisterConfig(&KumaSDConfig{})

	// Register protobuf types that need to be marshalled/ unmarshalled.
	mustRegisterMessage(protoTypes, (&v3.DiscoveryRequest{}).ProtoReflect().Type())
	mustRegisterMessage(protoTypes, (&v3.DiscoveryResponse{}).ProtoReflect().Type())
	mustRegisterMessage(protoTypes, (&MonitoringAssignment{}).ProtoReflect().Type())
}

var (
	protoTypes            = new(protoregistry.Types)
	protoUnmarshalOptions = proto.UnmarshalOptions{
		DiscardUnknown: true,       // Only want known fields.
		Merge:          true,       // Always using new messages.
		Resolver:       protoTypes, // Only want known types.
	}
	protoJSONUnmarshalOptions = protojson.UnmarshalOptions{
		DiscardUnknown: true,       // Only want known fields.
		Resolver:       protoTypes, // Only want known types.
	}
	protoJSONMarshalOptions = protojson.MarshalOptions{
		UseProtoNames: true,
		Resolver:      protoTypes, // Only want known types.
	}
)

// resourceParser is a function that takes raw discovered objects and translates them into
// targetgroup.Group Targets. On error, no updates are sent to the scrape manager and the failure count is incremented.
type resourceParser func(resources []*anypb.Any, typeUrl string) ([]model.LabelSet, error)

// fetchDiscovery implements long-polling via xDS Fetch REST-JSON.
type fetchDiscovery struct {
	client ResourceClient
	source string

	refreshInterval time.Duration

	parseResources resourceParser
	logger         log.Logger

	metrics *xdsMetrics
}

func (d *fetchDiscovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer d.client.Close()

	ticker := time.NewTicker(d.refreshInterval)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		default:
			d.poll(ctx, ch)
			<-ticker.C
		}
	}
}

func (d *fetchDiscovery) poll(ctx context.Context, ch chan<- []*targetgroup.Group) {
	t0 := time.Now()
	response, err := d.client.Fetch(ctx)
	elapsed := time.Since(t0)
	d.metrics.fetchDuration.Observe(elapsed.Seconds())

	// Check the context before in order to exit early.
	select {
	case <-ctx.Done():
		return
	default:
	}

	if err != nil {
		level.Error(d.logger).Log("msg", "error parsing resources", "err", err)
		d.metrics.fetchFailuresCount.Inc()
		return
	}

	if response == nil {
		// No update needed.
		d.metrics.fetchSkipUpdateCount.Inc()
		return
	}

	parsedTargets, err := d.parseResources(response.Resources, response.TypeUrl)
	if err != nil {
		level.Error(d.logger).Log("msg", "error parsing resources", "err", err)
		d.metrics.fetchFailuresCount.Inc()
		return
	}

	level.Debug(d.logger).Log("msg", "Updated to version", "version", response.VersionInfo, "targets", len(parsedTargets))

	select {
	case <-ctx.Done():
		return
	case ch <- []*targetgroup.Group{{Source: d.source, Targets: parsedTargets}}:
	}
}
