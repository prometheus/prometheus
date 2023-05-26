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
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v2"
)

const (
	dummyAudience   = "dummyAudience"
	dummyClientID   = "00000000-0000-0000-0000-000000000000"
	testTokenString = "testTokenString"
)

var testTokenExpiry = time.Now().Add(10 * time.Second)

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
	var gotReq *http.Request

	testToken := &azcore.AccessToken{
		Token:     testTokenString,
		ExpiresOn: testTokenExpiry,
	}

	managedIdentityConfig := &ManagedIdentityConfig{
		ClientID: dummyClientID,
	}

	azureAdConfig := &AzureADConfig{
		Cloud:           "AzurePublic",
		ManagedIdentity: managedIdentityConfig,
	}

	ad.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(*testToken, nil)

	tokenProvider, err := newTokenProvider(azureAdConfig, ad.mockCredential)
	ad.Assert().NoError(err)

	rt := &azureADRoundTripper{
		next: promhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotReq = req
			return &http.Response{StatusCode: http.StatusOK}, nil
		}),
		tokenProvider: tokenProvider,
	}

	cli := &http.Client{Transport: rt}

	req, err := http.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("Hello, world!"))
	ad.Assert().NoError(err)

	_, err = cli.Do(req)
	ad.Assert().NoError(err)
	ad.Assert().NotNil(gotReq)

	origReq := gotReq
	ad.Assert().NotEmpty(origReq.Header.Get("Authorization"))
	ad.Assert().Equal("Bearer "+testTokenString, origReq.Header.Get("Authorization"))
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

func testGoodConfig(t *testing.T, filename string) {
	_, err := loadAzureAdConfig(filename)
	if err != nil {
		t.Fatalf("Unexpected error parsing %s: %s", filename, err)
	}
}

func TestGoodAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_good.yaml"
	testGoodConfig(t, filename)
}

func TestGoodCloudMissingAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_good_cloudmissing.yaml"
	testGoodConfig(t, filename)
}

func TestBadClientIdMissingAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_bad_clientidmissing.yaml"
	_, err := loadAzureAdConfig(filename)
	if err == nil {
		t.Fatalf("Did not receive expected error unmarshaling bad azuread config")
	}
	if !strings.Contains(err.Error(), "must provide an Azure Managed Identity in the Azure AD config") {
		t.Errorf("Received unexpected error from unmarshal of %s: %s", filename, err.Error())
	}
}

func TestBadInvalidClientIdAzureAdConfig(t *testing.T) {
	filename := "testdata/azuread_bad_invalidclientid.yaml"
	_, err := loadAzureAdConfig(filename)
	if err == nil {
		t.Fatalf("Did not receive expected error unmarshaling bad azuread config")
	}
	if !strings.Contains(err.Error(), "the provided Azure Managed Identity client_id provided is invalid") {
		t.Errorf("Received unexpected error from unmarshal of %s: %s", filename, err.Error())
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

func (s *TokenProviderTestSuite) TestNewTokenProvider_NilAudience_Fail() {
	managedIdentityConfig := &ManagedIdentityConfig{
		ClientID: dummyClientID,
	}

	azureAdConfig := &AzureADConfig{
		Cloud:           "PublicAzure",
		ManagedIdentity: managedIdentityConfig,
	}

	actualTokenProvider, actualErr := newTokenProvider(azureAdConfig, s.mockCredential)

	s.Assert().Nil(actualTokenProvider)
	s.Assert().NotNil(actualErr)
	s.Assert().Equal("Cloud is not specified or is incorrect: "+azureAdConfig.Cloud, actualErr.Error())
}

func (s *TokenProviderTestSuite) TestNewTokenProvider_Success() {
	managedIdentityConfig := &ManagedIdentityConfig{
		ClientID: dummyClientID,
	}

	azureAdConfig := &AzureADConfig{
		Cloud:           "AzurePublic",
		ManagedIdentity: managedIdentityConfig,
	}
	s.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(getToken(), nil)

	actualTokenProvider, actualErr := newTokenProvider(azureAdConfig, s.mockCredential)

	s.Assert().NotNil(actualTokenProvider)
	s.Assert().Nil(actualErr)
	s.Assert().NotNil(actualTokenProvider.getAccessToken(context.Background()))
}

func (s *TokenProviderTestSuite) TestPeriodicTokenRefresh_Success() {
	// setup
	managedIdentityConfig := &ManagedIdentityConfig{
		ClientID: dummyClientID,
	}

	azureAdConfig := &AzureADConfig{
		Cloud:           "AzurePublic",
		ManagedIdentity: managedIdentityConfig,
	}
	testToken := &azcore.AccessToken{
		Token:     testTokenString,
		ExpiresOn: testTokenExpiry,
	}

	s.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(*testToken, nil).Once().
		On("GetToken", mock.Anything, mock.Anything).Return(getToken(), nil)

	actualTokenProvider, actualErr := newTokenProvider(azureAdConfig, s.mockCredential)

	s.Assert().NotNil(actualTokenProvider)
	s.Assert().Nil(actualErr)
	s.Assert().NotNil(actualTokenProvider.getAccessToken(context.Background()))

	// Token set to refresh at half of the expiry time. The test tokens are set to expiry in 10s.
	// Hence, the 6 seconds wait to check if the token is refreshed.
	time.Sleep(6 * time.Second)

	s.Assert().NotNil(actualTokenProvider.getAccessToken(context.Background()))

	s.mockCredential.AssertNumberOfCalls(s.T(), "GetToken", 2)
	accessToken, err := actualTokenProvider.getAccessToken(context.Background())
	s.Assert().Nil(err)
	s.Assert().NotEqual(accessToken, testTokenString)
}

func getToken() azcore.AccessToken {
	return azcore.AccessToken{
		Token:     uuid.New().String(),
		ExpiresOn: time.Now().Add(10 * time.Second),
	}
}
