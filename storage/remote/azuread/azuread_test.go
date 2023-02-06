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
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type AzureAdTestSuite struct {
	suite.Suite
	mockCredential *MockCredential
}

func (ad *AzureAdTestSuite) BeforeTest(suiteName, testName string) {
	ad.mockCredential = new(MockCredential)
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

	azureAdConfig := &AzureAdConfig{
		Cloud:         "AzurePublic",
		AzureClientId: dummyClientId,
	}

	ad.mockCredential.On("GetToken", mock.Anything, mock.Anything).Return(*testToken, nil)

	tokenProvider, err := NewTokenProvider(context.Background(), azureAdConfig, ad.mockCredential)
	ad.Assert().NoError(err)

	rt := &azureAdRoundTripper{
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
