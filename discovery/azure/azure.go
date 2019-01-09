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
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
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
	azureLabelMachineOSType        = azureLabel + "machine_os_type"
	azureLabelMachineLocation      = azureLabel + "machine_location"
	azureLabelMachinePrivateIP     = azureLabel + "machine_private_ip"
	azureLabelMachineTag           = azureLabel + "machine_tag_"
	azureLabelMachineScaleSet      = azureLabel + "machine_scale_set"

	authMethodOAuth           = "OAuth"
	authMethodManagedIdentity = "ManagedIdentity"
)

var (
	azureSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_azure_refresh_failures_total",
			Help: "Number of Azure-SD refresh failures.",
		})
	azureSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_azure_refresh_duration_seconds",
			Help: "The duration of a Azure-SD refresh in seconds.",
		})

	// DefaultSDConfig is the default Azure SD configuration.
	DefaultSDConfig = SDConfig{
		Port:                 80,
		RefreshInterval:      model.Duration(5 * time.Minute),
		Environment:          azure.PublicCloud.Name,
		AuthenticationMethod: authMethodOAuth,
	}
)

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

func init() {
	prometheus.MustRegister(azureSDRefreshDuration)
	prometheus.MustRegister(azureSDRefreshFailuresCount)
}

// Discovery periodically performs Azure-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	cfg      *SDConfig
	interval time.Duration
	port     int
	logger   log.Logger
}

// NewDiscovery returns a new AzureDiscovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &Discovery{
		cfg:      cfg,
		interval: time.Duration(cfg.RefreshInterval),
		port:     cfg.Port,
		logger:   logger,
	}
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tg, err := d.refresh()
		if err != nil {
			level.Error(d.logger).Log("msg", "Unable to refresh during Azure discovery", "err", err)
		} else {
			select {
			case <-ctx.Done():
			case ch <- []*targetgroup.Group{tg}:
			}
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
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
		msiEndpoint, err := adal.GetMSIVMEndpoint()
		if err != nil {
			return azureClient{}, err
		}

		spt, err = adal.NewServicePrincipalTokenFromMSI(msiEndpoint, resourceManagerEndpoint)
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

	bearerAuthorizer := autorest.NewBearerAuthorizer(spt)

	c.vm = compute.NewVirtualMachinesClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.vm.Authorizer = bearerAuthorizer

	c.nic = network.NewInterfacesClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.nic.Authorizer = bearerAuthorizer

	c.vmss = compute.NewVirtualMachineScaleSetsClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.vmss.Authorizer = bearerAuthorizer

	c.vmssvm = compute.NewVirtualMachineScaleSetVMsClientWithBaseURI(resourceManagerEndpoint, cfg.SubscriptionID)
	c.vmssvm.Authorizer = bearerAuthorizer

	return c, nil
}

// azureResource represents a resource identifier in Azure.
type azureResource struct {
	Name          string
	ResourceGroup string
}

// virtualMachine represents an Azure virtual machine (which can also be created by a VMSS)
type virtualMachine struct {
	ID             string
	Name           string
	Type           string
	Location       string
	OsType         string
	ScaleSet       string
	Tags           map[string]*string
	NetworkProfile compute.NetworkProfile
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

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	defer level.Debug(d.logger).Log("msg", "Azure discovery completed")

	t0 := time.Now()
	defer func() {
		azureSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			azureSDRefreshFailuresCount.Inc()
		}
	}()
	tg = &targetgroup.Group{}
	client, err := createAzureClient(*d.cfg)
	if err != nil {
		return tg, fmt.Errorf("could not create Azure client: %s", err)
	}

	machines, err := client.getVMs()
	if err != nil {
		return tg, fmt.Errorf("could not get virtual machines: %s", err)
	}

	level.Debug(d.logger).Log("msg", "Found virtual machines during Azure discovery.", "count", len(machines))

	// Load the vms managed by scale sets.
	scaleSets, err := client.getScaleSets()
	if err != nil {
		return tg, fmt.Errorf("could not get virtual machine scale sets: %s", err)
	}

	for _, scaleSet := range scaleSets {
		scaleSetVms, err := client.getScaleSetVMs(scaleSet)
		if err != nil {
			return tg, fmt.Errorf("could not get virtual machine scale set vms: %s", err)
		}
		machines = append(machines, scaleSetVms...)
	}

	// We have the slice of machines. Now turn them into targets.
	// Doing them in go routines because the network interface calls are slow.
	type target struct {
		labelSet model.LabelSet
		err      error
	}

	ch := make(chan target, len(machines))
	for i, vm := range machines {
		go func(i int, vm virtualMachine) {
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
				azureLabelMachineOSType:        model.LabelValue(vm.OsType),
				azureLabelMachineLocation:      model.LabelValue(vm.Location),
				azureLabelMachineResourceGroup: model.LabelValue(r.ResourceGroup),
			}

			if vm.ScaleSet != "" {
				labels[azureLabelMachineScaleSet] = model.LabelValue(vm.ScaleSet)
			}

			if vm.Tags != nil {
				for k, v := range vm.Tags {
					name := strutil.SanitizeLabelName(k)
					labels[azureLabelMachineTag+model.LabelName(name)] = model.LabelValue(*v)
				}
			}

			// Get the IP address information via separate call to the network provider.
			for _, nic := range *vm.NetworkProfile.NetworkInterfaces {
				networkInterface, err := client.getNetworkInterfaceByID(*nic.ID)
				if err != nil {
					level.Error(d.logger).Log("msg", "Unable to get network interface", "name", *nic.ID, "err", err)
					ch <- target{labelSet: nil, err: err}
					// Get out of this routine because we cannot continue without a network interface.
					return
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
					ch <- target{}
					return
				}

				if *networkInterface.Properties.Primary {
					for _, ip := range *networkInterface.Properties.IPConfigurations {
						if ip.Properties.PrivateIPAddress != nil {
							labels[azureLabelMachinePrivateIP] = model.LabelValue(*ip.Properties.PrivateIPAddress)
							address := net.JoinHostPort(*ip.Properties.PrivateIPAddress, fmt.Sprintf("%d", d.port))
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
		}(i, vm)
	}

	for range machines {
		tgt := <-ch
		if tgt.err != nil {
			return nil, fmt.Errorf("unable to complete Azure service discovery: %s", err)
		}
		if tgt.labelSet != nil {
			tg.Targets = append(tg.Targets, tgt.labelSet)
		}
	}

	return tg, nil
}

func (client *azureClient) getVMs() ([]virtualMachine, error) {
	var vms []virtualMachine
	result, err := client.vm.ListAll()
	if err != nil {
		return vms, fmt.Errorf("could not list virtual machines: %s", err)
	}

	for _, vm := range *result.Value {
		vms = append(vms, mapFromVM(vm))
	}

	// If we still have results, keep going until we have no more.
	for result.NextLink != nil {
		result, err = client.vm.ListAllNextResults(result)
		if err != nil {
			return vms, fmt.Errorf("could not list virtual machines: %s", err)
		}

		for _, vm := range *result.Value {
			vms = append(vms, mapFromVM(vm))
		}
	}

	return vms, nil
}

func (client *azureClient) getScaleSets() ([]compute.VirtualMachineScaleSet, error) {
	var scaleSets []compute.VirtualMachineScaleSet
	result, err := client.vmss.ListAll()
	if err != nil {
		return scaleSets, fmt.Errorf("could not list virtual machine scale sets: %s", err)
	}
	scaleSets = append(scaleSets, *result.Value...)

	for result.NextLink != nil {
		result, err = client.vmss.ListAllNextResults(result)
		if err != nil {
			return scaleSets, fmt.Errorf("could not list virtual machine scale sets: %s", err)
		}
		scaleSets = append(scaleSets, *result.Value...)
	}

	return scaleSets, nil
}

func (client *azureClient) getScaleSetVMs(scaleSet compute.VirtualMachineScaleSet) ([]virtualMachine, error) {
	var vms []virtualMachine
	//TODO do we really need to fetch the resourcegroup this way?
	r, err := newAzureResourceFromID(*scaleSet.ID, nil)

	if err != nil {
		return vms, fmt.Errorf("could not parse scale set ID: %s", err)
	}

	result, err := client.vmssvm.List(r.ResourceGroup, *(scaleSet.Name), "", "", "")
	if err != nil {
		return vms, fmt.Errorf("could not list virtual machine scale set vms: %s", err)
	}

	for _, vm := range *result.Value {
		vms = append(vms, mapFromVMScaleSetVM(vm, *scaleSet.Name))
	}

	for result.NextLink != nil {
		result, err = client.vmssvm.ListNextResults(result)
		if err != nil {
			return vms, fmt.Errorf("could not list virtual machine scale set vms: %s", err)
		}

		for _, vm := range *result.Value {
			vms = append(vms, mapFromVMScaleSetVM(vm, *scaleSet.Name))
		}
	}

	return vms, nil
}

func mapFromVM(vm compute.VirtualMachine) virtualMachine {
	osType := string(vm.Properties.StorageProfile.OsDisk.OsType)
	tags := map[string]*string{}

	if vm.Tags != nil {
		tags = *(vm.Tags)
	}

	return virtualMachine{
		ID:             *(vm.ID),
		Name:           *(vm.Name),
		Type:           *(vm.Type),
		Location:       *(vm.Location),
		OsType:         osType,
		ScaleSet:       "",
		Tags:           tags,
		NetworkProfile: *(vm.Properties.NetworkProfile),
	}
}

func mapFromVMScaleSetVM(vm compute.VirtualMachineScaleSetVM, scaleSetName string) virtualMachine {
	osType := string(vm.Properties.StorageProfile.OsDisk.OsType)
	tags := map[string]*string{}

	if vm.Tags != nil {
		tags = *(vm.Tags)
	}

	return virtualMachine{
		ID:             *(vm.ID),
		Name:           *(vm.Name),
		Type:           *(vm.Type),
		Location:       *(vm.Location),
		OsType:         osType,
		ScaleSet:       scaleSetName,
		Tags:           tags,
		NetworkProfile: *(vm.Properties.NetworkProfile),
	}
}

func (client *azureClient) getNetworkInterfaceByID(networkInterfaceID string) (network.Interface, error) {
	result := network.Interface{}
	queryParameters := map[string]interface{}{
		"api-version": client.nic.APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.nic.BaseURI),
		autorest.WithPath(networkInterfaceID),
		autorest.WithQueryParameters(queryParameters))
	req, err := preparer.Prepare(&http.Request{})
	if err != nil {
		return result, autorest.NewErrorWithError(err, "network.InterfacesClient", "Get", nil, "Failure preparing request")
	}

	resp, err := client.nic.GetSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		return result, autorest.NewErrorWithError(err, "network.InterfacesClient", "Get", resp, "Failure sending request")
	}

	result, err = client.nic.GetResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "network.InterfacesClient", "Get", resp, "Failure responding to request")
	}

	return result, nil
}
