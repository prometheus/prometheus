// Copyright 2021 The Prometheus Authors
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

package scaleway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/scaleway/scaleway-sdk-go/api/baremetal/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

type baremetalDiscovery struct {
	*refresh.Discovery
	Client    *scw.Client
	Port      int
	Zone      string
	Project   string
	AccessKey string
	SecretKey string
	Name      string
	Tags      []string
}

const (
	baremetalLabelPrefix = metaLabelPrefix + "baremetal_"

	baremetalIDLabel         = baremetalLabelPrefix + "id"
	baremetalIPIsPublicLabel = baremetalLabelPrefix + "ip_is_public"
	baremetalIPVersionLabel  = baremetalLabelPrefix + "ip_version"
	baremetalNameLabel       = baremetalLabelPrefix + "name"
	baremetalOSNameLabel     = baremetalLabelPrefix + "os_name"
	baremetalOSVersionLabel  = baremetalLabelPrefix + "os_version"
	baremetalStatusLabel     = baremetalLabelPrefix + "status"
	baremetalTagsLabel       = baremetalLabelPrefix + "tags"
	baremetalTypeLabel       = baremetalLabelPrefix + "type"
	baremetalZoneLabel       = baremetalLabelPrefix + "zone"
)

func newBaremetalDiscovery(conf *SDConfig) (*baremetalDiscovery, error) {
	d := &baremetalDiscovery{
		Port:      conf.Port,
		Zone:      conf.Zone,
		Project:   conf.Project,
		AccessKey: conf.AccessKey,
		SecretKey: string(conf.SecretKey),
		Name:      conf.FilterName,
		Tags:      conf.FilterTags,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "scaleway_sd", false, false)
	if err != nil {
		return nil, err
	}

	profile, err := LoadProfile(conf)
	if err != nil {
		return nil, err
	}
	d.Client, err = scw.NewClient(
		scw.WithHTTPClient(&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		}),
		scw.WithUserAgent(fmt.Sprintf("Prometheus/%s", version.Version)),
		scw.WithProfile(profile),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up scaleway client: %w", err)
	}

	return d, nil
}

func (d *baremetalDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	api := baremetal.NewAPI(d.Client)

	req := &baremetal.ListServersRequest{
		Zone: scw.Zone(d.Zone),
	}

	if d.Project != "" {
		req.ProjectID = scw.StringPtr(d.Project)
	}

	if d.Name != "" {
		req.Name = scw.StringPtr(d.Name)
	}

	if d.Tags != nil {
		req.Tags = d.Tags
	}

	servers, err := api.ListServers(req, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	offers, err := api.ListOffers(&baremetal.ListOffersRequest{
		Zone: req.Zone,
	}, scw.WithAllPages())
	if err != nil {
		return nil, err
	}

	osFullList, err := api.ListOS(&baremetal.ListOSRequest{
		Zone: req.Zone,
	}, scw.WithAllPages())
	if err != nil {
		return nil, err
	}

	var targets []model.LabelSet
	for _, server := range servers.Servers {
		labels := model.LabelSet{
			baremetalIDLabel:     model.LabelValue(server.ID),
			baremetalNameLabel:   model.LabelValue(server.Name),
			baremetalZoneLabel:   model.LabelValue(server.Zone.String()),
			baremetalStatusLabel: model.LabelValue(server.Status),
		}

		for _, offer := range offers.Offers {
			if server.OfferID == offer.ID {
				labels[baremetalTypeLabel] = model.LabelValue(offer.Name)
				break
			}
		}

		if server.Install != nil {
			for _, os := range osFullList.Os {
				if server.Install.OsID == os.ID {
					labels[baremetalOSNameLabel] = model.LabelValue(os.Name)
					labels[baremetalOSVersionLabel] = model.LabelValue(os.Version)
					break
				}
			}
		}

		if len(server.Tags) > 0 {
			// We surround the separated list with the separator as well. This way regular expressions
			// in relabeling rules don't have to consider tag positions.
			tags := separator + strings.Join(server.Tags, separator) + separator
			labels[baremetalTagsLabel] = model.LabelValue(tags)
		}

		for _, ip := range server.IPs {
			labels[baremetalIPVersionLabel] = model.LabelValue(ip.Version.String())
			labels[baremetalIPIsPublicLabel] = "true"

			addr := net.JoinHostPort(ip.Address.String(), strconv.FormatUint(uint64(d.Port), 10))
			labels[model.AddressLabel] = model.LabelValue(addr)

			targets = append(targets, labels.Clone())
		}
	}
	return []*targetgroup.Group{{Source: "scaleway", Targets: targets}}, nil
}
