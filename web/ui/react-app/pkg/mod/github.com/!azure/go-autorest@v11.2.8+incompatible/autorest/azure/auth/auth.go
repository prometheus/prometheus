package auth

// Copyright 2017 Microsoft Corporation
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"unicode/utf16"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/azure/cli"
	"github.com/dimchansky/utfbom"
	"golang.org/x/crypto/pkcs12"
)

// NewAuthorizerFromEnvironment creates an Authorizer configured from environment variables in the order:
// 1. Client credentials
// 2. Client certificate
// 3. Username password
// 4. MSI
func NewAuthorizerFromEnvironment() (autorest.Authorizer, error) {
	settings, err := getAuthenticationSettings()
	if err != nil {
		return nil, err
	}

	if settings.resource == "" {
		settings.resource = settings.environment.ResourceManagerEndpoint
	}

	return settings.getAuthorizer()
}

// NewAuthorizerFromEnvironmentWithResource creates an Authorizer configured from environment variables in the order:
// 1. Client credentials
// 2. Client certificate
// 3. Username password
// 4. MSI
func NewAuthorizerFromEnvironmentWithResource(resource string) (autorest.Authorizer, error) {
	settings, err := getAuthenticationSettings()
	if err != nil {
		return nil, err
	}
	settings.resource = resource
	return settings.getAuthorizer()
}

type settings struct {
	tenantID            string
	clientID            string
	clientSecret        string
	certificatePath     string
	certificatePassword string
	username            string
	password            string
	envName             string
	resource            string
	environment         azure.Environment
}

func getAuthenticationSettings() (s settings, err error) {
	s = settings{
		tenantID:            os.Getenv("AZURE_TENANT_ID"),
		clientID:            os.Getenv("AZURE_CLIENT_ID"),
		clientSecret:        os.Getenv("AZURE_CLIENT_SECRET"),
		certificatePath:     os.Getenv("AZURE_CERTIFICATE_PATH"),
		certificatePassword: os.Getenv("AZURE_CERTIFICATE_PASSWORD"),
		username:            os.Getenv("AZURE_USERNAME"),
		password:            os.Getenv("AZURE_PASSWORD"),
		envName:             os.Getenv("AZURE_ENVIRONMENT"),
		resource:            os.Getenv("AZURE_AD_RESOURCE"),
	}

	if s.envName == "" {
		s.environment = azure.PublicCloud
	} else {
		s.environment, err = azure.EnvironmentFromName(s.envName)
	}
	return
}

func (settings settings) getAuthorizer() (autorest.Authorizer, error) {
	//1.Client Credentials
	if settings.clientSecret != "" {
		config := NewClientCredentialsConfig(settings.clientID, settings.clientSecret, settings.tenantID)
		config.AADEndpoint = settings.environment.ActiveDirectoryEndpoint
		config.Resource = settings.resource
		return config.Authorizer()
	}

	//2. Client Certificate
	if settings.certificatePath != "" {
		config := NewClientCertificateConfig(settings.certificatePath, settings.certificatePassword, settings.clientID, settings.tenantID)
		config.AADEndpoint = settings.environment.ActiveDirectoryEndpoint
		config.Resource = settings.resource
		return config.Authorizer()
	}

	//3. Username Password
	if settings.username != "" && settings.password != "" {
		config := NewUsernamePasswordConfig(settings.username, settings.password, settings.clientID, settings.tenantID)
		config.AADEndpoint = settings.environment.ActiveDirectoryEndpoint
		config.Resource = settings.resource
		return config.Authorizer()
	}

	// 4. MSI
	config := NewMSIConfig()
	config.Resource = settings.resource
	config.ClientID = settings.clientID
	return config.Authorizer()
}

// NewAuthorizerFromFile creates an Authorizer configured from a configuration file.
func NewAuthorizerFromFile(baseURI string) (autorest.Authorizer, error) {
	file, err := getAuthFile()
	if err != nil {
		return nil, err
	}

	resource, err := getResourceForToken(*file, baseURI)
	if err != nil {
		return nil, err
	}
	return NewAuthorizerFromFileWithResource(resource)
}

// NewAuthorizerFromFileWithResource creates an Authorizer configured from a configuration file.
func NewAuthorizerFromFileWithResource(resource string) (autorest.Authorizer, error) {
	file, err := getAuthFile()
	if err != nil {
		return nil, err
	}

	config, err := adal.NewOAuthConfig(file.ActiveDirectoryEndpoint, file.TenantID)
	if err != nil {
		return nil, err
	}

	spToken, err := adal.NewServicePrincipalToken(*config, file.ClientID, file.ClientSecret, resource)
	if err != nil {
		return nil, err
	}

	return autorest.NewBearerAuthorizer(spToken), nil
}

// NewAuthorizerFromCLI creates an Authorizer configured from Azure CLI 2.0 for local development scenarios.
func NewAuthorizerFromCLI() (autorest.Authorizer, error) {
	settings, err := getAuthenticationSettings()
	if err != nil {
		return nil, err
	}

	if settings.resource == "" {
		settings.resource = settings.environment.ResourceManagerEndpoint
	}

	return NewAuthorizerFromCLIWithResource(settings.resource)
}

// NewAuthorizerFromCLIWithResource creates an Authorizer configured from Azure CLI 2.0 for local development scenarios.
func NewAuthorizerFromCLIWithResource(resource string) (autorest.Authorizer, error) {
	token, err := cli.GetTokenFromCLI(resource)
	if err != nil {
		return nil, err
	}

	adalToken, err := token.ToADALToken()
	if err != nil {
		return nil, err
	}

	return autorest.NewBearerAuthorizer(&adalToken), nil
}

func getAuthFile() (*file, error) {
	fileLocation := os.Getenv("AZURE_AUTH_LOCATION")
	if fileLocation == "" {
		return nil, errors.New("environment variable AZURE_AUTH_LOCATION is not set")
	}

	contents, err := ioutil.ReadFile(fileLocation)
	if err != nil {
		return nil, err
	}

	// Auth file might be encoded
	decoded, err := decode(contents)
	if err != nil {
		return nil, err
	}

	authFile := file{}
	err = json.Unmarshal(decoded, &authFile)
	if err != nil {
		return nil, err
	}

	return &authFile, nil
}

// File represents the authentication file
type file struct {
	ClientID                string `json:"clientId,omitempty"`
	ClientSecret            string `json:"clientSecret,omitempty"`
	SubscriptionID          string `json:"subscriptionId,omitempty"`
	TenantID                string `json:"tenantId,omitempty"`
	ActiveDirectoryEndpoint string `json:"activeDirectoryEndpointUrl,omitempty"`
	ResourceManagerEndpoint string `json:"resourceManagerEndpointUrl,omitempty"`
	GraphResourceID         string `json:"activeDirectoryGraphResourceId,omitempty"`
	SQLManagementEndpoint   string `json:"sqlManagementEndpointUrl,omitempty"`
	GalleryEndpoint         string `json:"galleryEndpointUrl,omitempty"`
	ManagementEndpoint      string `json:"managementEndpointUrl,omitempty"`
}

func decode(b []byte) ([]byte, error) {
	reader, enc := utfbom.Skip(bytes.NewReader(b))

	switch enc {
	case utfbom.UTF16LittleEndian:
		u16 := make([]uint16, (len(b)/2)-1)
		err := binary.Read(reader, binary.LittleEndian, &u16)
		if err != nil {
			return nil, err
		}
		return []byte(string(utf16.Decode(u16))), nil
	case utfbom.UTF16BigEndian:
		u16 := make([]uint16, (len(b)/2)-1)
		err := binary.Read(reader, binary.BigEndian, &u16)
		if err != nil {
			return nil, err
		}
		return []byte(string(utf16.Decode(u16))), nil
	}
	return ioutil.ReadAll(reader)
}

func getResourceForToken(f file, baseURI string) (string, error) {
	// Compare dafault base URI from the SDK to the endpoints from the public cloud
	// Base URI and token resource are the same string. This func finds the authentication
	// file field that matches the SDK base URI. The SDK defines the public cloud
	// endpoint as its default base URI
	if !strings.HasSuffix(baseURI, "/") {
		baseURI += "/"
	}
	switch baseURI {
	case azure.PublicCloud.ServiceManagementEndpoint:
		return f.ManagementEndpoint, nil
	case azure.PublicCloud.ResourceManagerEndpoint:
		return f.ResourceManagerEndpoint, nil
	case azure.PublicCloud.ActiveDirectoryEndpoint:
		return f.ActiveDirectoryEndpoint, nil
	case azure.PublicCloud.GalleryEndpoint:
		return f.GalleryEndpoint, nil
	case azure.PublicCloud.GraphEndpoint:
		return f.GraphResourceID, nil
	}
	return "", fmt.Errorf("auth: base URI not found in endpoints")
}

// NewClientCredentialsConfig creates an AuthorizerConfig object configured to obtain an Authorizer through Client Credentials.
// Defaults to Public Cloud and Resource Manager Endpoint.
func NewClientCredentialsConfig(clientID string, clientSecret string, tenantID string) ClientCredentialsConfig {
	return ClientCredentialsConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TenantID:     tenantID,
		Resource:     azure.PublicCloud.ResourceManagerEndpoint,
		AADEndpoint:  azure.PublicCloud.ActiveDirectoryEndpoint,
	}
}

// NewClientCertificateConfig creates a ClientCertificateConfig object configured to obtain an Authorizer through client certificate.
// Defaults to Public Cloud and Resource Manager Endpoint.
func NewClientCertificateConfig(certificatePath string, certificatePassword string, clientID string, tenantID string) ClientCertificateConfig {
	return ClientCertificateConfig{
		CertificatePath:     certificatePath,
		CertificatePassword: certificatePassword,
		ClientID:            clientID,
		TenantID:            tenantID,
		Resource:            azure.PublicCloud.ResourceManagerEndpoint,
		AADEndpoint:         azure.PublicCloud.ActiveDirectoryEndpoint,
	}
}

// NewUsernamePasswordConfig creates an UsernamePasswordConfig object configured to obtain an Authorizer through username and password.
// Defaults to Public Cloud and Resource Manager Endpoint.
func NewUsernamePasswordConfig(username string, password string, clientID string, tenantID string) UsernamePasswordConfig {
	return UsernamePasswordConfig{
		Username:    username,
		Password:    password,
		ClientID:    clientID,
		TenantID:    tenantID,
		Resource:    azure.PublicCloud.ResourceManagerEndpoint,
		AADEndpoint: azure.PublicCloud.ActiveDirectoryEndpoint,
	}
}

// NewMSIConfig creates an MSIConfig object configured to obtain an Authorizer through MSI.
func NewMSIConfig() MSIConfig {
	return MSIConfig{
		Resource: azure.PublicCloud.ResourceManagerEndpoint,
	}
}

// NewDeviceFlowConfig creates a DeviceFlowConfig object configured to obtain an Authorizer through device flow.
// Defaults to Public Cloud and Resource Manager Endpoint.
func NewDeviceFlowConfig(clientID string, tenantID string) DeviceFlowConfig {
	return DeviceFlowConfig{
		ClientID:    clientID,
		TenantID:    tenantID,
		Resource:    azure.PublicCloud.ResourceManagerEndpoint,
		AADEndpoint: azure.PublicCloud.ActiveDirectoryEndpoint,
	}
}

//AuthorizerConfig provides an authorizer from the configuration provided.
type AuthorizerConfig interface {
	Authorizer() (autorest.Authorizer, error)
}

// ClientCredentialsConfig provides the options to get a bearer authorizer from client credentials.
type ClientCredentialsConfig struct {
	ClientID     string
	ClientSecret string
	TenantID     string
	AADEndpoint  string
	Resource     string
}

// Authorizer gets the authorizer from client credentials.
func (ccc ClientCredentialsConfig) Authorizer() (autorest.Authorizer, error) {
	oauthConfig, err := adal.NewOAuthConfig(ccc.AADEndpoint, ccc.TenantID)
	if err != nil {
		return nil, err
	}

	spToken, err := adal.NewServicePrincipalToken(*oauthConfig, ccc.ClientID, ccc.ClientSecret, ccc.Resource)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token from client credentials: %v", err)
	}

	return autorest.NewBearerAuthorizer(spToken), nil
}

// ClientCertificateConfig provides the options to get a bearer authorizer from a client certificate.
type ClientCertificateConfig struct {
	ClientID            string
	CertificatePath     string
	CertificatePassword string
	TenantID            string
	AADEndpoint         string
	Resource            string
}

// Authorizer gets an authorizer object from client certificate.
func (ccc ClientCertificateConfig) Authorizer() (autorest.Authorizer, error) {
	oauthConfig, err := adal.NewOAuthConfig(ccc.AADEndpoint, ccc.TenantID)

	certData, err := ioutil.ReadFile(ccc.CertificatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read the certificate file (%s): %v", ccc.CertificatePath, err)
	}

	certificate, rsaPrivateKey, err := decodePkcs12(certData, ccc.CertificatePassword)
	if err != nil {
		return nil, fmt.Errorf("failed to decode pkcs12 certificate while creating spt: %v", err)
	}

	spToken, err := adal.NewServicePrincipalTokenFromCertificate(*oauthConfig, ccc.ClientID, certificate, rsaPrivateKey, ccc.Resource)

	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token from certificate auth: %v", err)
	}

	return autorest.NewBearerAuthorizer(spToken), nil
}

// DeviceFlowConfig provides the options to get a bearer authorizer using device flow authentication.
type DeviceFlowConfig struct {
	ClientID    string
	TenantID    string
	AADEndpoint string
	Resource    string
}

// Authorizer gets the authorizer from device flow.
func (dfc DeviceFlowConfig) Authorizer() (autorest.Authorizer, error) {
	oauthClient := &autorest.Client{}
	oauthConfig, err := adal.NewOAuthConfig(dfc.AADEndpoint, dfc.TenantID)
	deviceCode, err := adal.InitiateDeviceAuth(oauthClient, *oauthConfig, dfc.ClientID, dfc.Resource)
	if err != nil {
		return nil, fmt.Errorf("failed to start device auth flow: %s", err)
	}

	log.Println(*deviceCode.Message)

	token, err := adal.WaitForUserCompletion(oauthClient, deviceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to finish device auth flow: %s", err)
	}

	spToken, err := adal.NewServicePrincipalTokenFromManualToken(*oauthConfig, dfc.ClientID, dfc.Resource, *token)
	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token from device flow: %v", err)
	}

	return autorest.NewBearerAuthorizer(spToken), nil
}

func decodePkcs12(pkcs []byte, password string) (*x509.Certificate, *rsa.PrivateKey, error) {
	privateKey, certificate, err := pkcs12.Decode(pkcs, password)
	if err != nil {
		return nil, nil, err
	}

	rsaPrivateKey, isRsaKey := privateKey.(*rsa.PrivateKey)
	if !isRsaKey {
		return nil, nil, fmt.Errorf("PKCS#12 certificate must contain an RSA private key")
	}

	return certificate, rsaPrivateKey, nil
}

// UsernamePasswordConfig provides the options to get a bearer authorizer from a username and a password.
type UsernamePasswordConfig struct {
	ClientID    string
	Username    string
	Password    string
	TenantID    string
	AADEndpoint string
	Resource    string
}

// Authorizer gets the authorizer from a username and a password.
func (ups UsernamePasswordConfig) Authorizer() (autorest.Authorizer, error) {

	oauthConfig, err := adal.NewOAuthConfig(ups.AADEndpoint, ups.TenantID)

	spToken, err := adal.NewServicePrincipalTokenFromUsernamePassword(*oauthConfig, ups.ClientID, ups.Username, ups.Password, ups.Resource)

	if err != nil {
		return nil, fmt.Errorf("failed to get oauth token from username and password auth: %v", err)
	}

	return autorest.NewBearerAuthorizer(spToken), nil
}

// MSIConfig provides the options to get a bearer authorizer through MSI.
type MSIConfig struct {
	Resource string
	ClientID string
}

// Authorizer gets the authorizer from MSI.
func (mc MSIConfig) Authorizer() (autorest.Authorizer, error) {
	msiEndpoint, err := adal.GetMSIVMEndpoint()
	if err != nil {
		return nil, err
	}

	var spToken *adal.ServicePrincipalToken
	if mc.ClientID == "" {
		spToken, err = adal.NewServicePrincipalTokenFromMSI(msiEndpoint, mc.Resource)
		if err != nil {
			return nil, fmt.Errorf("failed to get oauth token from MSI: %v", err)
		}
	} else {
		spToken, err = adal.NewServicePrincipalTokenFromMSIWithUserAssignedID(msiEndpoint, mc.Resource, mc.ClientID)
		if err != nil {
			return nil, fmt.Errorf("failed to get oauth token from MSI for user assigned identity: %v", err)
		}
	}

	return autorest.NewBearerAuthorizer(spToken), nil
}
