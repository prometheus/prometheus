package azure

import (
	"fmt"
	"net/url"
)

const (
	activeDirectoryAPIVersion = "1.0"
)

// Environment represents a set of endpoints for each of Azure's Clouds.
type Environment struct {
	Name                      string
	ManagementPortalURL       string
	PublishSettingsURL        string
	ServiceManagementEndpoint string
	ResourceManagerEndpoint   string
	ActiveDirectoryEndpoint   string
	GalleryEndpoint           string
	KeyVaultEndpoint          string
	GraphEndpoint             string
	StorageEndpointSuffix     string
	SQLDatabaseDNSSuffix      string
	TrafficManagerDNSSuffix   string
	KeyVaultDNSSuffix         string
	ServiceBusEndpointSuffix  string
}

var (
	// PublicCloud is the default public Azure cloud environment
	PublicCloud = Environment{
		Name:                      "AzurePublicCloud",
		ManagementPortalURL:       "https://manage.windowsazure.com/",
		PublishSettingsURL:        "https://manage.windowsazure.com/publishsettings/index",
		ServiceManagementEndpoint: "https://management.core.windows.net/",
		ResourceManagerEndpoint:   "https://management.azure.com/",
		ActiveDirectoryEndpoint:   "https://login.microsoftonline.com/",
		GalleryEndpoint:           "https://gallery.azure.com/",
		KeyVaultEndpoint:          "https://vault.azure.net/",
		GraphEndpoint:             "https://graph.windows.net/",
		StorageEndpointSuffix:     "core.windows.net",
		SQLDatabaseDNSSuffix:      "database.windows.net",
		TrafficManagerDNSSuffix:   "trafficmanager.net",
		KeyVaultDNSSuffix:         "vault.azure.net",
		ServiceBusEndpointSuffix:  "servicebus.azure.com",
	}

	// USGovernmentCloud is the cloud environment for the US Government
	USGovernmentCloud = Environment{
		Name:                      "AzureUSGovernmentCloud",
		ManagementPortalURL:       "https://manage.windowsazure.us/",
		PublishSettingsURL:        "https://manage.windowsazure.us/publishsettings/index",
		ServiceManagementEndpoint: "https://management.core.usgovcloudapi.net/",
		ResourceManagerEndpoint:   "https://management.usgovcloudapi.net",
		ActiveDirectoryEndpoint:   "https://login.microsoftonline.com/",
		GalleryEndpoint:           "https://gallery.usgovcloudapi.net/",
		KeyVaultEndpoint:          "https://vault.azure.net/",
		GraphEndpoint:             "https://graph.usgovcloudapi.net/",
		StorageEndpointSuffix:     "core.usgovcloudapi.net",
		SQLDatabaseDNSSuffix:      "database.usgovcloudapi.net",
		TrafficManagerDNSSuffix:   "trafficmanager.net",
		KeyVaultDNSSuffix:         "vault.azure.net",
		ServiceBusEndpointSuffix:  "servicebus.usgovcloudapi.net",
	}

	// ChinaCloud is the cloud environment operated in China
	ChinaCloud = Environment{
		Name:                      "AzureChinaCloud",
		ManagementPortalURL:       "https://manage.chinacloudapi.com/",
		PublishSettingsURL:        "https://manage.chinacloudapi.com/publishsettings/index",
		ServiceManagementEndpoint: "https://management.core.chinacloudapi.cn/",
		ResourceManagerEndpoint:   "https://management.chinacloudapi.cn/",
		ActiveDirectoryEndpoint:   "https://login.chinacloudapi.cn/?api-version=1.0",
		GalleryEndpoint:           "https://gallery.chinacloudapi.cn/",
		KeyVaultEndpoint:          "https://vault.azure.net/",
		GraphEndpoint:             "https://graph.chinacloudapi.cn/",
		StorageEndpointSuffix:     "core.chinacloudapi.cn",
		SQLDatabaseDNSSuffix:      "database.chinacloudapi.cn",
		TrafficManagerDNSSuffix:   "trafficmanager.cn",
		KeyVaultDNSSuffix:         "vault.azure.net",
		ServiceBusEndpointSuffix:  "servicebus.chinacloudapi.net",
	}
)

// OAuthConfigForTenant returns an OAuthConfig with tenant specific urls
func (env Environment) OAuthConfigForTenant(tenantID string) (*OAuthConfig, error) {
	template := "%s/oauth2/%s?api-version=%s"
	u, err := url.Parse(env.ActiveDirectoryEndpoint)
	if err != nil {
		return nil, err
	}
	authorizeURL, err := u.Parse(fmt.Sprintf(template, tenantID, "authorize", activeDirectoryAPIVersion))
	if err != nil {
		return nil, err
	}
	tokenURL, err := u.Parse(fmt.Sprintf(template, tenantID, "token", activeDirectoryAPIVersion))
	if err != nil {
		return nil, err
	}
	deviceCodeURL, err := u.Parse(fmt.Sprintf(template, tenantID, "devicecode", activeDirectoryAPIVersion))
	if err != nil {
		return nil, err
	}

	return &OAuthConfig{
		AuthorizeEndpoint:  *authorizeURL,
		TokenEndpoint:      *tokenURL,
		DeviceCodeEndpoint: *deviceCodeURL,
	}, nil
}
