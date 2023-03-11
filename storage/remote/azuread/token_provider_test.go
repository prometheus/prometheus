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
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	dummyAudience   = "dummyAudience"
	dummyClientID   = "00000000-0000-0000-0000-000000000000"
	testTokenString = "testTokenString"
)

var testTokenExpiry = time.Now().Add(10 * time.Second)

type TokenProviderTestSuite struct {
	suite.Suite
	mockCredential *mockCredential
}

func (s *TokenProviderTestSuite) BeforeTest(suiteName, testName string) {
	s.mockCredential = new(mockCredential)
}

func TestTokenProvider(t *testing.T) {
	suite.Run(t, new(TokenProviderTestSuite))
}

func (s *TokenProviderTestSuite) TestNewTokenProvider_NilAudience_Fail() {
	azureAdConfig := &AzureADConfig{
		Cloud:    "PublicAzure",
		ClientID: dummyClientID,
	}

	actualTokenProvider, actualErr := newTokenProvider(context.Background(), azureAdConfig, s.mockCredential)

	//assert
	s.Assert().Nil(actualTokenProvider)
	s.Assert().NotNil(actualErr)
	s.Assert().Equal("Cloud is not specified or is incorrect: "+azureAdConfig.Cloud, actualErr.Error())
}

func (s *TokenProviderTestSuite) TestNewTokenProvider_Success() {
	azureAdConfig := &AzureADConfig{
		Cloud:    "AzurePublic",
		ClientID: dummyClientID,
	}
	s.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(getToken(), nil)

	actualTokenProvider, actualErr := newTokenProvider(context.Background(), azureAdConfig, s.mockCredential)

	//assert
	s.Assert().NotNil(actualTokenProvider)
	s.Assert().Nil(actualErr)
	s.Assert().NotNil(actualTokenProvider.getAccessToken())
}

func (s *TokenProviderTestSuite) TestPeriodicTokenRefresh_Success() {
	// setup
	azureAdConfig := &AzureADConfig{
		Cloud:    "AzurePublic",
		ClientID: dummyClientID,
	}
	testToken := &azcore.AccessToken{
		Token:     testTokenString,
		ExpiresOn: testTokenExpiry,
	}

	s.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(*testToken, nil).Once().
		On("GetToken", mock.Anything, mock.Anything).Return(getToken(), nil)

	actualTokenProvider, actualErr := newTokenProvider(context.Background(), azureAdConfig, s.mockCredential)

	// assert
	s.Assert().NotNil(actualTokenProvider)
	s.Assert().Nil(actualErr)
	s.Assert().NotNil(actualTokenProvider.getAccessToken())

	// Token set to refresh at half of the expiry time. The test tokens are set to expiry in 10s.
	// Hence, the 6 seconds wait to check if the token is refreshed.
	time.Sleep(6 * time.Second)

	s.mockCredential.AssertNumberOfCalls(s.T(), "GetToken", 2)
	accessToken, err := actualTokenProvider.getAccessToken()
	s.Assert().Nil(err)
	s.Assert().NotEqual(accessToken, testTokenString)
}

func getToken() azcore.AccessToken {
	return azcore.AccessToken{
		Token:     uuid.New().String(),
		ExpiresOn: time.Now().Add(10 * time.Second),
	}
}
