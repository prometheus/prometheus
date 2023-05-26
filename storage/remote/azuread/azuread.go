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
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/google/uuid"
)

const (
	// Clouds.
	AzureChina      = "AzureChina"
	AzureGovernment = "AzureGovernment"
	AzurePublic     = "AzurePublic"

	// Audiences.
	IngestionChinaAudience      = "https://monitor.azure.cn//.default"
	IngestionGovernmentAudience = "https://monitor.azure.us//.default"
	IngestionPublicAudience     = "https://monitor.azure.com//.default"
)

// ManagedIdentityConfig is used to store managed identity config values
type ManagedIdentityConfig struct {
	// ClientID is the clientId of the managed identity that is being used to authenticate.
	ClientID string `yaml:"client_id,omitempty"`
}

// AzureADConfig is used to store the config values.
type AzureADConfig struct { // nolint:revive
	// ManagedIdentity is the managed identity that is being used to authenticate.
	ManagedIdentity *ManagedIdentityConfig `yaml:"managed_identity,omitempty"`

	// Cloud is the Azure cloud in which the service is running. Example: AzurePublic/AzureGovernment/AzureChina.
	Cloud string `yaml:"cloud,omitempty"`
}

// azureADRoundTripper is used to store the roundtripper and the tokenprovider.
type azureADRoundTripper struct {
	next          http.RoundTripper
	tokenProvider *tokenProvider
}

// tokenProvider is used to store and retrieve Azure AD accessToken.
type tokenProvider struct {
	// token is member used to store the current valid accessToken.
	token string
	// mu guards access to token.
	mu sync.Mutex
	// refreshTime is used to store the refresh time of the current valid accessToken.
	refreshTime time.Time
	// credentialClient is the Azure AD credential client that is being used to retrieve accessToken.
	credentialClient azcore.TokenCredential
	options          *policy.TokenRequestOptions
}

// Validate validates config values provided.
func (c *AzureADConfig) Validate() error {
	if c.Cloud == "" {
		c.Cloud = AzurePublic
	}

	if c.Cloud != AzureChina && c.Cloud != AzureGovernment && c.Cloud != AzurePublic {
		return fmt.Errorf("must provide a cloud in the Azure AD config")
	}

	if c.ManagedIdentity == nil {
		return fmt.Errorf("must provide an Azure Managed Identity in the Azure AD config")
	}

	if c.ManagedIdentity.ClientID == "" {
		return fmt.Errorf("must provide an Azure Managed Identity client_id in the Azure AD config")
	}

	_, err := uuid.Parse(c.ManagedIdentity.ClientID)
	if err != nil {
		return fmt.Errorf("the provided Azure Managed Identity client_id provided is invalid")
	}
	return nil
}

// UnmarshalYAML unmarshal the Azure AD config yaml.
func (c *AzureADConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain AzureADConfig
	*c = AzureADConfig{}
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	return c.Validate()
}

// NewAzureADRoundTripper creates round tripper adding Azure AD authorization to calls.
func NewAzureADRoundTripper(cfg *AzureADConfig, next http.RoundTripper) (http.RoundTripper, error) {
	if next == nil {
		next = http.DefaultTransport
	}

	cred, err := newTokenCredential(cfg)
	if err != nil {
		return nil, err
	}

	tokenProvider, err := newTokenProvider(cfg, cred)
	if err != nil {
		return nil, err
	}

	rt := &azureADRoundTripper{
		next:          next,
		tokenProvider: tokenProvider,
	}
	return rt, nil
}

// RoundTrip sets Authorization header for requests.
func (rt *azureADRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	accessToken, err := rt.tokenProvider.getAccessToken(req.Context())
	if err != nil {
		return nil, err
	}
	bearerAccessToken := "Bearer " + accessToken
	req.Header.Set("Authorization", bearerAccessToken)

	return rt.next.RoundTrip(req)
}

// newTokenCredential returns a TokenCredential of different kinds like Azure Managed Identity and Azure AD application.
func newTokenCredential(cfg *AzureADConfig) (azcore.TokenCredential, error) {
	cred, err := newManagedIdentityTokenCredential(cfg.ManagedIdentity.ClientID)
	if err != nil {
		return nil, err
	}

	return cred, nil
}

// newManagedIdentityTokenCredential returns new Managed Identity token credential.
func newManagedIdentityTokenCredential(managedIdentityClientID string) (azcore.TokenCredential, error) {
	clientID := azidentity.ClientID(managedIdentityClientID)
	opts := &azidentity.ManagedIdentityCredentialOptions{ID: clientID}
	return azidentity.NewManagedIdentityCredential(opts)
}

// newTokenProvider helps to fetch accessToken for different types of credential. This also takes care of
// refreshing the accessToken before expiry. This accessToken is attached to the Authorization header while making requests.
func newTokenProvider(cfg *AzureADConfig, cred azcore.TokenCredential) (*tokenProvider, error) {
	audience, err := getAudience(cfg.Cloud)
	if err != nil {
		return nil, err
	}

	tokenProvider := &tokenProvider{
		credentialClient: cred,
		options:          &policy.TokenRequestOptions{Scopes: []string{audience}},
	}

	return tokenProvider, nil
}

// getAccessToken returns the current valid accessToken.
func (tokenProvider *tokenProvider) getAccessToken(ctx context.Context) (string, error) {
	tokenProvider.mu.Lock()
	defer tokenProvider.mu.Unlock()
	if tokenProvider.valid() {
		return tokenProvider.token, nil
	}
	err := tokenProvider.getToken(ctx)
	if err != nil {
		return "", errors.New("Failed to get access token: " + err.Error())
	}
	return tokenProvider.token, nil
}

// valid checks if the token in the token provider is valid and not expired.
func (tokenProvider *tokenProvider) valid() bool {
	if len(tokenProvider.token) == 0 {
		return false
	}
	if tokenProvider.refreshTime.After(time.Now().UTC()) {
		return true
	}
	return false
}

// getToken retrieves a new accessToken and stores the newly retrieved token in the tokenProvider.
func (tokenProvider *tokenProvider) getToken(ctx context.Context) error {
	accessToken, err := tokenProvider.credentialClient.GetToken(ctx, *tokenProvider.options)
	if err != nil {
		return err
	}
	if len(accessToken.Token) == 0 {
		return errors.New("access token is empty")
	}

	tokenProvider.token = accessToken.Token
	err = tokenProvider.updateRefreshTime(accessToken)
	if err != nil {
		return err
	}
	return nil
}

// updateRefreshTime handles logic to set refreshTime. The refreshTime is set at half the duration of the actual token expiry.
func (tokenProvider *tokenProvider) updateRefreshTime(accessToken azcore.AccessToken) error {
	tokenExpiryTimestamp := accessToken.ExpiresOn.UTC()
	deltaExpirytime := time.Now().Add(time.Until(tokenExpiryTimestamp) / 2)
	if deltaExpirytime.After(time.Now().UTC()) {
		tokenProvider.refreshTime = deltaExpirytime
	} else {
		return errors.New("access token expiry is less than the current time")
	}
	return nil
}

// getAudience returns audiences for different clouds.
func getAudience(cloud string) (string, error) {
	switch strings.ToLower(cloud) {
	case strings.ToLower(AzureChina):
		return IngestionChinaAudience, nil
	case strings.ToLower(AzureGovernment):
		return IngestionGovernmentAudience, nil
	case strings.ToLower(AzurePublic):
		return IngestionPublicAudience, nil
	default:
		return "", errors.New("Cloud is not specified or is incorrect: " + cloud)
	}
}
