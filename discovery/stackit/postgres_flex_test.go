// Copyright 2020 The Prometheus Authors
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
	"fmt"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
)

type postgresFlexSDTestSuite struct {
	Mock *SDMock
}

func (p *postgresFlexSDTestSuite) SetupTest(t *testing.T) {
	p.Mock = NewSDMock(t)
	p.Mock.Setup()

	p.Mock.HandlePostgresFlex()
}

func TestPostgresFlexSDRefresh(t *testing.T) {
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
				cfg.PrivateKey = string(pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(key),
				}))

				cfg.ServiceAccountKey = `{
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
}`

				return cfg
			}(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suite := &postgresFlexSDTestSuite{}
			suite.SetupTest(t)

			tc.cfg.Endpoint = suite.Mock.Endpoint()
			tc.cfg.tokenURL = suite.Mock.Endpoint() + "token"
			tc.cfg.Project = testProjectID
			tc.cfg.Region = testRegion

			d, err := newPostgresFlexDiscovery(&tc.cfg, promslog.NewNopLogger())
			require.NoError(t, err)

			targetGroups, err := d.refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, targetGroups, 1)

			targetGroup := targetGroups[0]
			require.NotNil(t, targetGroup, "targetGroup should not be nil")
			require.NotNil(t, targetGroup.Targets, "targetGroup.targets should not be nil")
			// one postgres db was not in running state in the mocks
			require.Len(t, targetGroup.Targets, 2)

			for i, labelSet := range []model.LabelSet{
				{
					"__meta_stackit_role":                 model.LabelValue(RolePostgresFlex),
					"__meta_stackit_postgres_flex_id":     model.LabelValue("a0e9a075-e485-41ad-b62e-d1c4706a33da"),
					"__meta_stackit_project":              model.LabelValue(testProjectID),
					"__meta_stackit_postgres_flex_name":   model.LabelValue("instance-1"),
					"__meta_stackit_postgres_flex_status": model.LabelValue("Ready"),
					"__meta_stackit_postgres_flex_region": model.LabelValue("eu01"),
					"__address__":                         model.LabelValue("postgres-prom-proxy.api.stackit.cloud"),
					"__metrics_path__":                    model.LabelValue("/v2/projects/00000000-0000-0000-0000-000000000000/regions/eu01/instances/a0e9a075-e485-41ad-b62e-d1c4706a33da/metrics"),
					"instance":                            model.LabelValue("a0e9a075-e485-41ad-b62e-d1c4706a33da"),
				},
			} {
				t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
					require.Equal(t, labelSet, targetGroup.Targets[i])
				})
			}
		})
	}
}
