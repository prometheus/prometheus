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
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-10-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
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

	authMethodOAuth           = "OAuth"
	authMethodManagedIdentity = "ManagedIdentity"
)

var (
	userAgent = fmt.Sprintf("Prometheus/%s", version.Version)

	// DefaultSDConfig is the default Azure SD configuration.
	DefaultSDConfig = SDConfig{
		Port:                 80,
		RefreshInterval:      model.Duration(5 * time.Minute),
		Environment:          azure.PublicCloud.Name,
		AuthenticationMethod: authMethodOAuth,
		HTTPClientConfig:     config_util.DefaultHTTPClientConfig,
	}

	failuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_azure_failures_total",
			Help: "Number of Azure service discovery refresh failures.",
		})
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
	prometheus.MustRegister(failuresCount)
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

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "azure" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger), nil
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

	if c.AuthenticationMethod != authMethodOAuth && c.AuthenticationMethod != authMethodManagedIdentity {
		return fmt.Errorf("unknown authentication_type %q. Supported types are %q or %q", c.AuthenticationMethod, authMethodOAuth, authMethodManagedIdentity)
	}

	return nil
}

type Discovery struct {
	*refresh.Discovery
	logger log.Logger
	cfg    *SDConfig
	port   int
}

// NewDiscovery returns a new AzureDiscovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &Discovery{
		cfg:    cfg,
		port:   cfg.Port,
		logger: logger,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"azure",
		time.Duration(cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

// azureClient represents multiple Azure Resource Manager providers.
type azureClient struct {
	nic    network.InterfacesClient
	vm     compute.VirtualMachinesClient
	vmss   compute.VirtualMachineScaleSetsClient
	vmssvm compute.VirtualMachineScaleSetVMsClient
}

// createAzureClient is a helper function for creating an Azure compute client to ARM.
func createAzureClient(cfg SDConfig) (azureClient, error) {
	env, err := azure.EnvironmentFromName(cfg.Environment)
	if err != nil {
		return azureClient{}, err
	}

	activeDirectoryEndpoint := env.ActiveDirectoryEndpoint
	resourceManagerEndpoint := env.ResourceManagerEndpoint

	var c azureClient

	var spt *adal.ServicePrincipalToken

	switch cfg.AuthenticationMethod {
	case authMethodManagedIdentity:
		spt, err = adal.NewServicePrincipalTokenFromManagedIdentity(resourceManagerEndpoint, &adal.ManagedIdentityOptions{ClientID: cfg.ClientID})
		if err != nil {
			return azureClient{}, err
		}
	case authMethodOAuth:
		oauthConfig, err := adal.NewOAuthConfig(activeDirectoryEndpoint, cfg.TenantID)
		if err != nil {
			return azureClient{}, err
		}

		spt, err = adal.NewServicePrincipalToken(*oauthConfig, cfg.ClientID, string(cfg.ClientSecret), resourceManagerEndpoint)
		if err != nil {
			return azureClient{}, err
		}
	}

	client, err := config_util.NewClientFromConfig(cfg.HTTPClientConfig, "azure_sd")
	if err != nil {
		return azureClient{}, err
	}
	sender := autorest.DecorateSender(client)
	preparer := autorest.WithUserAgent(userAgent)

	bearerAuthorizer := autorest.NewBearerAuthorizer(spt)

	c.vm = compute.NewVirtualMachinesClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.vm.Authorizer = bearerAuthorizer
	c.vm.Sender = sender
	c.vm.RequestInspector = preparer

	c.nic = network.NewInterfacesClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.nic.Authorizer = bearerAuthorizer
	c.nic.Sender = sender
	c.nic.RequestInspector = preparer

	c.vmss = compute.NewVirtualMachineScaleSetsClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.vmss.Authorizer = bearerAuthorizer
	c.vmss.Sender = sender
	c.vmss.RequestInspector = preparer

	c.vmssvm = compute.NewVirtualMachineScaleSetVMsClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.vmssvm.Authorizer = bearerAuthorizer
	c.vmssvm.Sender = sender
	c.vmssvm.RequestInspector = preparer

	return c, nil
}

// azureResource represents a resource identifier in Azure.
type azureResource struct {
	Name          string
	ResourceGroup string
}

// virtualMachine represents an Azure virtual machine (which can also be created by a VMSS)
type virtualMachine struct {
	ID                string
	Name              string
	ComputerName      string
	Type              string
	Location          string
	OsType            string
	ScaleSet          string
	Tags              map[string]*string
	NetworkInterfaces []string
}

// Create a new azureResource object from an ID string.
func newAzureResourceFromID(id string, logger log.Logger) (azureResource, error) {
	// Resource IDs have the following format.
	// /subscriptions/SUBSCRIPTION_ID/resourceGroups/RESOURCE_GROUP/providers/PROVIDER/TYPE/NAME
	// or if embedded resource then
	// /subscriptions/SUBSCRIPTION_ID/resourceGroups/RESOURCE_GROUP/providers/PROVIDER/TYPE/NAME/TYPE/NAME
	s := strings.Split(id, "/")
	if len(s) != 9 && len(s) != 11 {
		err := fmt.Errorf("invalid ID '%s'. Refusing to create azureResource", id)
		level.Error(logger).Log("err", err)
		return azureResource{}, err
	}

	return azureResource{
		Name:          strings.ToLower(s[8]),
		ResourceGroup: strings.ToLower(s[4]),
	}, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	defer level.Debug(d.logger).Log("msg", "Azure discovery completed")

	client, err := createAzureClient(*d.cfg)
	if err != nil {
		failuresCount.Inc()
		return nil, fmt.Errorf("could not create Azure client: %w", err)
	}

	machines, err := client.getVMs(ctx, d.cfg.ResourceGroup)
	if err != nil {
		failuresCount.Inc()
		return nil, fmt.Errorf("could not get virtual machines: %w", err)
	}

	level.Debug(d.logger).Log("msg", "Found virtual machines during Azure discovery.", "count", len(machines))

	// Load the vms managed by scale sets.
	scaleSets, err := client.getScaleSets(ctx, d.cfg.ResourceGroup)
	if err != nil {
		failuresCount.Inc()
		return nil, fmt.Errorf("could not get virtual machine scale sets: %w", err)
	}

	for _, scaleSet := range scaleSets {
		scaleSetVms, err := client.getScaleSetVMs(ctx, scaleSet)
		if err != nil {
			failuresCount.Inc()
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
			r, err := newAzureResourceFromID(vm.ID, d.logger)
			if err != nil {
				ch <- target{labelSet: nil, err: err}
				return
			}

			labels := model.LabelSet{
				azureLabelSubscriptionID:       model.LabelValue(d.cfg.SubscriptionID),
				azureLabelTenantID:             model.LabelValue(d.cfg.TenantID),
				azureLabelMachineID:            model.LabelValue(vm.ID),
				azureLabelMachineName:          model.LabelValue(vm.Name),
				azureLabelMachineComputerName:  model.LabelValue(vm.ComputerName),
				azureLabelMachineOSType:        model.LabelValue(vm.OsType),
				azureLabelMachineLocation:      model.LabelValue(vm.Location),
				azureLabelMachineResourceGroup: model.LabelValue(r.ResourceGroup),
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
				networkInterface, err := client.getNetworkInterfaceByID(ctx, nicID)
				if err != nil {
					if errors.Is(err, errorNotFound) {
						level.Warn(d.logger).Log("msg", "Network interface does not exist", "name", nicID, "err", err)
					} else {
						ch <- target{labelSet: nil, err: err}
					}
					// Get out of this routine because we cannot continue without a network interface.
					return
				}

				if networkInterface.InterfacePropertiesFormat == nil {
					continue
				}

				// Unfortunately Azure does not return information on whether a VM is deallocated.
				// This information is available via another API call however the Go SDK does not
				// yet support this. On deallocated machines, this value happens to be nil so it
				// is a cheap and easy way to determine if a machine is allocated or not.
				if networkInterface.Primary == nil {
					level.Debug(d.logger).Log("msg", "Skipping deallocated virtual machine", "machine", vm.Name)
					return
				}

				if *networkInterface.Primary {
					for _, ip := range *networkInterface.IPConfigurations {
						// IPAddress is a field defined in PublicIPAddressPropertiesFormat,
						// therefore we need to validate that both are not nil.
						if ip.PublicIPAddress != nil && ip.PublicIPAddress.PublicIPAddressPropertiesFormat != nil && ip.PublicIPAddress.IPAddress != nil {
							labels[azureLabelMachinePublicIP] = model.LabelValue(*ip.PublicIPAddress.IPAddress)
						}
						if ip.PrivateIPAddress != nil {
							labels[azureLabelMachinePrivateIP] = model.LabelValue(*ip.PrivateIPAddress)
							address := net.JoinHostPort(*ip.PrivateIPAddress, fmt.Sprintf("%d", d.port))
							labels[model.AddressLabel] = model.LabelValue(address)
							ch <- target{labelSet: labels, err: nil}
							return
						}
						// If we made it here, we don't have a private IP which should be impossible.
						// Return an empty target and error to ensure an all or nothing situation.
						err = fmt.Errorf("unable to find a private IP for VM %s", vm.Name)
						ch <- target{labelSet: nil, err: err}
						return
					}
				}
			}
		}(vm)
	}

	wg.Wait()
	close(ch)

	var tg targetgroup.Group
	for tgt := range ch {
		if tgt.err != nil {
			failuresCount.Inc()
			return nil, fmt.Errorf("unable to complete Azure service discovery: %w", tgt.err)
		}
		if tgt.labelSet != nil {
			tg.Targets = append(tg.Targets, tgt.labelSet)
		}
	}

	return []*targetgroup.Group{&tg}, nil
}

func (client *azureClient) getVMs(ctx context.Context, resourceGroup string) ([]virtualMachine, error) {
	var vms []virtualMachine
	var result compute.VirtualMachineListResultPage
	var err error
	if len(resourceGroup) == 0 {
		result, err = client.vm.ListAll(ctx)
	} else {
		result, err = client.vm.List(ctx, resourceGroup)
	}
	if err != nil {
		return nil, fmt.Errorf("could not list virtual machines: %w", err)
	}
	for result.NotDone() {
		for _, vm := range result.Values() {
			vms = append(vms, mapFromVM(vm))
		}
		err = result.NextWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not list virtual machines: %w", err)
		}
	}

	return vms, nil
}

type VmssListResultPage interface {
	NextWithContext(ctx context.Context) (err error)
	NotDone() bool
	Values() []compute.VirtualMachineScaleSet
}

func (client *azureClient) getScaleSets(ctx context.Context, resourceGroup string) ([]compute.VirtualMachineScaleSet, error) {
	var scaleSets []compute.VirtualMachineScaleSet
	var result VmssListResultPage
	var err error
	if len(resourceGroup) == 0 {
		var rtn compute.VirtualMachineScaleSetListWithLinkResultPage
		rtn, err = client.vmss.ListAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not list virtual machine scale sets: %w", err)
		}
		result = &rtn
	} else {
		var rtn compute.VirtualMachineScaleSetListResultPage
		rtn, err = client.vmss.List(ctx, resourceGroup)
		if err != nil {
			return nil, fmt.Errorf("could not list virtual machine scale sets: %w", err)
		}
		result = &rtn
	}

	for result.NotDone() {
		scaleSets = append(scaleSets, result.Values()...)
		err = result.NextWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not list virtual machine scale sets: %w", err)
		}
	}

	return scaleSets, nil
}

func (client *azureClient) getScaleSetVMs(ctx context.Context, scaleSet compute.VirtualMachineScaleSet) ([]virtualMachine, error) {
	var vms []virtualMachine
	// TODO do we really need to fetch the resourcegroup this way?
	r, err := newAzureResourceFromID(*scaleSet.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("could not parse scale set ID: %w", err)
	}

	result, err := client.vmssvm.List(ctx, r.ResourceGroup, *(scaleSet.Name), "", "", "")
	if err != nil {
		return nil, fmt.Errorf("could not list virtual machine scale set vms: %w", err)
	}
	for result.NotDone() {
		for _, vm := range result.Values() {
			vms = append(vms, mapFromVMScaleSetVM(vm, *scaleSet.Name))
		}
		err = result.NextWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not list virtual machine scale set vms: %w", err)
		}
	}

	return vms, nil
}

func mapFromVM(vm compute.VirtualMachine) virtualMachine {
	osType := string(vm.StorageProfile.OsDisk.OsType)
	tags := map[string]*string{}
	networkInterfaces := []string{}
	var computerName string

	if vm.Tags != nil {
		tags = vm.Tags
	}

	if vm.NetworkProfile != nil {
		for _, vmNIC := range *(vm.NetworkProfile.NetworkInterfaces) {
			networkInterfaces = append(networkInterfaces, *vmNIC.ID)
		}
	}

	if vm.VirtualMachineProperties != nil &&
		vm.VirtualMachineProperties.OsProfile != nil &&
		vm.VirtualMachineProperties.OsProfile.ComputerName != nil {
		computerName = *(vm.VirtualMachineProperties.OsProfile.ComputerName)
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
	}
}

func mapFromVMScaleSetVM(vm compute.VirtualMachineScaleSetVM, scaleSetName string) virtualMachine {
	osType := string(vm.StorageProfile.OsDisk.OsType)
	tags := map[string]*string{}
	networkInterfaces := []string{}
	var computerName string

	if vm.Tags != nil {
		tags = vm.Tags
	}

	if vm.NetworkProfile != nil {
		for _, vmNIC := range *(vm.NetworkProfile.NetworkInterfaces) {
			networkInterfaces = append(networkInterfaces, *vmNIC.ID)
		}
	}

	if vm.VirtualMachineScaleSetVMProperties != nil && vm.VirtualMachineScaleSetVMProperties.OsProfile != nil {
		computerName = *(vm.VirtualMachineScaleSetVMProperties.OsProfile.ComputerName)
	}

	return virtualMachine{
		ID:                *(vm.ID),
		Name:              *(vm.Name),
		ComputerName:      computerName,
		Type:              *(vm.Type),
		Location:          *(vm.Location),
		OsType:            osType,
		ScaleSet:          scaleSetName,
		Tags:              tags,
		NetworkInterfaces: networkInterfaces,
	}
}

var errorNotFound = errors.New("network interface does not exist")

// getNetworkInterfaceByID gets the network interface.
// If a 404 is returned from the Azure API, `errorNotFound` is returned.
// On all other errors, an autorest.DetailedError is returned.
func (client *azureClient) getNetworkInterfaceByID(ctx context.Context, networkInterfaceID string) (*network.Interface, error) {
	result := network.Interface{}
	queryParameters := map[string]interface{}{
		"api-version": "2018-10-01",
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.nic.BaseURI),
		autorest.WithPath(networkInterfaceID),
		autorest.WithQueryParameters(queryParameters),
		autorest.WithUserAgent(userAgent))
	req, err := preparer.Prepare((&http.Request{}).WithContext(ctx))
	if err != nil {
		return nil, autorest.NewErrorWithError(err, "network.InterfacesClient", "Get", nil, "Failure preparing request")
	}

	resp, err := client.nic.GetSender(req)
	if err != nil {
		return nil, autorest.NewErrorWithError(err, "network.InterfacesClient", "Get", resp, "Failure sending request")
	}

	result, err = client.nic.GetResponder(resp)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return nil, errorNotFound
		}
		return nil, autorest.NewErrorWithError(err, "network.InterfacesClient", "Get", resp, "Failure responding to request")
	}

	return &result, nil
}
