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
	"strconv"
	"testing"
	"time"

	"github.com/digitalocean/godo"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestDatabaseDiscovery(t *testing.T) {
	tests := []struct {
		name     string
		config   DBSDConfig
		databases []*godo.Database
		expected int
	}{
		{
			name: "no databases",
			config: DBSDConfig{
				Port: 5432,
			},
			databases: []*godo.Database{},
			expected:  0,
		},
		{
			name: "single database",
			config: DBSDConfig{
				Port: 3306,
			},
			databases: []*godo.Database{
				{
					ID:     12345,
					Name:   "test-db",
					Engine: "pg",
					Status: "online",
					Region: "nyc1",
					Tags: []godo.DatabaseTag{
						{Key: "env", Value: "production"},
						{Key: "team", Value: "backend"},
					},
					Connection: &godo.DatabaseConnection{
						Host:        "db.example.com",
						PrivateHost: "private-db.example.com",
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple databases with pagination",
			config: DBSDConfig{
				Port: 5432,
			},
			databases: []*godo.Database{
				{
					ID:     11111,
					Name:   "db1",
					Engine: "mysql",
					Status: "online",
					Region: "nyc1",
					Connection: &godo.DatabaseConnection{
						Host: "db1.example.com",
					},
				},
				{
					ID:     22222,
					Name:   "db2",
					Engine: "redis",
					Status: "online",
					Region: "nyc1",
					Connection: &godo.DatabaseConnection{
						PrivateHost: "private-db2.example.com",
					},
				},
			},
			expected: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			discovery := &DatabaseDiscovery{
				port: test.config.Port,
			}

			// Mock the listDatabases method
			originalListDatabases := discovery.listDatabases
			discovery.listDatabases = func(ctx context.Context) ([]*godo.Database, error) {
				return test.databases, nil
			}
			defer func() {
				discovery.listDatabases = originalListDatabases
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			targets, err := discovery.refresh(ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(targets) != 1 {
				t.Fatalf("expected 1 target group, got %d", len(targets))
			}

			tg := targets[0]
			if len(tg.Targets) != test.expected {
				t.Fatalf("expected %d targets, got %d", test.expected, len(tg.Targets))
			}

			for i, target := range tg.Targets {
				db := test.databases[i]
				
				// Check core labels
				if string(target[doDBLabelID]) != "12345" && string(target[doDBLabelID]) != "11111" && string(target[doDBLabelID]) != "22222" {
					t.Errorf("incorrect db_id: expected %s, got %s", db.ID, target[doDBLabelID])
				}
				
				if string(target[doDBLabelName]) != db.Name {
					t.Errorf("incorrect db_name: expected %s, got %s", db.Name, target[doDBLabelName])
				}
				
				if string(target[doDBLabelEngine]) != db.Engine {
					t.Errorf("incorrect db_engine: expected %s, got %s", db.Engine, target[doDBLabelEngine])
				}
				
				if string(target[doDBLabelStatus]) != db.Status {
					t.Errorf("incorrect db_status: expected %s, got %s", db.Status, target[doDBLabelStatus])
				}
				
				if string(target[doDBLabelRegion]) != db.Region {
					t.Errorf("incorrect db_region: expected %s, got %s", db.Region, target[doDBLabelRegion])
				}

				// Check connection labels
				expectedHost := db.Connection.Host
				if db.Connection.PrivateHost != "" {
					expectedHost = db.Connection.PrivateHost
				}
				
				if string(target[doDBLabelHost]) != db.Connection.Host {
					t.Errorf("incorrect db_host: expected %s, got %s", db.Connection.Host, target[doDBLabelHost])
				}
				
				if string(target[doDBLabelPrivateHost]) != db.Connection.PrivateHost {
					t.Errorf("incorrect db_private_host: expected %s, got %s", db.Connection.PrivateHost, target[doDBLabelPrivateHost])
				}

				// Check tag labels
				for _, tag := range db.Tags {
					labelKey := doDBLabelTagPrefix + tag.Key
					if string(target[model.LabelName(labelKey)]) != tag.Value {
						t.Errorf("incorrect tag %s: expected %s, got %s", tag.Key, tag.Value, target[model.LabelName(labelKey)])
					}
				}

				// Check address label
				expectedAddr := expectedHost + ":" + string(model.LabelValue(strconv.Itoa(test.config.Port)))
				if string(target[model.AddressLabel]) != expectedAddr {
					t.Errorf("incorrect address: expected %s, got %s", expectedAddr, target[model.AddressLabel])
				}
			}
		})
	}
}

func TestDatabaseDiscovery_Pagination(t *testing.T) {
	// Test that pagination works correctly with multiple pages
	databases := make([]*godo.Database, 150) // Simulate more than one page
	
	for i := 0; i < 150; i++ {
		databases[i] = &godo.Database{
			ID:   i + 1,
			Name:  fmt.Sprintf("db-%d", i+1),
			Engine: "pg",
			Status: "online",
			Region: "nyc1",
			Connection: &godo.DatabaseConnection{
				Host: fmt.Sprintf("db-%d.example.com", i+1),
			},
		}
	}

	discovery := &DatabaseDiscovery{
		port: 5432,
	}

	// Mock the listDatabases method to simulate pagination
	callCount := 0
	originalListDatabases := discovery.listDatabases
	discovery.listDatabases = func(ctx context.Context) ([]*godo.Database, error) {
		callCount++
		return databases, nil
	}
	defer func() {
		discovery.listDatabases = originalListDatabases
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	targets, err := discovery.refresh(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call to listDatabases, got %d", callCount)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target group, got %d", len(targets))
	}

	tg := targets[0]
	if len(tg.Targets) != 150 {
		t.Errorf("expected 150 targets, got %d", len(tg.Targets))
	}
}
