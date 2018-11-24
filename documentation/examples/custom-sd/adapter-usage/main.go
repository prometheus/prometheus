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
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/documentation/examples/custom-sd/adapter"
	"github.com/prometheus/prometheus/util/strutil"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	a             = kingpin.New("sd adapter usage", "Tool to generate file_sd target files for unimplemented SD mechanisms.")
	outputFile    = a.Flag("output.file", "Output file for file_sd compatible file.").Default("custom_sd.json").String()
	listenAddress = a.Flag("listen.address", "The address the Consul HTTP API is listening on for requests.").Default("localhost:8500").String()
	logger        log.Logger

	// addressLabel is the name for the label containing a target's address.
	addressLabel = model.MetaLabelPrefix + "consul_address"
	// nodeLabel is the name for the label containing a target's node name.
	nodeLabel = model.MetaLabelPrefix + "consul_node"
	// metaDataLabel is the prefix for the labels mapping to a target's metadata.
	metaDataLabel = model.MetaLabelPrefix + "consul_metadata_"
	// tagsLabel is the name of the label containing the tags assigned to the target.
	tagsLabel = model.MetaLabelPrefix + "consul_tags"
	// serviceLabel is the name of the label containing the service name.
	serviceLabel = model.MetaLabelPrefix + "consul_service"
	// serviceAddressLabel is the name of the label containing the (optional) service address.
	serviceAddressLabel = model.MetaLabelPrefix + "consul_service_address"
	//servicePortLabel is the name of the label containing the service port.
	servicePortLabel = model.MetaLabelPrefix + "consul_service_port"
	// datacenterLabel is the name of the label containing the datacenter ID.
	datacenterLabel = model.MetaLabelPrefix + "consul_dc"
	// serviceIDLabel is the name of the label containing the service ID.
	serviceIDLabel = model.MetaLabelPrefix + "consul_service_id"
)

// CatalogService is copied from https://github.com/hashicorp/consul/blob/master/api/catalog.go
// this struct respresents the response from a /service/<service-name> request.
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
	address          string
	refreshInterval  int
	clientDatacenter string
	tagSeparator     string
	logger           log.Logger
	oldSourceList    map[string]bool
}

func (d *discovery) parseServiceNodes(resp *http.Response, name string) (*targetgroup.Group, error) {
	var nodes []*CatalogService
	tgroup := targetgroup.Group{
		Source: name,
		Labels: make(model.LabelSet),
	}

	dec := json.NewDecoder(resp.Body)
	defer resp.Body.Close()
	err := dec.Decode(&nodes)

	if err != nil {
		return &tgroup, err
	}

	tgroup.Targets = make([]model.LabelSet, 0, len(nodes))

	for _, node := range nodes {
		// We surround the separated list with the separator as well. This way regular expressions
		// in relabeling rules don't have to consider tag positions.
		var tags = "," + strings.Join(node.ServiceTags, ",") + ","

		// If the service address is not empty it should be used instead of the node address
		// since the service may be registered remotely through a different node.
		var addr string
		if node.ServiceAddress != "" {
			addr = net.JoinHostPort(node.ServiceAddress, fmt.Sprintf("%d", node.ServicePort))
		} else {
			addr = net.JoinHostPort(node.Address, fmt.Sprintf("%d", node.ServicePort))
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
			level.Error(d.logger).Log("msg", "Error getting services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(&srvs)
		resp.Body.Close()
		if err != nil {
			level.Error(d.logger).Log("msg", "Error reading services list", "err", err)
			time.Sleep(time.Duration(d.refreshInterval) * time.Second)
			continue
		}

		var tgs []*targetgroup.Group
		// Note that we treat errors when querying specific consul services as fatal for for this
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
				level.Error(d.logger).Log("msg", "Error getting services nodes", "service", name, "err", err)
				break
			}
			tg, err := d.parseServiceNodes(resp, name)
			if err != nil {
				level.Error(d.logger).Log("msg", "Error parsing services nodes", "service", name, "err", err)
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
		if err == nil {
			// We're returning all Consul services as a single targetgroup.
			ch <- tgs
		}
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
	logger = log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC, "caller", log.DefaultCaller)

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
	sdAdapter := adapter.NewAdapter(ctx, *outputFile, "exampleSD", disc, logger)
	sdAdapter.Run()

	<-ctx.Done()
}
