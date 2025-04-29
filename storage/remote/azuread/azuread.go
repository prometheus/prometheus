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
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/google/uuid"
	"github.com/grafana/regexp"
)

// Clouds.
const (
	AzureChina      = "AzureChina"
	AzureGovernment = "AzureGovernment"
	AzurePublic     = "AzurePublic"
	AzureCustom     = "AzureCustom"
)

// Audiences.
const (
	IngestionChinaAudience      = "https://monitor.azure.cn//.default"
	IngestionGovernmentAudience = "https://monitor.azure.us//.default"
	IngestionPublicAudience     = "https://monitor.azure.com//.default"
)

// ManagedIdentityConfig is used to store managed identity config values.
type ManagedIdentityConfig struct {
	// ClientID is the clientId of the managed identity that is being used to authenticate.
	ClientID string `yaml:"client_id,omitempty"`
}

// OAuthConfig is used to store azure oauth config values.
type OAuthConfig struct {
	// ClientID is the clientId of the azure active directory application that is being used to authenticate.
	ClientID string `yaml:"client_id,omitempty"`

	// ClientSecret is the clientSecret of the azure active directory application that is being used to authenticate.
	ClientSecret string `yaml:"client_secret,omitempty"`

	// TenantID is the tenantId of the azure active directory application that is being used to authenticate.
	TenantID string `yaml:"tenant_id,omitempty"`
}

// SDKConfig is used to store azure SDK config values.
type SDKConfig struct {
	// TenantID is the tenantId of the azure active directory application that is being used to authenticate.
	TenantID string `yaml:"tenant_id,omitempty"`
}

// AzureADConfig is used to store the config values.
type AzureADConfig struct { //nolint:revive // exported.
	// ManagedIdentity is the managed identity that is being used to authenticate.
	ManagedIdentity *ManagedIdentityConfig `yaml:"managed_identity,omitempty"`

	// OAuth is the oauth config that is being used to authenticate.
	OAuth *OAuthConfig `yaml:"oauth,omitempty"`

	// SDK is the SDK config that is being used to authenticate.
	SDK *SDKConfig `yaml:"sdk,omitempty"`

	// Cloud is the Azure cloud in which the service is running
	Cloud *AzureADCloudConfig `yaml:"cloud,omitempty"`
}

// AzureADCloudConfig is used to store the AAD config values.
type AzureADCloudConfig struct { //nolint:revive // exported.
	// Name is the Azure cloud in which the service is running. Example: AzurePublic/AzureGovernment/AzureChina.
	Name string `yaml:"name,omitempty"`

	// AAD Endpoint to authenticate against. Example: https://login.microsoftonline.com/
	AadEndpoint string `yaml:"aad_endpoint,omitempty"`

	// Audience to acquire token for. Example: https://monitor.azure.com/.default
	TokenAudience string `yaml:"token_audience,omitempty"`
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
	if c.Cloud == nil {
		c.Cloud = &AzureADCloudConfig{Name: AzurePublic}
	}

	if c.Cloud.Name != AzureChina && c.Cloud.Name != AzureGovernment && c.Cloud.Name != AzurePublic && c.Cloud.Name != AzureCustom {
		return errors.New("must provide a cloud in the Azure AD config")
	}

	if c.ManagedIdentity == nil && c.OAuth == nil && c.SDK == nil {
		return errors.New("must provide an Azure Managed Identity, Azure OAuth or Azure SDK in the Azure AD config")
	}

	if c.ManagedIdentity != nil && c.OAuth != nil {
		return errors.New("cannot provide both Azure Managed Identity and Azure OAuth in the Azure AD config")
	}

	if c.ManagedIdentity != nil && c.SDK != nil {
		return errors.New("cannot provide both Azure Managed Identity and Azure SDK in the Azure AD config")
	}

	if c.OAuth != nil && c.SDK != nil {
		return errors.New("cannot provide both Azure OAuth and Azure SDK in the Azure AD config")
	}

	if c.ManagedIdentity != nil {
		if c.ManagedIdentity.ClientID == "" {
			return errors.New("must provide an Azure Managed Identity client_id in the Azure AD config")
		}

		_, err := uuid.Parse(c.ManagedIdentity.ClientID)
		if err != nil {
			return errors.New("the provided Azure Managed Identity client_id is invalid")
		}
	}

	if c.OAuth != nil {
		if c.OAuth.ClientID == "" {
			return errors.New("must provide an Azure OAuth client_id in the Azure AD config")
		}
		if c.OAuth.ClientSecret == "" {
			return errors.New("must provide an Azure OAuth client_secret in the Azure AD config")
		}
		if c.OAuth.TenantID == "" {
			return errors.New("must provide an Azure OAuth tenant_id in the Azure AD config")
		}

		var err error
		_, err = uuid.Parse(c.OAuth.ClientID)
		if err != nil {
			return errors.New("the provided Azure OAuth client_id is invalid")
		}
		_, err = regexp.MatchString("^[0-9a-zA-Z-.]+$", c.OAuth.TenantID)
		if err != nil {
			return errors.New("the provided Azure OAuth tenant_id is invalid")
		}
	}

	if c.SDK != nil {
		var err error

		if c.SDK.TenantID != "" {
			_, err = regexp.MatchString("^[0-9a-zA-Z-.]+$", c.SDK.TenantID)
			if err != nil {
				return errors.New("the provided Azure OAuth tenant_id is invalid")
			}
		}
	}

	return nil
}

// Validate validates config values provided.
func (c *AzureADCloudConfig) Validate() error {
	if c.Name != AzureCustom {
		return errors.New("cannot provide cloud name other than AzureCustom in the Azure AD cloud config")
	}

	if (c.AadEndpoint != "" && c.TokenAudience == "") || (c.AadEndpoint == "" && c.TokenAudience != "") {
		return errors.New("must provide both AAD Endpoint and Token Audience when either are provided in the Azure AD cloud config")
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

// UnmarshalYAML unmarshal the Azure AD cloud config yaml.
func (c *AzureADCloudConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var name string
	if err := unmarshal(&name); err == nil {
		// Legacy format used, just set the name
		c.Name = name
		return nil
	}

	type plain AzureADCloudConfig
	*c = AzureADCloudConfig{}
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	return c.Validate()
}

// MarshalYAML marshal the Azure AD cloud config to yaml.
func (c AzureADCloudConfig) MarshalYAML() (interface{}, error) {
	if c.Name != AzureCustom {
		return c.Name, nil
	}

	return c, nil
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
	var cred azcore.TokenCredential
	var err error
	cloudConfiguration, err := cfg.getCloudConfiguration()
	if err != nil {
		return nil, err
	}
	clientOpts := &azcore.ClientOptions{
		Cloud: cloudConfiguration,
	}

	if cfg.ManagedIdentity != nil {
		managedIdentityConfig := &ManagedIdentityConfig{
			ClientID: cfg.ManagedIdentity.ClientID,
		}
		cred, err = newManagedIdentityTokenCredential(clientOpts, managedIdentityConfig)
		if err != nil {
			return nil, err
		}
	}

	if cfg.OAuth != nil {
		oAuthConfig := &OAuthConfig{
			ClientID:     cfg.OAuth.ClientID,
			ClientSecret: cfg.OAuth.ClientSecret,
			TenantID:     cfg.OAuth.TenantID,
		}
		cred, err = newOAuthTokenCredential(clientOpts, oAuthConfig)
		if err != nil {
			return nil, err
		}
	}

	if cfg.SDK != nil {
		sdkConfig := &SDKConfig{
			TenantID: cfg.SDK.TenantID,
		}
		cred, err = newSDKTokenCredential(clientOpts, sdkConfig)
		if err != nil {
			return nil, err
		}
	}

	return cred, nil
}

// newManagedIdentityTokenCredential returns new Managed Identity token credential.
func newManagedIdentityTokenCredential(clientOpts *azcore.ClientOptions, managedIdentityConfig *ManagedIdentityConfig) (azcore.TokenCredential, error) {
	clientID := azidentity.ClientID(managedIdentityConfig.ClientID)
	opts := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: *clientOpts, ID: clientID}
	return azidentity.NewManagedIdentityCredential(opts)
}

// newOAuthTokenCredential returns new OAuth token credential.
func newOAuthTokenCredential(clientOpts *azcore.ClientOptions, oAuthConfig *OAuthConfig) (azcore.TokenCredential, error) {
	opts := &azidentity.ClientSecretCredentialOptions{ClientOptions: *clientOpts}
	return azidentity.NewClientSecretCredential(oAuthConfig.TenantID, oAuthConfig.ClientID, oAuthConfig.ClientSecret, opts)
}

// newSDKTokenCredential returns new SDK token credential.
func newSDKTokenCredential(clientOpts *azcore.ClientOptions, sdkConfig *SDKConfig) (azcore.TokenCredential, error) {
	opts := &azidentity.DefaultAzureCredentialOptions{ClientOptions: *clientOpts, TenantID: sdkConfig.TenantID}
	return azidentity.NewDefaultAzureCredential(opts)
}

// newTokenProvider helps to fetch accessToken for different types of credential. This also takes care of
// refreshing the accessToken before expiry. This accessToken is attached to the Authorization header while making requests.
func newTokenProvider(cfg *AzureADConfig, cred azcore.TokenCredential) (*tokenProvider, error) {
	audience, err := cfg.getAudience()
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
	if !deltaExpirytime.After(time.Now().UTC()) {
		return errors.New("access token expiry is less than the current time")
	}
	tokenProvider.refreshTime = deltaExpirytime
	return nil
}

// getAudience returns audiences for different clouds.
func (c *AzureADConfig) getAudience() (string, error) {
	switch strings.ToLower(c.Cloud.Name) {
	case strings.ToLower(AzureChina):
		return IngestionChinaAudience, nil
	case strings.ToLower(AzureGovernment):
		return IngestionGovernmentAudience, nil
	case strings.ToLower(AzurePublic):
		return IngestionPublicAudience, nil
	case strings.ToLower(AzureCustom):
		return c.Cloud.TokenAudience, nil
	default:
		return "", errors.New("Cloud is not specified or is incorrect: " + c.Cloud.Name)
	}
}

// getCloudConfiguration returns the cloud Configuration which contains AAD endpoint for different clouds.
func (c *AzureADConfig) getCloudConfiguration() (cloud.Configuration, error) {
	switch strings.ToLower(c.Cloud.Name) {
	case strings.ToLower(AzureChina):
		return cloud.AzureChina, nil
	case strings.ToLower(AzureGovernment):
		return cloud.AzureGovernment, nil
	case strings.ToLower(AzurePublic):
		return cloud.AzurePublic, nil
	case strings.ToLower(AzureCustom):
		return cloud.Configuration{
			ActiveDirectoryAuthorityHost: c.Cloud.AadEndpoint, Services: map[cloud.ServiceName]cloud.ServiceConfiguration{},
		}, nil
	default:
		return cloud.Configuration{}, errors.New("Cloud is not specified or is incorrect: " + c.Cloud.Name)
	}
}
