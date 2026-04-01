// Copyright The Prometheus Authors
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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestDigitalOceanDBRefresh(t *testing.T) {
	mock := NewSDMock(t)
	mock.Setup()
	defer mock.ShutdownServer()

	mock.HandleDatabasesList()

	cfg := DefaultSDConfig
	cfg.Role = DatabasesRole
	cfg.Port = 9273
	cfg.HTTPClient = mock.Server.Client()

	d, err := newRefresher(&cfg)
	require.NoError(t, err)

	// Inject the mock URL into the godo client since we can't easily change it via HTTPClient.
	endpoint, _ := url.Parse(mock.Endpoint())
	d.(*databasesDiscovery).client.BaseURL = endpoint

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.Len(t, tg.Targets, 2)

	expectedTargets := []struct {
		labels model.LabelSet
	}{
		{
			labels: model.LabelSet{
				"__address__":                       "do-fra1-pg-trading-001-do-user-XXXXX-0.f.db.ondigitalocean.com:9273",
				"__meta_digitalocean_db_id":         "9cc10173-e9ea-4176-9dbc-a4cee4c4ff30",
				"__meta_digitalocean_db_name":       "do-fra1-pg-trading-001",
				"__meta_digitalocean_db_engine":     "pg",
				"__meta_digitalocean_db_version":    "16",
				"__meta_digitalocean_db_status":     "online",
				"__meta_digitalocean_db_region":     "fra1",
				"__meta_digitalocean_db_size":       "db-s-1vcpu-2gb",
				"__meta_digitalocean_db_num_nodes":  "1",
				"__meta_digitalocean_db_host":       "do-fra1-pg-trading-001-do-user-XXXXX-0.f.db.ondigitalocean.com",
				"__meta_digitalocean_db_tag_prod":   "true",
				"__meta_digitalocean_db_tag_market": "true",
			},
		},
		{
			labels: model.LabelSet{
				"__address__":                         "private-db-host:9273",
				"__meta_digitalocean_db_id":           "a0b1c2d3-e4f5-4677-8899-001122334455",
				"__meta_digitalocean_db_name":         "private-db",
				"__meta_digitalocean_db_engine":       "mysql",
				"__meta_digitalocean_db_version":      "8",
				"__meta_digitalocean_db_status":       "online",
				"__meta_digitalocean_db_region":       "nyc1",
				"__meta_digitalocean_db_size":         "db-s-2vcpu-4gb",
				"__meta_digitalocean_db_num_nodes":    "2",
				"__meta_digitalocean_db_host":         "public-db-host",
				"__meta_digitalocean_db_private_host": "private-db-host",
			},
		},
	}

	for i, expected := range expectedTargets {
		require.Equal(t, expected.labels, tg.Targets[i])
	}
}

func TestDigitalOceanDBRefreshPagination(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "1", "":
			fmt.Fprint(w, `
{
  "databases": [
    {"id": "db-1", "name": "db-1", "engine": "pg", "version": "16", "status": "online", "region": "fra1", "size": "db-s-1vcpu-2gb", "num_nodes": 1}
  ],
  "links": {
    "pages": {
      "next": "http://example.com/v2/databases?page=2"
    }
  }
}
`)
		case "2":
			fmt.Fprint(w, `
{
  "databases": [
    {"id": "db-2", "name": "db-2", "engine": "pg", "version": "16", "status": "online", "region": "fra1", "size": "db-s-1vcpu-2gb", "num_nodes": 1}
  ]
}
`)
		default:
			fmt.Fprint(w, `{"databases": []}`)
		}
	}))
	defer ts.Close()

	cfg := DefaultSDConfig
	cfg.Role = DatabasesRole
	cfg.HTTPClient = ts.Client()

	d, err := newRefresher(&cfg)
	require.NoError(t, err)

	endpoint, _ := url.Parse(ts.URL)
	d.(*databasesDiscovery).client.BaseURL = endpoint

	tgs, err := d.refresh(context.Background())
	require.NoError(t, err)
	require.Len(t, tgs, 1)
	require.Len(t, tgs[0].Targets, 2)
}
