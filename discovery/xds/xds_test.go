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
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	sdConf = SDConfig{
		Server:          "http://127.0.0.1",
		RefreshInterval: model.Duration(10 * time.Second),
	}

	testFetchFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{})
	testFetchSkipUpdateCount = prometheus.NewCounter(
		prometheus.CounterOpts{})
	testFetchDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{},
	)
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type discoveryResponder func(request *v3.DiscoveryRequest) (*v3.DiscoveryResponse, error)

func createTestHTTPServer(t *testing.T, responder discoveryResponder) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate req MIME types.
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))

		body, err := ioutil.ReadAll(r.Body)
		defer func() {
			_, _ = io.Copy(ioutil.Discard, r.Body)
			_ = r.Body.Close()
		}()
		require.NotEmpty(t, body)
		require.NoError(t, err)

		// Validate discovery request.
		discoveryReq := &v3.DiscoveryRequest{}
		err = protoJSONUnmarshalOptions.Unmarshal(body, discoveryReq)
		require.NoError(t, err)

		discoveryRes, err := responder(discoveryReq)
		if err != nil {
			w.WriteHeader(500)
			return
		}

		if discoveryRes == nil {
			w.WriteHeader(304)
			return
		}

		w.WriteHeader(200)
		data, err := protoJSONMarshalOptions.Marshal(discoveryRes)
		require.NoError(t, err)

		_, err = w.Write(data)
		require.NoError(t, err)
	}))
}

func constantResourceParser(groups []*targetgroup.Group, err error) resourceParser {
	return func(resources []*anypb.Any, typeUrl string) ([]*targetgroup.Group, error) {
		return groups, err
	}
}

var nopLogger = log.NewNopLogger()

type testResourceClient struct {
	resourceTypeURL string
	server          string
	protocolVersion ProtocolVersion
	fetch           func(ctx context.Context) (*v3.DiscoveryResponse, error)
}

func (rc testResourceClient) ResourceTypeURL() string {
	return rc.resourceTypeURL
}

func (rc testResourceClient) Server() string {
	return rc.server
}

func (rc testResourceClient) Fetch(ctx context.Context) (*v3.DiscoveryResponse, error) {
	return rc.fetch(ctx)
}

func (rc testResourceClient) ID() string {
	return "test-client"
}

func (rc testResourceClient) Close() {
}

func TestPollingRefreshSkipUpdate(t *testing.T) {
	rc := &testResourceClient{
		fetch: func(ctx context.Context) (*v3.DiscoveryResponse, error) {
			return nil, nil
		},
	}
	pd := &fetchDiscovery{
		client:               rc,
		logger:               nopLogger,
		fetchDuration:        testFetchDuration,
		fetchFailuresCount:   testFetchFailuresCount,
		fetchSkipUpdateCount: testFetchSkipUpdateCount,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	ch := make(chan []*targetgroup.Group, 1)
	pd.poll(ctx, ch)
	select {
	case <-ctx.Done():
		return
	case <-ch:
		require.Fail(t, "no update expected")
	}
}

func TestPollingRefreshAttachesGroupMetadata(t *testing.T) {
	server := "http://198.161.2.0"
	source := "test"
	rc := &testResourceClient{
		server:          server,
		protocolVersion: ProtocolV3,
		fetch: func(ctx context.Context) (*v3.DiscoveryResponse, error) {
			return &v3.DiscoveryResponse{}, nil
		},
	}
	pd := &fetchDiscovery{
		source:               source,
		client:               rc,
		logger:               nopLogger,
		fetchDuration:        testFetchDuration,
		fetchFailuresCount:   testFetchFailuresCount,
		fetchSkipUpdateCount: testFetchSkipUpdateCount,
		parseResources: constantResourceParser([]*targetgroup.Group{
			{},
			{
				Source: "a-custom-source",
				Labels: model.LabelSet{
					"__meta_custom_xds_label": "a-value",
				},
			},
		}, nil),
	}
	ch := make(chan []*targetgroup.Group, 1)
	pd.poll(context.Background(), ch)
	groups := <-ch
	require.NotNil(t, groups)

	require.Len(t, groups, 2)

	for _, group := range groups {
		require.Equal(t, source, group.Source)
	}

	group2 := groups[1]
	require.Contains(t, group2.Labels, model.LabelName("__meta_custom_xds_label"))
	require.Equal(t, model.LabelValue("a-value"), group2.Labels["__meta_custom_xds_label"])
}
