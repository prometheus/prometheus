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
	"github.com/scaleway/scaleway-sdk-go/api/baremetal/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

type baremetalDiscovery struct {
	*refresh.Discovery
	client     *scw.Client
	port       int
	zone       string
	project    string
	accessKey  string
	secretKey  string
	nameFilter string
	tagsFilter []string
}

const (
	baremetalLabelPrefix = metaLabelPrefix + "baremetal_"

	baremetalIDLabel         = baremetalLabelPrefix + "id"
	baremetalPublicIPv4Label = baremetalLabelPrefix + "public_ipv4"
	baremetalPublicIPv6Label = baremetalLabelPrefix + "public_ipv6"
	baremetalNameLabel       = baremetalLabelPrefix + "name"
	baremetalOSNameLabel     = baremetalLabelPrefix + "os_name"
	baremetalOSVersionLabel  = baremetalLabelPrefix + "os_version"
	baremetalProjectLabel    = baremetalLabelPrefix + "project_id"
	baremetalStatusLabel     = baremetalLabelPrefix + "status"
	baremetalTagsLabel       = baremetalLabelPrefix + "tags"
	baremetalTypeLabel       = baremetalLabelPrefix + "type"
	baremetalZoneLabel       = baremetalLabelPrefix + "zone"
)

func newBaremetalDiscovery(conf *SDConfig) (*baremetalDiscovery, error) {
	d := &baremetalDiscovery{
		port:       conf.Port,
		zone:       conf.Zone,
		project:    conf.Project,
		accessKey:  conf.AccessKey,
		secretKey:  string(conf.SecretKey),
		nameFilter: conf.NameFilter,
		tagsFilter: conf.TagsFilter,
	}

	rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "scaleway_sd")
	if err != nil {
		return nil, err
	}

	if conf.SecretKeyFile != "" {
		rt, err = newAuthTokenFileRoundTripper(conf.SecretKeyFile, rt)
		if err != nil {
			return nil, err
		}
	}

	profile, err := loadProfile(conf)
	if err != nil {
		return nil, err
	}

	d.client, err = scw.NewClient(
		scw.WithHTTPClient(&http.Client{
			Transport: rt,
			Timeout:   time.Duration(conf.RefreshInterval),
		}),
		scw.WithUserAgent(version.PrometheusUserAgent()),
		scw.WithProfile(profile),
	)
	if err != nil {
		return nil, fmt.Errorf("error setting up scaleway client: %w", err)
	}

	return d, nil
}

func (d *baremetalDiscovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	api := baremetal.NewAPI(d.client)

	req := &baremetal.ListServersRequest{}

	if d.nameFilter != "" {
		req.Name = scw.StringPtr(d.nameFilter)
	}

	if d.tagsFilter != nil {
		req.Tags = d.tagsFilter
	}

	servers, err := api.ListServers(req, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	offers, err := api.ListOffers(&baremetal.ListOffersRequest{}, scw.WithAllPages())
	if err != nil {
		return nil, err
	}

	osFullList, err := api.ListOS(&baremetal.ListOSRequest{}, scw.WithAllPages())
	if err != nil {
		return nil, err
	}

	var targets []model.LabelSet
	for _, server := range servers.Servers {
		labels := model.LabelSet{
			baremetalIDLabel:      model.LabelValue(server.ID),
			baremetalNameLabel:    model.LabelValue(server.Name),
			baremetalZoneLabel:    model.LabelValue(server.Zone.String()),
			baremetalStatusLabel:  model.LabelValue(server.Status),
			baremetalProjectLabel: model.LabelValue(server.ProjectID),
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
			switch v := ip.Version.String(); v {
			case "IPv4":
				if _, ok := labels[baremetalPublicIPv4Label]; ok {
					// If the server has multiple IPv4, we only take the first one.
					// This should not happen.
					continue
				}
				labels[baremetalPublicIPv4Label] = model.LabelValue(ip.Address.String())

				// We always default the __address__ to IPv4.
				addr := net.JoinHostPort(ip.Address.String(), strconv.FormatUint(uint64(d.port), 10))
				labels[model.AddressLabel] = model.LabelValue(addr)
			case "IPv6":
				if _, ok := labels[baremetalPublicIPv6Label]; ok {
					// If the server has multiple IPv6, we only take the first one.
					// This should not happen.
					continue
				}
				labels[baremetalPublicIPv6Label] = model.LabelValue(ip.Address.String())
				if _, ok := labels[model.AddressLabel]; !ok {
					// This server does not have an IPv4 or we have not parsed it
					// yet.
					addr := net.JoinHostPort(ip.Address.String(), strconv.FormatUint(uint64(d.port), 10))
					labels[model.AddressLabel] = model.LabelValue(addr)
				}
			default:
				return nil, fmt.Errorf("unknown IP version: %s", v)
			}
		}
		targets = append(targets, labels)
	}
	return []*targetgroup.Group{{Source: "scaleway", Targets: targets}}, nil
}
