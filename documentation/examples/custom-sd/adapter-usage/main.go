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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"

	prom_discovery "github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/documentation/examples/custom-sd/adapter"
	"github.com/prometheus/prometheus/util/strutil"
)

var (
	a             = kingpin.New("sd adapter usage", "Tool to generate file_sd target files for unimplemented SD mechanisms.")
	outputFile    = a.Flag("output.file", "Output file for file_sd compatible file.").Default("custom_sd.json").String()
	listenAddress = a.Flag("listen.address", "The address the Consul HTTP API is listening on for requests.").Default("localhost:8500").String()
	logger        *slog.Logger

	// addressLabel is the name for the label containing a target's address.
	addressLabel = model.MetaLabelPrefix + "consul_address"
	// nodeLabel is the name for the label containing a target's node name.
	nodeLabel = model.MetaLabelPrefix + "consul_node"
	// tagsLabel is the name of the label containing the tags assigned to the target.
	tagsLabel = model.MetaLabelPrefix + "consul_tags"
	// serviceAddressLabel is the name of the label containing the (optional) service address.
	serviceAddressLabel = model.MetaLabelPrefix + "consul_service_address"
	// servicePortLabel is the name of the label containing the service port.
	servicePortLabel = model.MetaLabelPrefix + "consul_service_port"
	// serviceIDLabel is the name of the label containing the service ID.
	serviceIDLabel = model.MetaLabelPrefix + "consul_service_id"
)

// CatalogService is copied from https://github.com/hashicorp/consul/blob/master/api/catalog.go
// this struct represents the response from a /service/<service-name> request.
// Consul License: https://github.com/hashicorp/consul/blob/master/LICENSE
type CatalogService struct {
	ID                       string
	Node                     string
	Address                  string
	Datacenter               string
	TaggedAddresses          map[string]string
	NodeMeta                 map[string]string
	ServiceID                string
	ServiceName              string
	ServiceAddress           string
	ServiceTags              []string
	ServicePort              int
	ServiceEnableTagOverride bool
	CreateIndex              uint64
	ModifyIndex              uint64
}

// Note: create a config struct for your custom SD type here.
type sdConfig struct {
	Address         string
	TagSeparator    string
	RefreshInterval int
}

// Note: This is the struct with your implementation of the Discoverer interface (see Run function).
// Discovery retrieves target information from a Consul server and updates them via watches.
type discovery struct {
	address         string
	refreshInterval int
	tagSeparator    string
	logger          *slog.Logger
	oldSourceList   map[string]bool
}

func (*discovery) parseServiceNodes(resp *http.Response, name string) (*targetgroup.Group, error) {
	var nodes []*CatalogService
	tgroup := targetgroup.Group{
		Source: name,
		Labels: make(model.LabelSet),
	}

	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &nodes)
	if err != nil {
		return &tgroup, err
	}

	tgroup.Targets = make([]model.LabelSet, 0, len(nodes))

	for _, node := range nodes {
		// We surround the separated list with the separator as well. This way regular expressions
		// in relabeling rules don't have to consider tag positions.
		tags := "," + strings.Join(node.ServiceTags, ",") + ","

		// If the service address is not empty it should be used instead of the node address
		// since the service may be registered remotely through a different node.
		var addr string
		if node.ServiceAddress != "" {
			addr = net.JoinHostPort(node.ServiceAddress, strconv.Itoa(node.ServicePort))
		} else {
			addr = net.JoinHostPort(node.Address, strconv.Itoa(node.ServicePort))
		}

		target := model.LabelSet{model.AddressLabel: model.LabelValue(addr)}
		labels := model.LabelSet{
			model.AddressLabel:                   model.LabelValue(addr),
			model.LabelName(addressLabel):        model.LabelValue(node.Address),
			model.LabelName(nodeLabel):           model.LabelValue(node.Node),
			model.LabelName(tagsLabel):           model.LabelValue(tags),
			model.LabelName(serviceAddressLabel): model.LabelValue(node.ServiceAddress),
			model.LabelName(servicePortLabel):    model.LabelValue(strconv.Itoa(node.ServicePort)),
			model.LabelName(serviceIDLabel):      model.LabelValue(node.ServiceID),
		}
		tgroup.Labels = labels

		// Add all key/value pairs from the node's metadata as their own labels.
		for k, v := range node.NodeMeta {
			name := strutil.SanitizeLabelName(k)
			tgroup.Labels[model.LabelName(model.MetaLabelPrefix+name)] = model.LabelValue(v)
		}

		tgroup.Targets = append(tgroup.Targets, target)
	}
	return &tgroup, nil
}

// Note: you must implement this function for your discovery implementation as part of the
// Discoverer interface. Here you should query your SD for it's list of known targets, determine
// which of those targets you care about (for example, which of Consuls known services do you want
// to scrape for metrics), and then send those targets as a target.TargetGroup to the ch channel.
func (d *discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	for c := time.Tick(time.Duration(d.refreshInterval) * time.Second); ; {
		var srvs map[string][]string
		resp, err := http.Get(fmt.Sprintf("http://%s/v1/catalog/services", d.address))
		if err != nil {
			d.logger.Error("Error getting services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		b, err := io.ReadAll(resp.Body)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err != nil {
			d.logger.Error("Error reading services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		err = json.Unmarshal(b, &srvs)
		resp.Body.Close()
		if err != nil {
			d.logger.Error("Error parsing services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		var tgs []*targetgroup.Group
		// Note that we treat errors when querying specific consul services as fatal for this
		// iteration of the time.Tick loop. It's better to have some stale targets than an incomplete
		// list of targets simply because there may have been a timeout. If the service is actually
		// gone as far as consul is concerned, that will be picked up during the next iteration of
		// the outer loop.

		newSourceList := make(map[string]bool)
		for name := range srvs {
			if name == "consul" {
				continue
			}
			resp, err := http.Get(fmt.Sprintf("http://%s/v1/catalog/service/%s", d.address, name))
			if err != nil {
				d.logger.Error("Error getting services nodes", "service", name, "err", err)
				break
			}

			tg, err := d.parseServiceNodes(resp, name)
			if err != nil {
				d.logger.Error("Error parsing services nodes", "service", name, "err", err)
				break
			}
			tgs = append(tgs, tg)
			newSourceList[tg.Source] = true
		}
		// When targetGroup disappear, send an update with empty targetList.
		for key := range d.oldSourceList {
			if !newSourceList[key] {
				tgs = append(tgs, &targetgroup.Group{
					Source: key,
				})
			}
		}
		d.oldSourceList = newSourceList
		// We're returning all Consul services as a single targetgroup.
		ch <- tgs
		// Wait for ticker or exit when ctx is closed.
		select {
		case <-c:
			continue
		case <-ctx.Done():
			return
		}
	}
}

func newDiscovery(conf sdConfig) (*discovery, error) {
	cd := &discovery{
		address:         conf.Address,
		refreshInterval: conf.RefreshInterval,
		tagSeparator:    conf.TagSeparator,
		logger:          logger,
		oldSourceList:   make(map[string]bool),
	}
	return cd, nil
}

func main() {
	a.HelpFlag.Short('h')

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Println("err: ", err)
		return
	}
	logger = promslog.New(&promslog.Config{})

	ctx := context.Background()

	// NOTE: create an instance of your new SD implementation here.
	cfg := sdConfig{
		TagSeparator:    ",",
		Address:         *listenAddress,
		RefreshInterval: 30,
	}

	disc, err := newDiscovery(cfg)
	if err != nil {
		fmt.Println("err: ", err)
	}

	if err != nil {
		logger.Error("failed to create discovery metrics", "err", err)
		os.Exit(1)
	}

	reg := prometheus.NewRegistry()
	refreshMetrics := prom_discovery.NewRefreshMetrics(reg)
	metrics, err := prom_discovery.RegisterSDMetrics(reg, refreshMetrics)
	if err != nil {
		logger.Error("failed to register service discovery metrics", "err", err)
		os.Exit(1)
	}

	sdAdapter := adapter.NewAdapter(ctx, *outputFile, "exampleSD", disc, logger, metrics, reg)
	sdAdapter.Run()

	<-ctx.Done()
}
