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
	"go.yaml.in/yaml/v2"
)

const (
	dummyAudience     = "dummyAudience"
	dummyClientID     = "00000000-0000-0000-0000-000000000000"
	dummyClientSecret = "Cl1ent$ecret!"
	dummyTenantID     = "00000000-a12b-3cd4-e56f-000000000000"
	testTokenString   = "testTokenString"
)

func testTokenExpiry() time.Time { return time.Now().Add(5 * time.Second) }

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
		// AzureAd roundtripper with ManagedIdentity.
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
		// AzureAd roundtripper with Workload Identity.
		{
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				WorkloadIdentity: &WorkloadIdentityConfig{
					ClientID:      dummyClientID,
					TenantID:      dummyTenantID,
					TokenFilePath: DefaultWorkloadIdentityTokenPath,
				},
			},
		},
	}
	for _, c := range cases {
		var gotReq *http.Request

		testToken := &azcore.AccessToken{
			Token:     testTokenString,
			ExpiresOn: testTokenExpiry(),
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
		// Missing managedidentity or oauth field.
		{
			filename: "testdata/azuread_bad_configmissing.yaml",
			err:      "must provide an Azure Managed Identity, Azure Workload Identity, Azure OAuth or Azure SDK in the Azure AD config",
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
			err:      "cannot provide multiple authentication methods in the Azure AD config",
		},
		// Invalid config when both sdk and oauth is provided.
		{
			filename: "testdata/azuread_bad_oauthsdkconfig.yaml",
			err:      "cannot provide multiple authentication methods in the Azure AD config",
		},
		// Invalid workload identity client id.
		{
			filename: "testdata/azuread_bad_workloadidentity_invalidclientid.yaml",
			err:      "the provided Azure Workload Identity client_id is invalid",
		},
		// Invalid workload identity tenant id.
		{
			filename: "testdata/azuread_bad_workloadidentity_invalidtenantid.yaml",
			err:      "the provided Azure Workload Identity tenant_id is invalid",
		},
		// Missing workload identity client id.
		{
			filename: "testdata/azuread_bad_workloadidentity_missingclientid.yaml",
			err:      "must provide an Azure Workload Identity client_id in the Azure AD config",
		},
		// Missing workload identity tenant id.
		{
			filename: "testdata/azuread_bad_workloadidentity_missingtenantid.yaml",
			err:      "must provide an Azure Workload Identity tenant_id in the Azure AD config",
		},
		// Invalid scope validation.
		{
			filename: "testdata/azuread_bad_scope_invalid.yaml",
			err:      "the provided scope contains invalid characters",
		},
		// Valid config with missing  optionally cloud field.
		{
			filename: "testdata/azuread_good_cloudmissing.yaml",
		},
		// Valid specific managed identity config.
		{
			filename: "testdata/azuread_good_specificmanagedidentity.yaml",
		},
		// Valid default managed identity config.
		{
			filename: "testdata/azuread_good_defaultmanagedidentity.yaml",
		},
		// Valid Oauth config.
		{
			filename: "testdata/azuread_good_oauth.yaml",
		},
		// Valid SDK config.
		{
			filename: "testdata/azuread_good_sdk.yaml",
		},
		// Valid workload identity config.
		{
			filename: "testdata/azuread_good_workloadidentity.yaml",
		},
		// Valid OAuth config with custom scope.
		{
			filename: "testdata/azuread_good_oauth_customscope.yaml",
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
		// Invalid tokenProvider for SDK.
		{
			cfg: &AzureADConfig{
				Cloud: "PublicAzure",
				SDK: &SDKConfig{
					TenantID: dummyTenantID,
				},
			},
			err: "Cloud is not specified or is incorrect: ",
		},
		// Invalid tokenProvider for workload identity.
		{
			cfg: &AzureADConfig{
				Cloud: "PublicAzure",
				WorkloadIdentity: &WorkloadIdentityConfig{
					ClientID:      dummyClientID,
					TenantID:      dummyTenantID,
					TokenFilePath: DefaultWorkloadIdentityTokenPath,
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
		// Valid tokenProvider for SDK.
		{
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				SDK: &SDKConfig{
					TenantID: dummyTenantID,
				},
			},
		},
		// Valid tokenProvider for workload identity.
		{
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				WorkloadIdentity: &WorkloadIdentityConfig{
					ClientID:      dummyClientID,
					TenantID:      dummyTenantID,
					TokenFilePath: DefaultWorkloadIdentityTokenPath,
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
				ExpiresOn: testTokenExpiry(),
			}

			s.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(*testToken, nil).Once().
				On("GetToken", mock.Anything, mock.Anything).Return(getToken(), nil).Once()

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

func TestCustomScopeSupport(t *testing.T) {
	mockCredential := new(mockCredential)
	testToken := &azcore.AccessToken{
		Token:     testTokenString,
		ExpiresOn: testTokenExpiry(),
	}

	cases := []struct {
		name          string
		cfg           *AzureADConfig
		expectedScope string
	}{
		{
			name: "Custom scope with OAuth",
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				OAuth: &OAuthConfig{
					ClientID:     dummyClientID,
					ClientSecret: dummyClientSecret,
					TenantID:     dummyTenantID,
				},
				Scope: "https://custom-app.com/.default",
			},
			expectedScope: "https://custom-app.com/.default",
		},
		{
			name: "Custom scope with Managed Identity",
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				ManagedIdentity: &ManagedIdentityConfig{
					ClientID: dummyClientID,
				},
				Scope: "https://monitor.azure.com//.default",
			},
			expectedScope: "https://monitor.azure.com//.default",
		},
		{
			name: "Default scope fallback with OAuth",
			cfg: &AzureADConfig{
				Cloud: "AzurePublic",
				OAuth: &OAuthConfig{
					ClientID:     dummyClientID,
					ClientSecret: dummyClientSecret,
					TenantID:     dummyTenantID,
				},
			},
			expectedScope: IngestionPublicAudience,
		},
		{
			name: "Default scope fallback with China cloud",
			cfg: &AzureADConfig{
				Cloud: "AzureChina",
				OAuth: &OAuthConfig{
					ClientID:     dummyClientID,
					ClientSecret: dummyClientSecret,
					TenantID:     dummyTenantID,
				},
			},
			expectedScope: IngestionChinaAudience,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Set up mock to capture the actual scopes used
			mockCredential.On("GetToken", mock.Anything, mock.MatchedBy(func(options policy.TokenRequestOptions) bool {
				return len(options.Scopes) == 1 && options.Scopes[0] == c.expectedScope
			})).Return(*testToken, nil).Once()

			tokenProvider, err := newTokenProvider(c.cfg, mockCredential)
			require.NoError(t, err)
			require.NotNil(t, tokenProvider)

			// Verify that the token provider uses the expected scope
			token, err := tokenProvider.getAccessToken(context.Background())
			require.NoError(t, err)
			require.Equal(t, testTokenString, token)

			// Reset mock for next test
			mockCredential.ExpectedCalls = nil
		})
	}
}
