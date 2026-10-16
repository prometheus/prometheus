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

package stackit

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"
)

type serverSDTestSuite struct {
	Mock *SDMock
}

func (s *serverSDTestSuite) SetupTest(t *testing.T) {
	s.Mock = NewSDMock(t)
	s.Mock.Setup()

	s.Mock.HandleServers()
}

func TestServerSDRefresh(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  SDConfig
	}{
		{
			name: "default with token",
			cfg: func() SDConfig {
				cfg := DefaultSDConfig
				cfg.HTTPClientConfig.BearerToken = testToken

				return cfg
			}(),
		},
		{
			name: "default with service account key",
			cfg: func() SDConfig {
				// Generate a new RSA key pair with a size of 2048 bits
				key, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)

				cfg := DefaultSDConfig
				cfg.PrivateKey = config.Secret(pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(key),
				}))

				cfg.ServiceAccountKey = config.Secret(`{
  "Active": true,
  "CreatedAt": "2025-04-05T12:34:56Z",
  "Credentials": {
    "Aud": "https://stackit-service-account-prod.apps.01.cf.eu01.stackit.cloud",
    "Iss": "stackit@sa.stackit.cloud",
    "Kid": "123e4567-e89b-12d3-a456-426614174000",
    "Sub": "123e4567-e89b-12d3-a456-426614174001"
  },
  "ID": "123e4567-e89b-12d3-a456-426614174002",
  "KeyAlgorithm": "RSA_2048",
  "KeyOrigin": "USER_PROVIDED",
  "KeyType": "USER_MANAGED",
  "PublicKey": "...",
  "ValidUntil": "2025-04-05T13:34:56Z"
}`)

				return cfg
			}(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite := &serverSDTestSuite{}
			suite.SetupTest(t)
			defer suite.Mock.ShutdownServer()

			tc.cfg.Endpoint = suite.Mock.Endpoint()
			tc.cfg.tokenURL = suite.Mock.Endpoint() + "token"
			tc.cfg.Project = testProjectID

			d, err := newClient(&tc.cfg, promslog.NewNopLogger())
			require.NoError(t, err)

			targetGroups, err := d.refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, targetGroups, 1)

			targetGroup := targetGroups[0]
			require.NotNil(t, targetGroup, "targetGroup should not be nil")
			require.NotNil(t, targetGroup.Targets, "targetGroup.targets should not be nil")
			require.Len(t, targetGroup.Targets, 1)

			expectedTargets := []model.LabelSet{
				{
					"__address__":                      model.LabelValue("192.0.2.1:80"),
					"__meta_stackit_project":           model.LabelValue("00000000-0000-0000-0000-000000000000"),
					"__meta_stackit_id":                model.LabelValue("b4176700-596a-4f80-9fc8-5f9c58a606e1"),
					"__meta_stackit_type":              model.LabelValue("g1.1"),
					"__meta_stackit_private_ipv4_test": model.LabelValue("10.0.0.153"),
					"__meta_stackit_public_ipv4":       model.LabelValue("192.0.2.1"),
					"__meta_stackit_labelpresent_provisionSTACKITServerAgent": model.LabelValue("true"),
					"__meta_stackit_label_provisionSTACKITServerAgent":        model.LabelValue("true"),
					"__meta_stackit_labelpresent_stackit_project_id":          model.LabelValue("true"),
					"__meta_stackit_name":                                     model.LabelValue("runcommandtest"),
					"__meta_stackit_availability_zone":                        model.LabelValue("eu01-3"),
					"__meta_stackit_status":                                   model.LabelValue("INACTIVE"),
					"__meta_stackit_power_status":                             model.LabelValue("STOPPED"),
					"__meta_stackit_label_stackit_project_id":                 model.LabelValue("00000000-0000-0000-0000-000000000000"),
				},
			}

			for i, labelSet := range expectedTargets {
				require.Equal(t, labelSet, targetGroup.Targets[i])
			}
		})
	}
}

func TestGetPostgresInstances(t *testing.T) {
	for _, tc := range []struct {
		name           string
		handler        func(w http.ResponseWriter, r *http.Request)
		expectedLen    int
		expectError    bool
		expectedLabels model.LabelSet
	}{
		{
			name: "success with instances",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{
					"items": [
						{"id": "pg-111", "name": "db-1", "status": "READY"}
					],
					"count": 1
				}`))
			},
			expectedLen: 1,
			expectedLabels: model.LabelSet{
				"__address__":            model.LabelValue("postgres-prom-proxy.api.stackit.cloud:443"),
				"__metrics_path__":       model.LabelValue("/v2/projects/test-proj/regions/eu01/instances/pg-111/metrics"),
				"__scheme__":             model.LabelValue("https"),
				"__meta_stackit_project": model.LabelValue("test-proj"),
				"__meta_stackit_id":      model.LabelValue("pg-111"),
				"__meta_stackit_name":    model.LabelValue("db-1"),
				"__meta_stackit_status":  model.LabelValue("READY"),
				"__meta_stackit_type":    model.LabelValue("postgres"),
			},
		},
		{
			name: "empty items list",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"items": [], "count": 0}`))
			},
			expectedLen: 0,
		},
		{
			name: "unexpected HTTP status code",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`Internal Server Error`))
			},
			expectError: true,
		},
		{
			name: "invalid JSON response",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{invalid-json`))
			},
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite := &serverSDTestSuite{}
			suite.SetupTest(t)
			defer suite.Mock.ShutdownServer()

			suite.Mock.Mux.HandleFunc("/v2/projects/test-proj/regions/eu01/instances", tc.handler)

			cfg := DefaultSDConfig
			cfg.HTTPClientConfig.BearerToken = testToken
			cfg.Endpoint = suite.Mock.Endpoint()
			cfg.tokenURL = suite.Mock.Endpoint() + "token"
			cfg.Project = "test-proj"

			c, err := newClient(&cfg, promslog.NewNopLogger())
			require.NoError(t, err)

			targets, err := c.getPostgresInstances(context.Background())
			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, targets, tc.expectedLen)

			if tc.expectedLen > 0 {
				require.Equal(t, tc.expectedLabels, targets[0])
			}
		})
	}
}

func TestBuildURL(t *testing.T) {
	for _, tc := range []struct {
		name         string
		baseEndpoint string
		segments     []string
		expected     string
		expectError  bool
	}{
		{
			name:         "valid endpoint with segments",
			baseEndpoint: "https://iaas.api.eu01.stackit.cloud",
			segments:     []string{"v1", "projects", "proj-123", "servers"},
			expected:     "https://iaas.api.eu01.stackit.cloud/v1/projects/proj-123/servers?details=true",
		},
		{
			name:         "valid endpoint with trailing slash in base",
			baseEndpoint: "http://127.0.0.1:8080/",
			segments:     []string{"v2", "instances"},
			expected:     "http://127.0.0.1:8080/v2/instances?details=true",
		},
		{
			name:         "invalid base endpoint URL",
			baseEndpoint: "://invalid-url",
			segments:     []string{"v1"},
			expectError:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := buildURL(tc.baseEndpoint, tc.segments...)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, res)
			}
		})
	}
}

func TestFetchJSON(t *testing.T) {
	type testResponse struct {
		Message string `json:"message"`
	}

	for _, tc := range []struct {
		name        string
		handler     func(w http.ResponseWriter, _ *http.Request)
		expected    *testResponse
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful JSON decode",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"message": "hello world"}`))
			},
			expected: &testResponse{Message: "hello world"},
		},
		{
			name: "non-200 status code",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`bad request error`))
			},
			expectError: true,
			errorMsg:    "unexpected status code 400: bad request error",
		},
		{
			name: "invalid JSON body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`not-json`))
			},
			expectError: true,
			errorMsg:    "decoding response",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tc.handler))
			t.Cleanup(server.Close)

			cfg := DefaultSDConfig
			c, err := newClient(&cfg, promslog.NewNopLogger())
			require.NoError(t, err)

			var res testResponse
			err = c.fetchJSON(context.Background(), server.URL, &res)
			if tc.expectError {
				require.Error(t, err)
				if tc.errorMsg != "" {
					require.Contains(t, err.Error(), tc.errorMsg)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, *tc.expected, res)
			}
		})
	}

	t.Run("invalid request URL", func(t *testing.T) {
		cfg := DefaultSDConfig
		c, err := newClient(&cfg, promslog.NewNopLogger())
		require.NoError(t, err)

		var res testResponse
		err = c.fetchJSON(context.Background(), "::not-a-valid-url", &res)
		require.Error(t, err)
		require.Contains(t, err.Error(), "creating request")
	})
}

func TestRoleFiltering(t *testing.T) {
	for _, tc := range []struct {
		name        string
		role        Role
		expectedLen int
	}{
		{
			name:        "role server returns only server targets",
			role:        RoleServer,
			expectedLen: 1,
		},
		{
			name:        "role postgres returns only postgres targets",
			role:        RolePostgres,
			expectedLen: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite := &serverSDTestSuite{}
			suite.SetupTest(t)
			defer suite.Mock.ShutdownServer()

			cfg := DefaultSDConfig
			cfg.HTTPClientConfig.BearerToken = testToken
			cfg.Endpoint = suite.Mock.Endpoint()
			cfg.tokenURL = suite.Mock.Endpoint() + "token"
			cfg.Project = testProjectID
			cfg.Role = tc.role

			c, err := newClient(&cfg, promslog.NewNopLogger())
			require.NoError(t, err)

			targetGroups, err := c.refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, targetGroups, 1)
			require.Len(t, targetGroups[0].Targets, tc.expectedLen)
		})
	}
}

func TestRoleUnmarshalYAML(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		expected    Role
		expectError bool
	}{
		{
			name:     "server role",
			input:    "role: server\n",
			expected: RoleServer,
		},
		{
			name:     "postgres role",
			input:    "role: postgres\n",
			expected: RolePostgres,
		},
		{
			name:        "invalid role",
			input:       "role: invalid\n",
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s struct {
				Role Role `yaml:"role"`
			}
			err := yaml.Unmarshal([]byte(tc.input), &s)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, s.Role)
			}
		})
	}
}

func TestSDConfigUnmarshalYAML(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected SDConfig
	}{
		{
			name:  "default role when omitted",
			input: "project: 00000000-0000-0000-0000-000000000000\n",
			expected: func() SDConfig {
				cfg := DefaultSDConfig
				cfg.Project = "00000000-0000-0000-0000-000000000000"
				return cfg
			}(),
		},
		{
			name:  "explicit postgres role",
			input: "project: 00000000-0000-0000-0000-000000000000\nrole: postgres\n",
			expected: func() SDConfig {
				cfg := DefaultSDConfig
				cfg.Project = "00000000-0000-0000-0000-000000000000"
				cfg.Role = RolePostgres
				return cfg
			}(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg SDConfig
			err := yaml.Unmarshal([]byte(tc.input), &cfg)
			require.NoError(t, err)
			require.Equal(t, tc.expected, cfg)
		})
	}
}
