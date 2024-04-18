// Copyright 2015 The Prometheus Authors
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

package azure

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4"
	cache "github.com/Code-Hex/go-generics-cache"
	"github.com/Code-Hex/go-generics-cache/policy/lru"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	azureLabel                     = model.MetaLabelPrefix + "azure_"
	azureLabelSubscriptionID       = azureLabel + "subscription_id"
	azureLabelTenantID             = azureLabel + "tenant_id"
	azureLabelMachineID            = azureLabel + "machine_id"
	azureLabelMachineResourceGroup = azureLabel + "machine_resource_group"
	azureLabelMachineName          = azureLabel + "machine_name"
	azureLabelMachineComputerName  = azureLabel + "machine_computer_name"
	azureLabelMachineOSType        = azureLabel + "machine_os_type"
	azureLabelMachineLocation      = azureLabel + "machine_location"
	azureLabelMachinePrivateIP     = azureLabel + "machine_private_ip"
	azureLabelMachinePublicIP      = azureLabel + "machine_public_ip"
	azureLabelMachineTag           = azureLabel + "machine_tag_"
	azureLabelMachineScaleSet      = azureLabel + "machine_scale_set"
	azureLabelMachineSize          = azureLabel + "machine_size"

	authMethodOAuth           = "OAuth"
	authMethodSDK             = "SDK"
	authMethodManagedIdentity = "ManagedIdentity"
)

var (
	userAgent = fmt.Sprintf("Prometheus/%s", version.Version)

	// DefaultSDConfig is the default Azure SD configuration.
	DefaultSDConfig = SDConfig{
		Port:                 80,
		RefreshInterval:      model.Duration(5 * time.Minute),
		Environment:          "AzurePublicCloud",
		AuthenticationMethod: authMethodOAuth,
		HTTPClientConfig:     config_util.DefaultHTTPClientConfig,
	}
)

var environments = map[string]cloud.Configuration{
	"AZURECHINACLOUD":        cloud.AzureChina,
	"AZURECLOUD":             cloud.AzurePublic,
	"AZUREGERMANCLOUD":       cloud.AzurePublic,
	"AZUREPUBLICCLOUD":       cloud.AzurePublic,
	"AZUREUSGOVERNMENT":      cloud.AzureGovernment,
	"AZUREUSGOVERNMENTCLOUD": cloud.AzureGovernment,
}

// CloudConfigurationFromName returns cloud configuration based on the common name specified.
func CloudConfigurationFromName(name string) (cloud.Configuration, error) {
	name = strings.ToUpper(name)
	env, ok := environments[name]
	if !ok {
		return env, fmt.Errorf("there is no cloud configuration matching the name %q", name)
	}

	return env, nil
}

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for Azure based service discovery.
type SDConfig struct {
	Environment          string             `yaml:"environment,omitempty"`
	Port                 int                `yaml:"port"`
	SubscriptionID       string             `yaml:"subscription_id"`
	TenantID             string             `yaml:"tenant_id,omitempty"`
	ClientID             string             `yaml:"client_id,omitempty"`
	ClientSecret         config_util.Secret `yaml:"client_secret,omitempty"`
	RefreshInterval      model.Duration     `yaml:"refresh_interval,omitempty"`
	AuthenticationMethod string             `yaml:"authentication_method,omitempty"`
	ResourceGroup        string             `yaml:"resource_group,omitempty"`

	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`
}

// NewDiscovererMetrics implements discovery.Config.
func (*SDConfig) NewDiscovererMetrics(reg prometheus.Registerer, rmi discovery.RefreshMetricsInstantiator) discovery.DiscovererMetrics {
	return newDiscovererMetrics(reg, rmi)
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "azure" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger, opts.Metrics)
}

func validateAuthParam(param, name string) error {
	if len(param) == 0 {
		return fmt.Errorf("azure SD configuration requires a %s", name)
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if err = validateAuthParam(c.SubscriptionID, "subscription_id"); err != nil {
		return err
	}

	if c.AuthenticationMethod == authMethodOAuth {
		if err = validateAuthParam(c.TenantID, "tenant_id"); err != nil {
			return err
		}
		if err = validateAuthParam(c.ClientID, "client_id"); err != nil {
			return err
		}
		if err = validateAuthParam(string(c.ClientSecret), "client_secret"); err != nil {
			return err
		}
	}

	if c.AuthenticationMethod != authMethodOAuth && c.AuthenticationMethod != authMethodManagedIdentity && c.AuthenticationMethod != authMethodSDK {
		return fmt.Errorf("unknown authentication_type %q. Supported types are %q, %q or %q", c.AuthenticationMethod, authMethodOAuth, authMethodManagedIdentity, authMethodSDK)
	}

	return c.HTTPClientConfig.Validate()
}

type Discovery struct {
	*refresh.Discovery
	logger  log.Logger
	cfg     *SDConfig
	port    int
	cache   *cache.Cache[string, *armnetwork.Interface]
	metrics *azureMetrics
}

// NewDiscovery returns a new AzureDiscovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger, metrics discovery.DiscovererMetrics) (*Discovery, error) {
	m, ok := metrics.(*azureMetrics)
	if !ok {
		return nil, fmt.Errorf("invalid discovery metrics type")
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}
	l := cache.New(cache.AsLRU[string, *armnetwork.Interface](lru.WithCapacity(5000)))
	d := &Discovery{
		cfg:     cfg,
		port:    cfg.Port,
		logger:  logger,
		cache:   l,
		metrics: m,
	}

	d.Discovery = refresh.NewDiscovery(
		refresh.Options{
			Logger:              logger,
			Mech:                "azure",
			Interval:            time.Duration(cfg.RefreshInterval),
			RefreshF:            d.refresh,
			MetricsInstantiator: m.refreshMetrics,
		},
	)

	return d, nil
}

type client interface {
	getVMs(ctx context.Context, resourceGroup string) ([]virtualMachine, error)
	getScaleSets(ctx context.Context, resourceGroup string) ([]armcompute.VirtualMachineScaleSet, error)
	getScaleSetVMs(ctx context.Context, scaleSet armcompute.VirtualMachineScaleSet) ([]virtualMachine, error)
	getVMNetworkInterfaceByID(ctx context.Context, networkInterfaceID string) (*armnetwork.Interface, error)
	getVMScaleSetVMNetworkInterfaceByID(ctx context.Context, networkInterfaceID, scaleSetName, instanceID string) (*armnetwork.Interface, error)
}

// azureClient represents multiple Azure Resource Manager providers.
type azureClient struct {
	nic    *armnetwork.InterfacesClient
	vm     *armcompute.VirtualMachinesClient
	vmss   *armcompute.VirtualMachineScaleSetsClient
	vmssvm *armcompute.VirtualMachineScaleSetVMsClient
	logger log.Logger
}

var _ client = &azureClient{}

// createAzureClient is a helper function for creating an Azure compute client to ARM.
func createAzureClient(cfg SDConfig, logger log.Logger) (client, error) {
	cloudConfiguration, err := CloudConfigurationFromName(cfg.Environment)
	if err != nil {
		return &azureClient{}, err
	}

	var c azureClient
	c.logger = logger

	telemetry := policy.TelemetryOptions{
		ApplicationID: userAgent,
	}

	credential, err := newCredential(cfg, policy.ClientOptions{
		Cloud:     cloudConfiguration,
		Telemetry: telemetry,
	})
	if err != nil {
		return &azureClient{}, err
	}

	client, err := config_util.NewClientFromConfig(cfg.HTTPClientConfig, "azure_sd")
	if err != nil {
		return &azureClient{}, err
	}
	options := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: client,
			Cloud:     cloudConfiguration,
			Telemetry: telemetry,
		},
	}

	c.vm, err = armcompute.NewVirtualMachinesClient(cfg.SubscriptionID, credential, options)
	if err != nil {
		return &azureClient{}, err
	}

	c.nic, err = armnetwork.NewInterfacesClient(cfg.SubscriptionID, credential, options)
	if err != nil {
		return &azureClient{}, err
	}

	c.vmss, err = armcompute.NewVirtualMachineScaleSetsClient(cfg.SubscriptionID, credential, options)
	if err != nil {
		return &azureClient{}, err
	}

	c.vmssvm, err = armcompute.NewVirtualMachineScaleSetVMsClient(cfg.SubscriptionID, credential, options)
	if err != nil {
		return &azureClient{}, err
	}

	return &c, nil
}

func newCredential(cfg SDConfig, policyClientOptions policy.ClientOptions) (azcore.TokenCredential, error) {
	var credential azcore.TokenCredential
	switch cfg.AuthenticationMethod {
	case authMethodManagedIdentity:
		options := &azidentity.ManagedIdentityCredentialOptions{ClientOptions: policyClientOptions, ID: azidentity.ClientID(cfg.ClientID)}
		managedIdentityCredential, err := azidentity.NewManagedIdentityCredential(options)
		if err != nil {
			return nil, err
		}
		credential = azcore.TokenCredential(managedIdentityCredential)
	case authMethodOAuth:
		options := &azidentity.ClientSecretCredentialOptions{ClientOptions: policyClientOptions}
		secretCredential, err := azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, string(cfg.ClientSecret), options)
		if err != nil {
			return nil, err
		}
		credential = azcore.TokenCredential(secretCredential)
	case authMethodSDK:
		options := &azidentity.DefaultAzureCredentialOptions{ClientOptions: policyClientOptions}
		if len(cfg.TenantID) != 0 {
			options.TenantID = cfg.TenantID
		}
		sdkCredential, err := azidentity.NewDefaultAzureCredential(options)
		if err != nil {
			return nil, err
		}
		credential = azcore.TokenCredential(sdkCredential)
	}
	return credential, nil
}

// virtualMachine represents an Azure virtual machine (which can also be created by a VMSS).
type virtualMachine struct {
	ID                string
	Name              string
	ComputerName      string
	Type              string
	Location          string
	OsType            string
	ScaleSet          string
	InstanceID        string
	Tags              map[string]*string
	NetworkInterfaces []string
	Size              string
}

// Create a new azureResource object from an ID string.
func newAzureResourceFromID(id string, logger log.Logger) (*arm.ResourceID, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	resourceID, err := arm.ParseResourceID(id)
	if err != nil {
		err := fmt.Errorf("invalid ID '%s': %w", id, err)
		level.Error(logger).Log("err", err)
		return &arm.ResourceID{}, err
	}
	return resourceID, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	defer level.Debug(d.logger).Log("msg", "Azure discovery completed")

	client, err := createAzureClient(*d.cfg, d.logger)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, fmt.Errorf("could not create Azure client: %w", err)
	}

	machines, err := client.getVMs(ctx, d.cfg.ResourceGroup)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, fmt.Errorf("could not get virtual machines: %w", err)
	}

	level.Debug(d.logger).Log("msg", "Found virtual machines during Azure discovery.", "count", len(machines))

	// Load the vms managed by scale sets.
	scaleSets, err := client.getScaleSets(ctx, d.cfg.ResourceGroup)
	if err != nil {
		d.metrics.failuresCount.Inc()
		return nil, fmt.Errorf("could not get virtual machine scale sets: %w", err)
	}

	for _, scaleSet := range scaleSets {
		scaleSetVms, err := client.getScaleSetVMs(ctx, scaleSet)
		if err != nil {
			d.metrics.failuresCount.Inc()
			return nil, fmt.Errorf("could not get virtual machine scale set vms: %w", err)
		}
		machines = append(machines, scaleSetVms...)
	}

	// We have the slice of machines. Now turn them into targets.
	// Doing them in go routines because the network interface calls are slow.
	type target struct {
		labelSet model.LabelSet
		err      error
	}

	var wg sync.WaitGroup
	wg.Add(len(machines))
	ch := make(chan target, len(machines))
	for _, vm := range machines {
		go func(vm virtualMachine) {
			defer wg.Done()
			labelSet, err := d.vmToLabelSet(ctx, client, vm)
			ch <- target{labelSet: labelSet, err: err}
		}(vm)
	}

	wg.Wait()
	close(ch)

	var tg targetgroup.Group
	for tgt := range ch {
		if tgt.err != nil {
			d.metrics.failuresCount.Inc()
			return nil, fmt.Errorf("unable to complete Azure service discovery: %w", tgt.err)
		}
		if tgt.labelSet != nil {
			tg.Targets = append(tg.Targets, tgt.labelSet)
		}
	}

	return []*targetgroup.Group{&tg}, nil
}

func (d *Discovery) vmToLabelSet(ctx context.Context, client client, vm virtualMachine) (model.LabelSet, error) {
	r, err := newAzureResourceFromID(vm.ID, d.logger)
	if err != nil {
		return nil, err
	}

	labels := model.LabelSet{
		azureLabelSubscriptionID:       model.LabelValue(d.cfg.SubscriptionID),
		azureLabelTenantID:             model.LabelValue(d.cfg.TenantID),
		azureLabelMachineID:            model.LabelValue(vm.ID),
		azureLabelMachineName:          model.LabelValue(vm.Name),
		azureLabelMachineComputerName:  model.LabelValue(vm.ComputerName),
		azureLabelMachineOSType:        model.LabelValue(vm.OsType),
		azureLabelMachineLocation:      model.LabelValue(vm.Location),
		azureLabelMachineResourceGroup: model.LabelValue(r.ResourceGroupName),
		azureLabelMachineSize:          model.LabelValue(vm.Size),
	}

	if vm.ScaleSet != "" {
		labels[azureLabelMachineScaleSet] = model.LabelValue(vm.ScaleSet)
	}

	for k, v := range vm.Tags {
		name := strutil.SanitizeLabelName(k)
		labels[azureLabelMachineTag+model.LabelName(name)] = model.LabelValue(*v)
	}

	// Get the IP address information via separate call to the network provider.
	for _, nicID := range vm.NetworkInterfaces {
		var networkInterface *armnetwork.Interface
		if v, ok := d.getFromCache(nicID); ok {
			networkInterface = v
			d.metrics.cacheHitCount.Add(1)
		} else {
			if vm.ScaleSet == "" {
				networkInterface, err = client.getVMNetworkInterfaceByID(ctx, nicID)
			} else {
				networkInterface, err = client.getVMScaleSetVMNetworkInterfaceByID(ctx, nicID, vm.ScaleSet, vm.InstanceID)
			}
			if err != nil {
				if errors.Is(err, errorNotFound) {
					level.Warn(d.logger).Log("msg", "Network interface does not exist", "name", nicID, "err", err)
				} else {
					return nil, err
				}
				// Get out of this routine because we cannot continue without a network interface.
				return nil, nil
			}

			// Continue processing with the network interface
			d.addToCache(nicID, networkInterface)
		}

		if networkInterface.Properties == nil {
			continue
		}

		// Unfortunately Azure does not return information on whether a VM is deallocated.
		// This information is available via another API call however the Go SDK does not
		// yet support this. On deallocated machines, this value happens to be nil so it
		// is a cheap and easy way to determine if a machine is allocated or not.
		if networkInterface.Properties.Primary == nil {
			level.Debug(d.logger).Log("msg", "Skipping deallocated virtual machine", "machine", vm.Name)
			return nil, nil
		}

		if *networkInterface.Properties.Primary {
			for _, ip := range networkInterface.Properties.IPConfigurations {
				// IPAddress is a field defined in PublicIPAddressPropertiesFormat,
				// therefore we need to validate that both are not nil.
				if ip.Properties != nil && ip.Properties.PublicIPAddress != nil && ip.Properties.PublicIPAddress.Properties != nil && ip.Properties.PublicIPAddress.Properties.IPAddress != nil {
					labels[azureLabelMachinePublicIP] = model.LabelValue(*ip.Properties.PublicIPAddress.Properties.IPAddress)
				}
				if ip.Properties != nil && ip.Properties.PrivateIPAddress != nil {
					labels[azureLabelMachinePrivateIP] = model.LabelValue(*ip.Properties.PrivateIPAddress)
					address := net.JoinHostPort(*ip.Properties.PrivateIPAddress, fmt.Sprintf("%d", d.port))
					labels[model.AddressLabel] = model.LabelValue(address)
					return labels, nil
				}
				// If we made it here, we don't have a private IP which should be impossible.
				// Return an empty target and error to ensure an all or nothing situation.
				return nil, fmt.Errorf("unable to find a private IP for VM %s", vm.Name)
			}
		}
	}
	// TODO: Should we say something at this point?
	return nil, nil
}

func (client *azureClient) getVMs(ctx context.Context, resourceGroup string) ([]virtualMachine, error) {
	var vms []virtualMachine
	if len(resourceGroup) == 0 {
		pager := client.vm.NewListAllPager(nil)
		for pager.More() {
			nextResult, err := pager.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("could not list virtual machines: %w", err)
			}
			for _, vm := range nextResult.Value {
				vms = append(vms, mapFromVM(*vm))
			}
		}
	} else {
		pager := client.vm.NewListPager(resourceGroup, nil)
		for pager.More() {
			nextResult, err := pager.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("could not list virtual machines: %w", err)
			}
			for _, vm := range nextResult.Value {
				vms = append(vms, mapFromVM(*vm))
			}
		}
	}
	return vms, nil
}

func (client *azureClient) getScaleSets(ctx context.Context, resourceGroup string) ([]armcompute.VirtualMachineScaleSet, error) {
	var scaleSets []armcompute.VirtualMachineScaleSet
	if len(resourceGroup) == 0 {
		pager := client.vmss.NewListAllPager(nil)
		for pager.More() {
			nextResult, err := pager.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("could not list virtual machine scale sets: %w", err)
			}
			for _, vmss := range nextResult.Value {
				scaleSets = append(scaleSets, *vmss)
			}
		}
	} else {
		pager := client.vmss.NewListPager(resourceGroup, nil)
		for pager.More() {
			nextResult, err := pager.NextPage(ctx)
			if err != nil {
				return nil, fmt.Errorf("could not list virtual machine scale sets: %w", err)
			}
			for _, vmss := range nextResult.Value {
				scaleSets = append(scaleSets, *vmss)
			}
		}
	}
	return scaleSets, nil
}

func (client *azureClient) getScaleSetVMs(ctx context.Context, scaleSet armcompute.VirtualMachineScaleSet) ([]virtualMachine, error) {
	var vms []virtualMachine
	// TODO do we really need to fetch the resourcegroup this way?
	r, err := newAzureResourceFromID(*scaleSet.ID, client.logger)
	if err != nil {
		return nil, fmt.Errorf("could not parse scale set ID: %w", err)
	}

	pager := client.vmssvm.NewListPager(r.ResourceGroupName, *(scaleSet.Name), nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not list virtual machine scale set vms: %w", err)
		}
		for _, vmssvm := range nextResult.Value {
			vms = append(vms, mapFromVMScaleSetVM(*vmssvm, *scaleSet.Name))
		}
	}

	return vms, nil
}

func mapFromVM(vm armcompute.VirtualMachine) virtualMachine {
	var osType string
	tags := map[string]*string{}
	networkInterfaces := []string{}
	var computerName string
	var size string

	if vm.Tags != nil {
		tags = vm.Tags
	}

	if vm.Properties != nil {
		if vm.Properties.StorageProfile != nil &&
			vm.Properties.StorageProfile.OSDisk != nil &&
			vm.Properties.StorageProfile.OSDisk.OSType != nil {
			osType = string(*vm.Properties.StorageProfile.OSDisk.OSType)
		}

		if vm.Properties.NetworkProfile != nil {
			for _, vmNIC := range vm.Properties.NetworkProfile.NetworkInterfaces {
				networkInterfaces = append(networkInterfaces, *vmNIC.ID)
			}
		}
		if vm.Properties.OSProfile != nil && vm.Properties.OSProfile.ComputerName != nil {
			computerName = *(vm.Properties.OSProfile.ComputerName)
		}
		if vm.Properties.HardwareProfile != nil {
			size = string(*vm.Properties.HardwareProfile.VMSize)
		}
	}

	return virtualMachine{
		ID:                *(vm.ID),
		Name:              *(vm.Name),
		ComputerName:      computerName,
		Type:              *(vm.Type),
		Location:          *(vm.Location),
		OsType:            osType,
		ScaleSet:          "",
		Tags:              tags,
		NetworkInterfaces: networkInterfaces,
		Size:              size,
	}
}

func mapFromVMScaleSetVM(vm armcompute.VirtualMachineScaleSetVM, scaleSetName string) virtualMachine {
	var osType string
	tags := map[string]*string{}
	networkInterfaces := []string{}
	var computerName string
	var size string

	if vm.Tags != nil {
		tags = vm.Tags
	}

	if vm.Properties != nil {
		if vm.Properties.StorageProfile != nil &&
			vm.Properties.StorageProfile.OSDisk != nil &&
			vm.Properties.StorageProfile.OSDisk.OSType != nil {
			osType = string(*vm.Properties.StorageProfile.OSDisk.OSType)
		}

		if vm.Properties.NetworkProfile != nil {
			for _, vmNIC := range vm.Properties.NetworkProfile.NetworkInterfaces {
				networkInterfaces = append(networkInterfaces, *vmNIC.ID)
			}
		}
		if vm.Properties.OSProfile != nil && vm.Properties.OSProfile.ComputerName != nil {
			computerName = *(vm.Properties.OSProfile.ComputerName)
		}
		if vm.Properties.HardwareProfile != nil {
			size = string(*vm.Properties.HardwareProfile.VMSize)
		}
	}

	return virtualMachine{
		ID:                *(vm.ID),
		Name:              *(vm.Name),
		ComputerName:      computerName,
		Type:              *(vm.Type),
		Location:          *(vm.Location),
		OsType:            osType,
		ScaleSet:          scaleSetName,
		InstanceID:        *(vm.InstanceID),
		Tags:              tags,
		NetworkInterfaces: networkInterfaces,
		Size:              size,
	}
}

var errorNotFound = errors.New("network interface does not exist")

// getVMNetworkInterfaceByID gets the network interface.
// If a 404 is returned from the Azure API, `errorNotFound` is returned.
func (client *azureClient) getVMNetworkInterfaceByID(ctx context.Context, networkInterfaceID string) (*armnetwork.Interface, error) {
	r, err := newAzureResourceFromID(networkInterfaceID, client.logger)
	if err != nil {
		return nil, fmt.Errorf("could not parse network interface ID: %w", err)
	}

	resp, err := client.nic.Get(ctx, r.ResourceGroupName, r.Name, &armnetwork.InterfacesClientGetOptions{Expand: to.Ptr("IPConfigurations/PublicIPAddress")})
	if err != nil {
		var responseError *azcore.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return nil, errorNotFound
		}
		return nil, fmt.Errorf("failed to retrieve Interface %v with error: %w", networkInterfaceID, err)
	}

	return &resp.Interface, nil
}

// getVMScaleSetVMNetworkInterfaceByID gets the network interface.
// If a 404 is returned from the Azure API, `errorNotFound` is returned.
func (client *azureClient) getVMScaleSetVMNetworkInterfaceByID(ctx context.Context, networkInterfaceID, scaleSetName, instanceID string) (*armnetwork.Interface, error) {
	r, err := newAzureResourceFromID(networkInterfaceID, client.logger)
	if err != nil {
		return nil, fmt.Errorf("could not parse network interface ID: %w", err)
	}

	resp, err := client.nic.GetVirtualMachineScaleSetNetworkInterface(ctx, r.ResourceGroupName, scaleSetName, instanceID, r.Name, &armnetwork.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceOptions{Expand: to.Ptr("IPConfigurations/PublicIPAddress")})
	if err != nil {
		var responseError *azcore.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return nil, errorNotFound
		}
		return nil, fmt.Errorf("failed to retrieve Interface %v with error: %w", networkInterfaceID, err)
	}

	return &resp.Interface, nil
}

// addToCache will add the network interface information for the specified nicID.
func (d *Discovery) addToCache(nicID string, netInt *armnetwork.Interface) {
	random := rand.Int63n(int64(time.Duration(d.cfg.RefreshInterval * 3).Seconds()))
	rs := time.Duration(random) * time.Second
	exptime := time.Duration(d.cfg.RefreshInterval*10) + rs
	d.cache.Set(nicID, netInt, cache.WithExpiration(exptime))
	level.Debug(d.logger).Log("msg", "Adding nic", "nic", nicID, "time", exptime.Seconds())
}

// getFromCache will get the network Interface for the specified nicID
// If the cache is disabled nothing will happen.
func (d *Discovery) getFromCache(nicID string) (*armnetwork.Interface, bool) {
	net, found := d.cache.Get(nicID)
	return net, found
}
