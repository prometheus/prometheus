// Copyright 2016 The Prometheus Authors
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

package digitalocean

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/digitalocean/godo"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"

	"github.com/prometheus/common/model"
)

var (
	godoBase *url.URL

	d = &Discovery{
		region:   "nyc3",
		interval: time.Duration(DefaultSDConfig.RefreshInterval),
		port:     DefaultSDConfig.Port,
		logger:   log.NewNopLogger(),
	}
)

func TestListDroplets(t *testing.T) {
	var dropletTests = []struct {
		resp     string
		expected []godo.Droplet
	}{
		{`{"droplets": [
            {"region":{"slug":"nyc3"}},
            {"region":{"slug":"nyc3"}}
        ]}`,
			[]godo.Droplet{
				godo.Droplet{Region: &godo.Region{Slug: "nyc3"}},
				godo.Droplet{Region: &godo.Region{Slug: "nyc3"}},
			}},
		{`{"droplets": [
            {"region":{"slug":"nyc3"}},
            {"region":{"slug":"nyc3"}},
            {"region":{"slug":"sfo1"}}
        ]}`,
			// We should only find Droplets in the specified region.
			[]godo.Droplet{
				godo.Droplet{Region: &godo.Region{Slug: "nyc3"}},
				godo.Droplet{Region: &godo.Region{Slug: "nyc3"}},
			}},
	}

	for _, tt := range dropletTests {
		mockServer(t, "/v2/droplets", tt.resp, func() {
			d.client = getGodoClient()
			droplets, err := listDroplets(d)
			assert.Nil(t, err)
			assert.Equal(t, tt.expected, droplets, "they should be equal")
		})
	}
}

func TestBuildMetadata(t *testing.T) {
	droplets := []godo.Droplet{
		godo.Droplet{
			ID:     12345,
			Name:   "test-node-01",
			Status: "active",
			Size:   &godo.Size{Slug: "1gb"},
			Region: &godo.Region{Slug: "nyc3"},
			Networks: &godo.Networks{
				V4: []godo.NetworkV4{
					godo.NetworkV4{IPAddress: "192.0.2.10", Type: "public"},
					godo.NetworkV4{IPAddress: "192.0.2.11", Type: "private"},
				},
			},
			Tags: []string{"foo", "bar", "baz"},
		},
		godo.Droplet{
			Networks: &godo.Networks{
				V4: []godo.NetworkV4{
					godo.NetworkV4{IPAddress: "192.0.2.12", Type: "public"},
				},
			},
		},
	}

	tg, err := buildMetadata(d, droplets)
	assert.Nil(t, err)
	// Nodes not on the private network should not be returned.
	assert.Equal(t, len(tg.Targets), 1)
	assert.Equal(t, tg.Targets[0]["__address__"], model.LabelValue("192.0.2.11:80"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_instance_id"], model.LabelValue("12345"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_instance_name"], model.LabelValue("test-node-01"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_instance_status"], model.LabelValue("active"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_instance_size"], model.LabelValue("1gb"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_region"], model.LabelValue("nyc3"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_public_ipv4"], model.LabelValue("192.0.2.10"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_private_ipv4"], model.LabelValue("192.0.2.11"))
	assert.Equal(t, tg.Targets[0]["__meta_digitalocean_instance_tags"], model.LabelValue(",foo,bar,baz,"))
}

func getGodoClient() *godo.Client {
	ts := &TokenSource{AccessToken: "fake-testing-token"}
	oauthClient := oauth2.NewClient(oauth2.NoContext, ts)
	c := godo.NewClient(oauthClient)
	c.BaseURL = godoBase

	return c
}

func mockServer(t testing.TB, path string, resp string, test func()) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("Wrong URL: %v", r.URL.String())
			return
		}
		w.WriteHeader(200)
		fmt.Fprintln(w, resp)
	}))

	u, err := url.Parse(server.URL)
	assert.Nil(t, err)
	godoBase = u

	defer server.Close()
	test()
}
