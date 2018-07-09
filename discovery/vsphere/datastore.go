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
	vsphereLabelDatastoreID                  = vsphereLabel + "datastore_id"
	vsphereLabelDatastoreName                = vsphereLabel + "datastore_name"
	vsphereLabelDatastoreStatus              = vsphereLabel + "datastore_status"
	vsphereLabelDatastoreDatacenter          = vsphereLabel + "datastore_datacenter"
	vsphereLabelDatastoreMetricsProxyAddress = vsphereLabel + "datastore_metrics_proxy_address"
	vsphereLabelDatastoreMetricsProxyPath    = vsphereLabel + "datastore_metrics_proxy_path"
	vsphereLabelDatastoreHostList            = vsphereLabel + "datastore_esx_list"
	vsphereLabelDatastoreVirtualMachineList  = vsphereLabel + "datastore_virtual_machine_list"
	vsphereLabelDatastoreTags                = vsphereLabel + "datastore_tags"
)

// DatastoreDiscovery periodically performs vSphere-SD requests. It implements
// the Discoverer interface.
type DatastoreDiscovery struct {
	client   *Client
	interval time.Duration
	logger   log.Logger
}

// NewDatastoreDiscovery returns a new VSphere Datastore Discovery which periodically
// refreshes its targets.
func NewDatastoreDiscovery(conf *SDConfig, logger log.Logger) *DatastoreDiscovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &DatastoreDiscovery{
		client: &Client{
			config: conf,
		},
		interval: time.Duration(conf.RefreshInterval),
		logger:   logger,
	}
}

// Run implements the Discoverer interface.
func (d *DatastoreDiscovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
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

func (d *DatastoreDiscovery) refresh() (tg *targetgroup.Group, err error) {
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
		Source: "vsphere_datastore_" + d.client.config.VCenterAddress,
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

		datastoreList, err := finder.DatastoreList(*d.client.ctx, "*")
		if err != nil {
			continue
		}

		for _, datastore := range datastoreList {
			var ds mo.Datastore
			err = datastore.Properties(*d.client.ctx, datastore.Reference(), []string{"summary"}, &ds)
			if err != nil {
				continue
			}

			metricsAddress := d.client.config.MetricsProxyAddress + ":" + strconv.Itoa(d.client.config.MetricsProxyPort)
			metricsPath := "/datacenter/" + dcName + "/datastore/" + ds.Summary.Name + "/metrics"

			// NOTE: list is , delimited with a comma before the first element and after the last
			var vmBuffer bytes.Buffer
			vmBuffer.WriteString(",")
			for _, vm := range ds.Vm {
				vmBuffer.WriteString(vm.Value)
				vmBuffer.WriteString(",")
			}

			// NOTE: list is , delimited with a comma before the first element and after the last
			var hostBuffer bytes.Buffer
			hostBuffer.WriteString(",")
			for _, hs := range ds.Host {
				hostBuffer.WriteString(hs.Key.Value)
				hostBuffer.WriteString(",")
			}

			labels := model.LabelSet{
				vsphereLabelDatastoreID:                  model.LabelValue(ds.Self.Value),
				vsphereLabelDatastoreName:                model.LabelValue(ds.Summary.Name),
				vsphereLabelDatastoreStatus:              model.LabelValue(ds.OverallStatus),
				vsphereLabelDatastoreDatacenter:          model.LabelValue(dcName),
				vsphereLabelDatastoreMetricsProxyAddress: model.LabelValue(metricsAddress),
				vsphereLabelDatastoreMetricsProxyPath:    model.LabelValue(metricsPath),
				vsphereLabelDatastoreHostList:            model.LabelValue(hostBuffer.String()),
				vsphereLabelDatastoreVirtualMachineList:  model.LabelValue(vmBuffer.String()),
				//vsphereLabelDatastoreTags: ,
			}

			tg.Targets = append(tg.Targets, labels)
		}
	}

	return tg, nil
}
