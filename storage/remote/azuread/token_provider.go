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
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// tokenProvider is used to store and retrieve Azure AD accessToken
type tokenProvider struct {
	// token is member used to store the current valid accessToken
	token string
	ctx   context.Context
	// refreshDuration is used to store the refresh time duration of the current valid accessToken
	refreshDuration time.Duration
	// credentialClient is the Azure AD credential client that is being used to retrive accessToken
	credentialClient azcore.TokenCredential
	options          *policy.TokenRequestOptions
}

// TokenProvider is the interface for Credential client
type TokenProvider interface {
	GetAccessToken() string
}

// NewTokenProvider helps to fetch accessToken for different types of credential. This also takes care of
// refreshing the accessToken before expiry. This accessToken is attached to the Authorization header while making requests.
func NewTokenProvider(ctx context.Context, cfg *AzureAdConfig, cred azcore.TokenCredential) (TokenProvider, error) {
	audience, err := getAudience(cfg.Cloud)
	if err != nil {
		return nil, err
	}

	tokenProvider := &tokenProvider{
		ctx:              ctx,
		credentialClient: cred,
		options:          &policy.TokenRequestOptions{Scopes: []string{audience}},
	}

	err = tokenProvider.setAccessToken()
	if err != nil {
		return nil, errors.New("Failed to get access token: " + err.Error())
	}

	go tokenProvider.periodicallyRefreshClientToken()
	return tokenProvider, nil
}

// Returns the current valid accessToken
func (tokenProvider *tokenProvider) GetAccessToken() string {
	return tokenProvider.token
}

// Retrieves a new accessToken and stores the newly retrieved token in the tokenProvider
func (tokenProvider *tokenProvider) setAccessToken() error {
	accessToken, err := tokenProvider.credentialClient.GetToken(tokenProvider.ctx, *tokenProvider.options)
	if err != nil {
		return err
	}

	tokenProvider.setToken(accessToken.Token)
	tokenProvider.updateRefreshDuration(accessToken)
	return nil
}

// Periodically looks at the token refreshDuration and calls setAccessToken when it is past the refreshDuration
func (tokenProvider *tokenProvider) periodicallyRefreshClientToken() error {
	for {
		select {
		case <-tokenProvider.ctx.Done():
			return nil
		case <-time.After(tokenProvider.refreshDuration):
			err := tokenProvider.setAccessToken()
			if err != nil {
				return errors.New("Failed to refresh token: " + err.Error())
			}
		}
	}
}

// Handles storing of accessToken value in tokenProvider
func (tokenProvider *tokenProvider) setToken(token string) {
	var V atomic.Value
	V.Store(token)
	tokenProvider.token = V.Load().(string)
}

// Handles logic to set refreshDuration. The refreshDuration is set at half the duration of the actual token expiry.
func (tokenProvider *tokenProvider) updateRefreshDuration(accessToken azcore.AccessToken) error {
	if len(accessToken.Token) == 0 {
		return errors.New("Access Token is empty")
	}
	tokenExpiryTimestamp := accessToken.ExpiresOn.UTC()
	deltaExpirytime := time.Now().Add(tokenExpiryTimestamp.Sub(time.Now()) / 2)
	if deltaExpirytime.After(time.Now().UTC()) {
		tokenProvider.refreshDuration = deltaExpirytime.Sub(time.Now().UTC())
	} else {
		return errors.New("Access Token expiry is less than the current time")
	}

	return nil
}

// Returns audiences for different clouds
func getAudience(cloud string) (string, error) {
	switch strings.ToLower(cloud) {
	case strings.ToLower(AzureChina):
		return INGESTION_CHINA_AUDIENCE, nil
	case strings.ToLower(AzureGovernment):
		return INGESTION_GOVERNMENT_AUDIENCE, nil
	case strings.ToLower(AzurePublic):
		return INGESTION_PUBLIC_AUDIENCE, nil
	default:
		return "", errors.New("Cloud is not specified or is incorrect: " + cloud)
	}
}
