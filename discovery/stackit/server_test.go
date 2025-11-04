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
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
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
			suite := &serverSDTestSuite{}
			suite.SetupTest(t)
			defer suite.Mock.ShutdownServer()

			tc.cfg.Endpoint = suite.Mock.Endpoint()
			tc.cfg.tokenURL = suite.Mock.Endpoint() + "token"
			tc.cfg.Project = testProjectID

			d, err := newServerDiscovery(&tc.cfg, promslog.NewNopLogger())
			require.NoError(t, err)

			targetGroups, err := d.refresh(context.Background())
			require.NoError(t, err)
			require.Len(t, targetGroups, 1)

			targetGroup := targetGroups[0]
			require.NotNil(t, targetGroup, "targetGroup should not be nil")
			require.NotNil(t, targetGroup.Targets, "targetGroup.targets should not be nil")
			require.Len(t, targetGroup.Targets, 1)

			for i, labelSet := range []model.LabelSet{
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
			} {
				require.Equal(t, labelSet, targetGroup.Targets[i])
			}
		})
	}
}
