// Copyright 2018 The Prometheus Authors
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

package vsphere

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/mo"
)

const (
	vsphereLabelVirtualMachineID                  = vsphereLabel + "virtual_machine_id"
	vsphereLabelVirtualMachineName                = vsphereLabel + "virtual_machine_name"
	vsphereLabelVirtualMachineStatus              = vsphereLabel + "virtual_machine_status"
	vsphereLabelVirtualMachineDatacenter          = vsphereLabel + "virtual_machine_datacenter"
	vsphereLabelVirtualMachineMetricsProxyAddress = vsphereLabel + "virtual_machine_metrics_proxy_address"
	vsphereLabelVirtualMachineMetricsProxyPath    = vsphereLabel + "virtual_machine_metrics_proxy_path"
	vsphereLabelVirtualMachineHost                = vsphereLabel + "virtual_machine_owning_host"
	vsphereLabelVirtualMachineDatastoreList       = vsphereLabel + "virtual_machine_datastore_list"
	vsphereLabelVirtualMachineTags                = vsphereLabel + "vm_tags"
)

// VirtualMachineDiscovery periodically performs vSphere-SD requests. It implements
// the Discoverer interface.
type VirtualMachineDiscovery struct {
	client   *Client
	interval time.Duration
	logger   log.Logger
}

// NewVirtualMachineDiscovery returns a new VSphere VirtualMachine Discovery which periodically
// refreshes its targets.
func NewVirtualMachineDiscovery(conf *SDConfig, logger log.Logger) *VirtualMachineDiscovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &VirtualMachineDiscovery{
		client: &Client{
			config: conf,
		},
		interval: time.Duration(conf.RefreshInterval),
		logger:   logger,
	}
}

// Run implements the Discoverer interface.
func (d *VirtualMachineDiscovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
	} else {
		select {
		case ch <- []*targetgroup.Group{tg}:
		case <-ctx.Done():
			return
		}
	}

	for {
		select {
		case <-ticker.C:
			tg, err := d.refresh()
			if err != nil {
				level.Error(d.logger).Log("msg", "Refresh failed", "err", err)
				continue
			}

			select {
			case ch <- []*targetgroup.Group{tg}:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *VirtualMachineDiscovery) refresh() (tg *targetgroup.Group, err error) {
	t0 := time.Now()

	defer func() {
		vsphereSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			vsphereSDRefreshFailuresCount.Inc()
		}
	}()

	// Connect and login to ESX or vCenter
	err = d.client.getClient()
	if err != nil {
		return nil, err
	}
	if d.client.config.NewConnections {
		defer d.client.logout()
	}

	tg = &targetgroup.Group{
		Source: "vsphere_virtualmachine_" + d.client.config.VCenterAddress,
	}

	finder := find.NewFinder(d.client.vClient.Client, false)

	dcs, err := finder.DatacenterList(*d.client.ctx, "*")
	if err != nil {
		return nil, err
	}

	for _, dc := range dcs {
		finder.SetDatacenter(dc)

		dcName := dc.InventoryPath
		if dcName[0] == '/' {
			dcName = dcName[1:]
		}

		virtualmachineList, err := finder.VirtualMachineList(*d.client.ctx, "*")
		if err != nil {
			continue
		}

		for _, virtualmachine := range virtualmachineList {
			var vm mo.VirtualMachine
			err = virtualmachine.Properties(*d.client.ctx, virtualmachine.Reference(), []string{"config", "summary", "runtime"}, &vm)
			if err != nil {
				continue
			}

			metricsAddress := d.client.config.MetricsProxyAddress + ":" + strconv.Itoa(d.client.config.MetricsProxyPort)
			metricsPath := "/datacenter/" + dcName + "/vm/" + vm.Config.Name + "/metrics"

			// NOTE: list is , delimited with a comma before the first element and after the last
			var dsBuffer bytes.Buffer
			dsBuffer.WriteString(",")
			for _, ds := range vm.Datastore {
				dsBuffer.WriteString(ds.Value)
				dsBuffer.WriteString(",")
			}

			labels := model.LabelSet{
				vsphereLabelVirtualMachineID:                  model.LabelValue(vm.Self.Value),
				vsphereLabelVirtualMachineName:                model.LabelValue(vm.Config.Name),
				vsphereLabelVirtualMachineStatus:              model.LabelValue(vm.OverallStatus),
				vsphereLabelVirtualMachineDatacenter:          model.LabelValue(dcName),
				vsphereLabelVirtualMachineMetricsProxyAddress: model.LabelValue(metricsAddress),
				vsphereLabelVirtualMachineMetricsProxyPath:    model.LabelValue(metricsPath),
				vsphereLabelVirtualMachineHost:                model.LabelValue(vm.Runtime.Host.Value),
				vsphereLabelVirtualMachineDatastoreList:       model.LabelValue(dsBuffer.String()),
				//vsphereLabelVirtualMachineTags: ,
			}

			tg.Targets = append(tg.Targets, labels)
		}
	}

	return tg, nil
}
