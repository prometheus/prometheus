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
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/go-autorest/autorest/azure"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
	yaml_util "github.com/prometheus/prometheus/util/yaml"
)

const (
	azureLabel                     = model.MetaLabelPrefix + "azure_"
	azureLabelMachineID            = azureLabel + "machine_id"
	azureLabelMachineResourceGroup = azureLabel + "machine_resource_group"
	azureLabelMachineName          = azureLabel + "machine_name"
	azureLabelMachineOSType        = azureLabel + "machine_os_type"
	azureLabelMachineLocation      = azureLabel + "machine_location"
	azureLabelMachinePrivateIP     = azureLabel + "machine_private_ip"
	azureLabelMachineTag           = azureLabel + "machine_tag_"
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
		Port:            80,
		RefreshInterval: model.Duration(5 * time.Minute),
	}
)

// SDConfig is the configuration for Azure based service discovery.
type SDConfig struct {
	Port            int                `yaml:"port"`
	SubscriptionID  string             `yaml:"subscription_id"`
	TenantID        string             `yaml:"tenant_id,omitempty"`
	ClientID        string             `yaml:"client_id,omitempty"`
	ClientSecret    config_util.Secret `yaml:"client_secret,omitempty"`
	RefreshInterval model.Duration     `yaml:"refresh_interval,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.SubscriptionID == "" {
		return fmt.Errorf("Azure SD configuration requires a subscription_id")
	}

	return yaml_util.CheckOverflow(c.XXX, "azure_sd_config")
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
	nic network.InterfacesClient
	vm  compute.VirtualMachinesClient
}

// createAzureClient is a helper function for creating an Azure compute client to ARM.
func createAzureClient(cfg SDConfig) (azureClient, error) {
	var c azureClient
	oauthConfig, err := azure.PublicCloud.OAuthConfigForTenant(cfg.TenantID)
	if err != nil {
		return azureClient{}, err
	}
	spt, err := azure.NewServicePrincipalToken(*oauthConfig, cfg.ClientID, string(cfg.ClientSecret), azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return azureClient{}, err
	}

	c.vm = compute.NewVirtualMachinesClient(cfg.SubscriptionID)
	c.vm.Authorizer = spt

	c.nic = network.NewInterfacesClient(cfg.SubscriptionID)
	c.nic.Authorizer = spt

	return c, nil
}

// azureResource represents a resource identifier in Azure.
type azureResource struct {
	Name          string
	ResourceGroup string
}

// Create a new azureResource object from an ID string.
func newAzureResourceFromID(id string, logger log.Logger) (azureResource, error) {
	// Resource IDs have the following format.
	// /subscriptions/SUBSCRIPTION_ID/resourceGroups/RESOURCE_GROUP/providers/PROVIDER/TYPE/NAME
	s := strings.Split(id, "/")
	if len(s) != 9 {
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

	var machines []compute.VirtualMachine
	result, err := client.vm.ListAll()
	if err != nil {
		return tg, fmt.Errorf("could not list virtual machines: %s", err)
	}
	machines = append(machines, *result.Value...)

	// If we still have results, keep going until we have no more.
	for result.NextLink != nil {
		result, err = client.vm.ListAllNextResults(result)
		if err != nil {
			return tg, fmt.Errorf("could not list virtual machines: %s", err)
		}
		machines = append(machines, *result.Value...)
	}
	level.Debug(d.logger).Log("msg", "Found virtual machines during Azure discovery.", "count", len(machines))

	// We have the slice of machines. Now turn them into targets.
	// Doing them in go routines because the network interface calls are slow.
	type target struct {
		labelSet model.LabelSet
		err      error
	}

	ch := make(chan target, len(machines))
	for i, vm := range machines {
		go func(i int, vm compute.VirtualMachine) {
			r, err := newAzureResourceFromID(*vm.ID, d.logger)
			if err != nil {
				ch <- target{labelSet: nil, err: err}
				return
			}

			labels := model.LabelSet{
				azureLabelMachineID:            model.LabelValue(*vm.ID),
				azureLabelMachineName:          model.LabelValue(*vm.Name),
				azureLabelMachineOSType:        model.LabelValue(vm.Properties.StorageProfile.OsDisk.OsType),
				azureLabelMachineLocation:      model.LabelValue(*vm.Location),
				azureLabelMachineResourceGroup: model.LabelValue(r.ResourceGroup),
			}

			if vm.Tags != nil {
				for k, v := range *vm.Tags {
					name := strutil.SanitizeLabelName(k)
					labels[azureLabelMachineTag+model.LabelName(name)] = model.LabelValue(*v)
				}
			}

			// Get the IP address information via separate call to the network provider.
			for _, nic := range *vm.Properties.NetworkProfile.NetworkInterfaces {
				r, err := newAzureResourceFromID(*nic.ID, d.logger)
				if err != nil {
					ch <- target{labelSet: nil, err: err}
					return
				}
				networkInterface, err := client.nic.Get(r.ResourceGroup, r.Name, "")
				if err != nil {
					level.Error(d.logger).Log("msg", "Unable to get network interface", "name", r.Name, "err", err)
					ch <- target{labelSet: nil, err: err}
					// Get out of this routine because we cannot continue without a network interface.
					return
				}

				// Unfortunately Azure does not return information on whether a VM is deallocated.
				// This information is available via another API call however the Go SDK does not
				// yet support this. On deallocated machines, this value happens to be nil so it
				// is a cheap and easy way to determine if a machine is allocated or not.
				if networkInterface.Properties.Primary == nil {
					level.Debug(d.logger).Log("msg", "Skipping deallocated virtual machine", "machine", *vm.Name)
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
						err = fmt.Errorf("unable to find a private IP for VM %s", *vm.Name)
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
