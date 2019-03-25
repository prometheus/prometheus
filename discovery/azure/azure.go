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
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-10-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

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
	azureLabelMachineOSType        = azureLabel + "machine_os_type"
	azureLabelMachineLocation      = azureLabel + "machine_location"
	azureLabelMachinePrivateIP     = azureLabel + "machine_private_ip"
	azureLabelMachineTag           = azureLabel + "machine_tag_"
	azureLabelMachineScaleSet      = azureLabel + "machine_scale_set"

	authMethodOAuth           = "OAuth"
	authMethodManagedIdentity = "ManagedIdentity"
)

// DefaultSDConfig is the default Azure SD configuration.
var DefaultSDConfig = SDConfig{
	Port:                 80,
	RefreshInterval:      model.Duration(5 * time.Minute),
	Environment:          azure.PublicCloud.Name,
	AuthenticationMethod: authMethodOAuth,
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
}

func validateAuthParam(param, name string) error {
	if len(param) == 0 {
		return errors.Errorf("azure SD configuration requires a %s", name)
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
		return errors.Errorf("unknown authentication_type %q. Supported types are %q or %q", c.AuthenticationMethod, authMethodOAuth, authMethodManagedIdentity)
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
	ID                string
	Name              string
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
		err := errors.Errorf("invalid ID '%s'. Refusing to create azureResource", id)
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
		return nil, errors.Wrap(err, "could not create Azure client")
	}

	machines, err := client.getVMs(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get virtual machines")
	}

	level.Debug(d.logger).Log("msg", "Found virtual machines during Azure discovery.", "count", len(machines))

	// Load the vms managed by scale sets.
	scaleSets, err := client.getScaleSets(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get virtual machine scale sets")
	}

	for _, scaleSet := range scaleSets {
		scaleSetVms, err := client.getScaleSetVMs(ctx, scaleSet)
		if err != nil {
			return nil, errors.Wrap(err, "could not get virtual machine scale set vms")
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
	for i, vm := range machines {
		go func(i int, vm virtualMachine) {
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
			for _, nicID := range vm.NetworkInterfaces {
				networkInterface, err := client.getNetworkInterfaceByID(ctx, nicID)

				if err != nil {
					level.Error(d.logger).Log("msg", "Unable to get network interface", "name", nicID, "err", err)
					ch <- target{labelSet: nil, err: err}
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
						if ip.PrivateIPAddress != nil {
							labels[azureLabelMachinePrivateIP] = model.LabelValue(*ip.PrivateIPAddress)
							address := net.JoinHostPort(*ip.PrivateIPAddress, fmt.Sprintf("%d", d.port))
							labels[model.AddressLabel] = model.LabelValue(address)
							ch <- target{labelSet: labels, err: nil}
							return
						}
						// If we made it here, we don't have a private IP which should be impossible.
						// Return an empty target and error to ensure an all or nothing situation.
						err = errors.Errorf("unable to find a private IP for VM %s", vm.Name)
						ch <- target{labelSet: nil, err: err}
						return
					}
				}
			}
		}(i, vm)
	}

	wg.Wait()
	close(ch)

	var tg targetgroup.Group
	for tgt := range ch {
		if tgt.err != nil {
			return nil, errors.Wrap(err, "unable to complete Azure service discovery")
		}
		if tgt.labelSet != nil {
			tg.Targets = append(tg.Targets, tgt.labelSet)
		}
	}

	return []*targetgroup.Group{&tg}, nil
}

func (client *azureClient) getVMs(ctx context.Context) ([]virtualMachine, error) {
	var vms []virtualMachine
	result, err := client.vm.ListAll(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not list virtual machines")
	}
	for result.NotDone() {
		for _, vm := range result.Values() {
			vms = append(vms, mapFromVM(vm))
		}
		err = result.NextWithContext(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not list virtual machines")
		}
	}

	return vms, nil
}

func (client *azureClient) getScaleSets(ctx context.Context) ([]compute.VirtualMachineScaleSet, error) {
	var scaleSets []compute.VirtualMachineScaleSet
	result, err := client.vmss.ListAll(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not list virtual machine scale sets")
	}
	for result.NotDone() {
		scaleSets = append(scaleSets, result.Values()...)
		err = result.NextWithContext(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not list virtual machine scale sets")
		}
	}

	return scaleSets, nil
}

func (client *azureClient) getScaleSetVMs(ctx context.Context, scaleSet compute.VirtualMachineScaleSet) ([]virtualMachine, error) {
	var vms []virtualMachine
	//TODO do we really need to fetch the resourcegroup this way?
	r, err := newAzureResourceFromID(*scaleSet.ID, nil)

	if err != nil {
		return nil, errors.Wrap(err, "could not parse scale set ID")
	}

	result, err := client.vmssvm.List(ctx, r.ResourceGroup, *(scaleSet.Name), "", "", "")
	if err != nil {
		return nil, errors.Wrap(err, "could not list virtual machine scale set vms")
	}
	for result.NotDone() {
		for _, vm := range result.Values() {
			vms = append(vms, mapFromVMScaleSetVM(vm, *scaleSet.Name))
		}
		err = result.NextWithContext(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not list virtual machine scale set vms")
		}
	}

	return vms, nil
}

func mapFromVM(vm compute.VirtualMachine) virtualMachine {
	osType := string(vm.StorageProfile.OsDisk.OsType)
	tags := map[string]*string{}
	networkInterfaces := []string{}

	if vm.Tags != nil {
		tags = vm.Tags
	}

	if vm.NetworkProfile != nil {
		for _, vmNIC := range *(vm.NetworkProfile.NetworkInterfaces) {
			networkInterfaces = append(networkInterfaces, *vmNIC.ID)
		}
	}

	return virtualMachine{
		ID:                *(vm.ID),
		Name:              *(vm.Name),
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

	if vm.Tags != nil {
		tags = vm.Tags
	}

	if vm.NetworkProfile != nil {
		for _, vmNIC := range *(vm.NetworkProfile.NetworkInterfaces) {
			networkInterfaces = append(networkInterfaces, *vmNIC.ID)
		}
	}

	return virtualMachine{
		ID:                *(vm.ID),
		Name:              *(vm.Name),
		Type:              *(vm.Type),
		Location:          *(vm.Location),
		OsType:            osType,
		ScaleSet:          scaleSetName,
		Tags:              tags,
		NetworkInterfaces: networkInterfaces,
	}
}

func (client *azureClient) getNetworkInterfaceByID(ctx context.Context, networkInterfaceID string) (*network.Interface, error) {
	result := network.Interface{}
	queryParameters := map[string]interface{}{
		"api-version": "2018-10-01",
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.nic.BaseURI),
		autorest.WithPath(networkInterfaceID),
		autorest.WithQueryParameters(queryParameters))
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
		return nil, autorest.NewErrorWithError(err, "network.InterfacesClient", "Get", resp, "Failure responding to request")
	}

	return &result, nil
}
