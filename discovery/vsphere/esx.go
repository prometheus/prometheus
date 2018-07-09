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
	vsphereLabelEsxID                  = vsphereLabel + "esx_id"
	vsphereLabelEsxName                = vsphereLabel + "esx_name"
	vsphereLabelEsxVersion             = vsphereLabel + "esx_version"
	vsphereLabelEsxStatus              = vsphereLabel + "esx_status"
	vsphereLabelEsxDatacenter          = vsphereLabel + "esx_datacenter"
	vsphereLabelEsxMetricsProxyAddress = vsphereLabel + "esx_metrics_proxy_address"
	vsphereLabelEsxMetricsProxyPath    = vsphereLabel + "esx_metrics_proxy_path"
	vsphereLabelEsxDatastoreList       = vsphereLabel + "esx_datastore_list"
	vsphereLabelEsxVirtualMachineList  = vsphereLabel + "esx_virtual_machine_list"
	vsphereLabelEsxTags                = vsphereLabel + "esx_tags"
)

// EsxDiscovery periodically performs vSphere-SD requests. It implements
// the Discoverer interface.
type EsxDiscovery struct {
	client   *Client
	interval time.Duration
	logger   log.Logger
}

// NewEsxDiscovery returns a new VSphere ESX Discovery which periodically
// refreshes its targets.
func NewEsxDiscovery(conf *SDConfig, logger log.Logger) *EsxDiscovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &EsxDiscovery{
		client: &Client{
			config: conf,
		},
		interval: time.Duration(conf.RefreshInterval),
		logger:   logger,
	}
}

// Run implements the Discoverer interface.
func (d *EsxDiscovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
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

func (d *EsxDiscovery) refresh() (tg *targetgroup.Group, err error) {
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
		Source: "vsphere_esx_" + d.client.config.VCenterAddress,
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

		hostList, err := finder.HostSystemList(*d.client.ctx, "*/*")
		if err != nil {
			continue
		}

		for _, host := range hostList {
			var hs mo.HostSystem
			err = host.Properties(*d.client.ctx, host.Reference(), []string{"summary"}, &hs)
			if err != nil {
				continue
			}

			metricsAddress := d.client.config.MetricsProxyAddress + ":" + strconv.Itoa(d.client.config.MetricsProxyPort)
			metricsPath := "/datacenter/" + dcName + "/host/" + hs.Summary.Config.Name + "/metrics"

			// NOTE: list is , delimited with a comma before the first element and after the last
			var vmBuffer bytes.Buffer
			vmBuffer.WriteString(",")
			for _, vm := range hs.Vm {
				vmBuffer.WriteString(vm.Value)
				vmBuffer.WriteString(",")
			}

			// NOTE: list is , delimited with a comma before the first element and after the last
			var dsBuffer bytes.Buffer
			dsBuffer.WriteString(",")
			for _, ds := range hs.Datastore {
				vmBuffer.WriteString(ds.Value)
				vmBuffer.WriteString(",")
			}

			labels := model.LabelSet{
				vsphereLabelEsxID:                  model.LabelValue(hs.Self.Value),
				vsphereLabelEsxName:                model.LabelValue(hs.Summary.Config.Name),
				vsphereLabelEsxVersion:             model.LabelValue(hs.Summary.Config.Product.Version),
				vsphereLabelEsxStatus:              model.LabelValue(hs.OverallStatus),
				vsphereLabelEsxDatacenter:          model.LabelValue(dcName),
				vsphereLabelEsxMetricsProxyAddress: model.LabelValue(metricsAddress),
				vsphereLabelEsxMetricsProxyPath:    model.LabelValue(metricsPath),
				vsphereLabelEsxDatastoreList:       model.LabelValue(dsBuffer.String()),
				vsphereLabelEsxVirtualMachineList:  model.LabelValue(vmBuffer.String()),
				//vsphereLabelEsxTags: ,
			}

			tg.Targets = append(tg.Targets, labels)
		}
	}

	return tg, nil
}
