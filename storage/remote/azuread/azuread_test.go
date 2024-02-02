// Copyright 2023 The Prometheus Authors
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

package azuread

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"
)

const (
	dummyAudience     = "dummyAudience"
	dummyClientID     = "00000000-0000-0000-0000-000000000000"
	dummyClientSecret = "Cl1ent$ecret!"
	dummyTenantID     = "00000000-a12b-3cd4-e56f-000000000000"
	testTokenString   = "testTokenString"
)

var testTokenExpiry = time.Now().Add(5 * time.Second)

type AzureAdTestSuite struct {
	suite.Suite
	mockCredential *mockCredential
}

type TokenProviderTestSuite struct {
	suite.Suite
	mockCredential *mockCredential
}

// mockCredential mocks azidentity TokenCredential interface.
type mockCredential struct {
	mock.Mock
}

func (ad *AzureAdTestSuite) BeforeTest(_, _ string) {
	ad.mockCredential = new(mockCredential)
}

func TestAzureAd(t *testing.T) {
	suite.Run(t, new(AzureAdTestSuite))
}

func (ad *AzureAdTestSuite) TestAzureAdRoundTripper() {
	cases := []struct {
		cfg *AzureADConfig
	}{
		// AzureAd roundtripper with Managedidentity.
		{
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				ManagedIdentity: &ManagedIdentityConfig{
					ClientID: dummyClientID,
				},
			},
		},
		// AzureAd roundtripper with OAuth.
		{
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				OAuth: &OAuthConfig{
					ClientID:     dummyClientID,
					ClientSecret: dummyClientSecret,
					TenantID:     dummyTenantID,
				},
			},
		},
	}
	for _, c := range cases {
		var gotReq *http.Request

		testToken := &azcore.AccessToken{
			Token:     testTokenString,
			ExpiresOn: testTokenExpiry,
		}

		ad.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(*testToken, nil)

		tokenProvider, err := newTokenProvider(c.cfg, ad.mockCredential)
		ad.Require().NoError(err)

		rt := &azureADRoundTripper{
			next: promhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				gotReq = req
				return &http.Response{StatusCode: http.StatusOK}, nil
			}),
			tokenProvider: tokenProvider,
		}

		cli := &http.Client{Transport: rt}

		req, err := http.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("Hello, world!"))
		ad.Require().NoError(err)

		_, err = cli.Do(req)
		ad.Require().NoError(err)
		ad.NotNil(gotReq)

		origReq := gotReq
		ad.NotEmpty(origReq.Header.Get("Authorization"))
		ad.Equal("Bearer "+testTokenString, origReq.Header.Get("Authorization"))
	}
}

func loadAzureAdConfig(filename string) (*AzureADConfig, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg := AzureADConfig{}
	if err = yaml.UnmarshalStrict(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func TestAzureAdConfig(t *testing.T) {
	cases := []struct {
		filename string
		err      string
	}{
		// Missing managedidentiy or oauth field.
		{
			filename: "testdata/azuread_bad_configmissing.yaml",
			err:      "must provide an Azure Managed Identity or Azure OAuth in the Azure AD config",
		},
		// Invalid managedidentity client id.
		{
			filename: "testdata/azuread_bad_invalidclientid.yaml",
			err:      "the provided Azure Managed Identity client_id is invalid",
		},
		// Missing tenant id in oauth config.
		{
			filename: "testdata/azuread_bad_invalidoauthconfig.yaml",
			err:      "must provide an Azure OAuth tenant_id in the Azure AD config",
		},
		// Invalid config when both managedidentity and oauth is provided.
		{
			filename: "testdata/azuread_bad_twoconfig.yaml",
			err:      "cannot provide both Azure Managed Identity and Azure OAuth in the Azure AD config",
		},
		// Valid config with missing  optionally cloud field.
		{
			filename: "testdata/azuread_good_cloudmissing.yaml",
		},
		// Valid managed identity config.
		{
			filename: "testdata/azuread_good_managedidentity.yaml",
		},
		// Valid Oauth config.
		{
			filename: "testdata/azuread_good_oauth.yaml",
		},
	}
	for _, c := range cases {
		_, err := loadAzureAdConfig(c.filename)
		if c.err != "" {
			if err == nil {
				t.Fatalf("Did not receive expected error unmarshaling bad azuread config")
			}
			require.EqualError(t, err, c.err)
		} else {
			require.NoError(t, err)
		}
	}
}

func (m *mockCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	args := m.MethodCalled("GetToken", ctx, options)
	if args.Get(0) == nil {
		return azcore.AccessToken{}, args.Error(1)
	}

	return args.Get(0).(azcore.AccessToken), nil
}

func (s *TokenProviderTestSuite) BeforeTest(_, _ string) {
	s.mockCredential = new(mockCredential)
}

func TestTokenProvider(t *testing.T) {
	suite.Run(t, new(TokenProviderTestSuite))
}

func (s *TokenProviderTestSuite) TestNewTokenProvider() {
	cases := []struct {
		cfg *AzureADConfig
		err string
	}{
		// Invalid tokenProvider for managedidentity.
		{
			cfg: &AzureADConfig{
				Cloud: "PublicAzure",
				ManagedIdentity: &ManagedIdentityConfig{
					ClientID: dummyClientID,
				},
			},
			err: "Cloud is not specified or is incorrect: ",
		},
		// Invalid tokenProvider for oauth.
		{
			cfg: &AzureADConfig{
				Cloud: "PublicAzure",
				OAuth: &OAuthConfig{
					ClientID:     dummyClientID,
					ClientSecret: dummyClientSecret,
					TenantID:     dummyTenantID,
				},
			},
			err: "Cloud is not specified or is incorrect: ",
		},
		// Valid tokenProvider for managedidentity.
		{
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				ManagedIdentity: &ManagedIdentityConfig{
					ClientID: dummyClientID,
				},
			},
		},
		// Valid tokenProvider for oauth.
		{
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				OAuth: &OAuthConfig{
					ClientID:     dummyClientID,
					ClientSecret: dummyClientSecret,
					TenantID:     dummyTenantID,
				},
			},
		},
	}
	mockGetTokenCallCounter := 1
	for _, c := range cases {
		if c.err != "" {
			actualTokenProvider, actualErr := newTokenProvider(c.cfg, s.mockCredential)

			s.Nil(actualTokenProvider)
			s.Require().Error(actualErr)
			s.Require().ErrorContains(actualErr, c.err)
		} else {
			testToken := &azcore.AccessToken{
				Token:     testTokenString,
				ExpiresOn: testTokenExpiry,
			}

			s.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(*testToken, nil).Once().
				On("GetToken", mock.Anything, mock.Anything).Return(getToken(), nil)

			actualTokenProvider, actualErr := newTokenProvider(c.cfg, s.mockCredential)

			s.NotNil(actualTokenProvider)
			s.Require().NoError(actualErr)
			s.NotNil(actualTokenProvider.getAccessToken(context.Background()))

			// Token set to refresh at half of the expiry time. The test tokens are set to expiry in 5s.
			// Hence, the 4 seconds wait to check if the token is refreshed.
			time.Sleep(4 * time.Second)

			s.NotNil(actualTokenProvider.getAccessToken(context.Background()))

			s.mockCredential.AssertNumberOfCalls(s.T(), "GetToken", 2*mockGetTokenCallCounter)
			mockGetTokenCallCounter++
			accessToken, err := actualTokenProvider.getAccessToken(context.Background())
			s.Require().NoError(err)
			s.NotEqual(testTokenString, accessToken)
		}
	}
}

func getToken() azcore.AccessToken {
	return azcore.AccessToken{
		Token:     uuid.New().String(),
		ExpiresOn: time.Now().Add(10 * time.Second),
	}
}
