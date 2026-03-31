// Copyright The Prometheus Authors
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

package outscale

import (
	"context"
	"fmt"
	"net"
	"strconv"

	osc "github.com/outscale/osc-sdk-go/v2"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/strutil"
)

const (
	vmLabelPrefix     = metaLabelPrefix + "vm_"
	vmLabelInstanceID = vmLabelPrefix + "instance_id"
	vmLabelRegion     = vmLabelPrefix + "region"
	vmLabelSubregion  = vmLabelPrefix + "subregion"
	vmLabelState      = vmLabelPrefix + "state"
	vmLabelPrivateIP  = vmLabelPrefix + "private_ip"
	vmLabelPublicIP   = vmLabelPrefix + "public_ip"
	vmLabelTag        = vmLabelPrefix + "tag_"
)

type vmDiscovery struct {
	client *osc.APIClient
	cfg    *SDConfig
}

func newVMDiscovery(conf *SDConfig) (*vmDiscovery, error) {
	client, err := loadClient(conf)
	if err != nil {
		return nil, err
	}
	return &vmDiscovery{
		client: client,
		cfg:    conf,
	}, nil
}

func (d *vmDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	secretKey, err := d.cfg.SecretKeyValue()
	if err != nil {
		return nil, err
	}

	// Merge request context with auth context so the SDK signs requests.
	ctx = context.WithValue(ctx, osc.ContextAWSv4, osc.AWSv4{
		AccessKey: d.cfg.AccessKey,
		SecretKey: string(secretKey),
	})

	tg := &targetgroup.Group{Source: d.cfg.Region}

	var allVms []osc.Vm
	req := d.client.VmApi.ReadVms(ctx).ReadVmsRequest(osc.ReadVmsRequest{})

	for {
		resp, _, err := req.Execute()
		if err != nil {
			return nil, fmt.Errorf("outscale ReadVms: %w", err)
		}
		if resp.Vms != nil {
			allVms = append(allVms, *resp.Vms...)
		}
		if resp.NextPageToken == nil || *resp.NextPageToken == "" {
			break
		}
		req = d.client.VmApi.ReadVms(ctx).ReadVmsRequest(osc.ReadVmsRequest{NextPageToken: resp.NextPageToken})
	}

	tg.Targets = vmsToLabelSets(allVms, d.cfg)
	return []*targetgroup.Group{tg}, nil
}

// vmsToLabelSets converts Outscale VMs into target label sets.
func vmsToLabelSets(vms []osc.Vm, cfg *SDConfig) []model.LabelSet {
	var out []model.LabelSet
	for _, vm := range vms {
		var addr string
		switch {
		case vm.PrivateIp != nil && *vm.PrivateIp != "":
			addr = net.JoinHostPort(*vm.PrivateIp, strconv.Itoa(cfg.Port))
		case vm.PublicIp != nil && *vm.PublicIp != "":
			addr = net.JoinHostPort(*vm.PublicIp, strconv.Itoa(cfg.Port))
		default:
			continue
		}

		vmID := ""
		if vm.VmId != nil {
			vmID = *vm.VmId
		}
		state := ""
		if vm.State != nil {
			state = *vm.State
		}

		labels := model.LabelSet{
			model.AddressLabel: model.LabelValue(addr),
			vmLabelInstanceID:  model.LabelValue(vmID),
			vmLabelRegion:      model.LabelValue(cfg.Region),
			vmLabelState:       model.LabelValue(state),
		}

		if vm.Placement != nil && vm.Placement.SubregionName != nil {
			labels[vmLabelSubregion] = model.LabelValue(*vm.Placement.SubregionName)
		}
		if vm.PrivateIp != nil {
			labels[vmLabelPrivateIP] = model.LabelValue(*vm.PrivateIp)
		}
		if vm.PublicIp != nil {
			labels[vmLabelPublicIP] = model.LabelValue(*vm.PublicIp)
		}

		if vm.Tags != nil {
			for _, t := range *vm.Tags {
				if t.Key == "" || t.Value == "" {
					continue
				}
				name := strutil.SanitizeLabelName(t.Key)
				labels[vmLabelTag+model.LabelName(name)] = model.LabelValue(t.Value)
			}
		}

		out = append(out, labels)
	}
	return out
}
