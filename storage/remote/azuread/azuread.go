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
)

// Audiences.
const (
	IngestionChinaAudience      = "https://monitor.azure.cn//.default"
	IngestionGovernmentAudience = "https://monitor.azure.us//.default"
	IngestionPublicAudience     = "https://monitor.azure.com//.default"
)

// Default paths
const (
	// DefaultWorkloadIdentityTokenPath is the default path where the Azure Workload Identity
	// webhook projects the service account token. This path is automatically configured
	// by the webhook when a pod has the label azure.workload.identity/use: "true"
	DefaultWorkloadIdentityTokenPath = "/var/run/secrets/azure/tokens/azure-identity-token"
)

// ManagedIdentityConfig is used to store managed identity config values.
type ManagedIdentityConfig struct {
	// ClientID is the clientId of the managed identity that is being used to authenticate.
	ClientID string `yaml:"client_id,omitempty"`
}

// WorkloadIdentityConfig is used to store workload identity config values.
type WorkloadIdentityConfig struct {
	// ClientID is the clientId of the Microsoft Entra application or user-assigned managed identity.
	// This should match the azure.workload.identity/client-id annotation on the Kubernetes service account.
	ClientID string `yaml:"client_id,omitempty"`
	
	// TenantID is the tenantId of the Microsoft Entra application or user-assigned managed identity.
	// This should match the tenant ID where your application or managed identity is registered.
	TenantID string `yaml:"tenant_id,omitempty"`
	
	// TokenFilePath is the path to the token file provided by the Kubernetes service account projected volume.
	// If not specified, it defaults to DefaultWorkloadIdentityTokenPath.
	// 
	// Note: The token file is automatically mounted by the Azure Workload Identity webhook
	// when the pod has the label azure.workload.identity/use: "true". This field only needs
	// to be specified if you have a non-standard setup or the webhook is mounting the token
	// in a custom location.
	TokenFilePath string `yaml:"token_file_path,omitempty"`
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

	// WorkloadIdentity is the workload identity that is being used to authenticate.
	WorkloadIdentity *WorkloadIdentityConfig `yaml:"workload_identity,omitempty"`

	// OAuth is the oauth config that is being used to authenticate.
	OAuth *OAuthConfig `yaml:"oauth,omitempty"`

	// SDK is the SDK config that is being used to authenticate.
	SDK *SDKConfig `yaml:"sdk,omitempty"`

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
		return errors.New("must provide a cloud in the Azure AD config")
	}

	if c.ManagedIdentity == nil && c.WorkloadIdentity == nil && c.OAuth == nil && c.SDK == nil {
		return errors.New("must provide an Azure Managed Identity, Azure Workload Identity, Azure OAuth or Azure SDK in the Azure AD config")
	}

	// Check for mutually exclusive configurations
	authenticators := 0
	if c.ManagedIdentity != nil {
		authenticators++
	}
	if c.WorkloadIdentity != nil {
		authenticators++
	}
	if c.OAuth != nil {
		authenticators++
	}
	if c.SDK != nil {
		authenticators++
	}
	
	if authenticators > 1 {
		return errors.New("cannot provide multiple authentication methods in the Azure AD config")
	}

	if c.ManagedIdentity != nil {
		if c.ManagedIdentity.ClientID != "" {
			_, err := uuid.Parse(c.ManagedIdentity.ClientID)
			if err != nil {
				return errors.New("the provided Azure Managed Identity client_id is invalid")
			}
		}
	}
	
	if c.WorkloadIdentity != nil {
		if c.WorkloadIdentity.ClientID == "" {
			return errors.New("must provide an Azure Workload Identity client_id in the Azure AD config")
		}
		
		if c.WorkloadIdentity.TenantID == "" {
			return errors.New("must provide an Azure Workload Identity tenant_id in the Azure AD config")
		}
		
		// Validate client ID format
		_, err := uuid.Parse(c.WorkloadIdentity.ClientID)
		if err != nil {
			return errors.New("the provided Azure Workload Identity client_id is invalid")
		}
		
		// Validate tenant ID format
		_, err = regexp.MatchString("^[0-9a-zA-Z-.]+$", c.WorkloadIdentity.TenantID)
		if err != nil {
			return errors.New("the provided Azure Workload Identity tenant_id is invalid")
		}
		
		// Set default token file path when not specified - this matches the path used
		// by the Azure Workload Identity webhook
		if c.WorkloadIdentity.TokenFilePath == "" {
			c.WorkloadIdentity.TokenFilePath = DefaultWorkloadIdentityTokenPath
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

// newTokenCredential returns a TokenCredential of different kinds like Azure Managed Identity, Workload Identity and Azure AD application.
func newTokenCredential(cfg *AzureADConfig) (azcore.TokenCredential, error) {
	var cred azcore.TokenCredential
	var err error
	cloudConfiguration, err := getCloudConfiguration(cfg.Cloud)
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

	if cfg.WorkloadIdentity != nil {
		workloadIdentityConfig := &WorkloadIdentityConfig{
			ClientID:      cfg.WorkloadIdentity.ClientID,
			TenantID:      cfg.WorkloadIdentity.TenantID,
			TokenFilePath: cfg.WorkloadIdentity.TokenFilePath,
		}
		cred, err = newWorkloadIdentityTokenCredential(clientOpts, workloadIdentityConfig)
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
	var opts *azidentity.ManagedIdentityCredentialOptions
	if managedIdentityConfig.ClientID != "" {
		clientID := azidentity.ClientID(managedIdentityConfig.ClientID)
		opts = &azidentity.ManagedIdentityCredentialOptions{ClientOptions: *clientOpts, ID: clientID}
	} else {
		opts = &azidentity.ManagedIdentityCredentialOptions{ClientOptions: *clientOpts}
	}
	return azidentity.NewManagedIdentityCredential(opts)
}

// newWorkloadIdentityTokenCredential returns new Microsoft Entra Workload Identity token credential.
// This is used for Kubernetes Pods that have been configured with the azure.workload.identity/use: "true" label
// and a service account annotated with azure.workload.identity/client-id annotation.
// 
// For this to work properly, the Kubernetes pod must:
// 1. Have the label azure.workload.identity/use: "true"
// 2. Use a service account with annotation azure.workload.identity/client-id matching the ClientID
// 3. Have the Azure Workload Identity webhook installed in the cluster
// 4. Have federated identity credentials established between the identity and service account
//
// The token file path (typically DefaultWorkloadIdentityTokenPath) is mounted automatically
// by the Azure Workload Identity webhook when it mutates the pod. The webhook:
// - Projects the service account token to this path
// - Sets environment variables like AZURE_AUTHORITY_HOST and AZURE_FEDERATED_TOKEN_FILE
// - Adds the necessary volume mounts to the pod
//
// For detailed setup instructions, see: 
// https://learn.microsoft.com/en-us/azure/azure-monitor/essentials/prometheus-metrics-enable-workload-identity
func newWorkloadIdentityTokenCredential(clientOpts *azcore.ClientOptions, workloadIdentityConfig *WorkloadIdentityConfig) (azcore.TokenCredential, error) {
	opts := &azidentity.WorkloadIdentityCredentialOptions{
		ClientOptions: *clientOpts,
		ClientID:      workloadIdentityConfig.ClientID,
		TenantID:      workloadIdentityConfig.TenantID, 
		TokenFilePath: workloadIdentityConfig.TokenFilePath,
	}
	
	return azidentity.NewWorkloadIdentityCredential(opts)
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
	if !deltaExpirytime.After(time.Now().UTC()) {
		return errors.New("access token expiry is less than the current time")
	}
	tokenProvider.refreshTime = deltaExpirytime
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

// getCloudConfiguration returns the cloud Configuration which contains AAD endpoint for different clouds.
func getCloudConfiguration(c string) (cloud.Configuration, error) {
	switch strings.ToLower(c) {
	case strings.ToLower(AzureChina):
		return cloud.AzureChina, nil
	case strings.ToLower(AzureGovernment):
		return cloud.AzureGovernment, nil
	case strings.ToLower(AzurePublic):
		return cloud.AzurePublic, nil
	default:
		return cloud.Configuration{}, errors.New("Cloud is not specified or is incorrect: " + c)
	}
}
