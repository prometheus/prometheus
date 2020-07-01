package hcloud

import (
	"context"
	"fmt"
	"github.com/hetznercloud/hcloud-go/hcloud"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"net"
	"net/http"
	"strconv"

	"github.com/go-kit/kit/log"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/discovery/refresh"
	"time"
)

const (
	hcloudLabelID          = model.MetaLabelPrefix + "hcloud_id"
	hcloudLabelName        = model.MetaLabelPrefix + "hcloud_name"
	hcloudLabelImage       = model.MetaLabelPrefix + "hcloud_image"
	hcloudLabelPublicIPv4  = model.MetaLabelPrefix + "hcloud_ipv4"
	hcloudLabelPrivateIPv4 = model.MetaLabelPrefix + "hcloud_private_ipv4"
	hcloudLabelNetwork     = model.MetaLabelPrefix + "hcloud_network"
	hcloudLabelLocation    = model.MetaLabelPrefix + "hcloud_location"
	hcloudLabelDatacenter  = model.MetaLabelPrefix + "hcloud_datacenter"
	hcloudLabelCPUCores    = model.MetaLabelPrefix + "hcloud_cpu_cores"
	hcloudLabelCPUType     = model.MetaLabelPrefix + "hcloud_cpu_type"
	hcloudLabelMemory      = model.MetaLabelPrefix + "hcloud_memory_size"
	hcloudLabelDisk        = model.MetaLabelPrefix + "hcloud_disk_size"
	hcloudLabelType        = model.MetaLabelPrefix + "hcloud_server_type"
)

// DefaultSDConfig is the default Hetzner Cloudc SD configuration.
var DefaultSDConfig = SDConfig{
	Port:            80,
	RefreshInterval: model.Duration(60 * time.Second),
	Network:         "",
}

// SDConfig is the configuration for Hetzner Cloud based service discovery.
type SDConfig struct {
	HTTPClientConfig config_util.HTTPClientConfig `yaml:",inline"`

	RefreshInterval model.Duration `yaml:"refresh_interval"`
	Port            int            `yaml:"port"`
	Network         string         `yaml:"hcloud_network"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	return nil
}

// Discovery periodically performs Hetzner Cloud requests. It implements
// the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client  *hcloud.Client
	port    int
	network *hcloud.Network
}

// NewDiscovery returns a new Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	d := &Discovery{
		port: conf.Port,
	}

	rt, err := config_util.NewRoundTripperFromConfig(conf.HTTPClientConfig, "hcloud_sd", false)
	if err != nil {
		return nil, err
	}
	opts := []hcloud.ClientOption{
		hcloud.WithToken(string(conf.HTTPClientConfig.BearerToken)),
		hcloud.WithApplication("Prometheus", version.Version),
		hcloud.WithHTTPClient(&http.Client{
			Transport: rt,
			Timeout:   5 * time.Duration(conf.RefreshInterval),
		}),
		hcloud.WithPollInterval(time.Duration(conf.RefreshInterval)),
	}
	d.client = hcloud.NewClient(
		opts...,
	)
	if conf.Network != "" {
		d.network, _, err = d.client.Network.Get(context.Background(), conf.Network)
		if err != nil {
			return nil, err
		}
		if d.network == nil {
			return nil, fmt.Errorf("error finding requested network: %s", conf.Network)
		}
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"hcloud",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d, nil
}
func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	servers, err := d.client.Server.All(ctx)
	if err != nil {
		return nil, err
	}
	var targets []model.LabelSet
	for _, server := range servers {
		labels := model.LabelSet{
			hcloudLabelID:         model.LabelValue(fmt.Sprintf("%d", server.ID)),
			hcloudLabelName:       model.LabelValue(server.Name),
			hcloudLabelLocation:   model.LabelValue(server.Datacenter.Location.Name),
			hcloudLabelDatacenter: model.LabelValue(server.Datacenter.Name),
			hcloudLabelPublicIPv4: model.LabelValue(server.PublicNet.IPv4.IP.String()),
			hcloudLabelType:       model.LabelValue(server.ServerType.Name),
			hcloudLabelCPUCores:   model.LabelValue(fmt.Sprintf("%d", server.ServerType.Cores)),
			hcloudLabelCPUType:    model.LabelValue(server.ServerType.CPUType),
			hcloudLabelMemory:     model.LabelValue(fmt.Sprintf("%v", server.ServerType.Memory)),
			hcloudLabelDisk:       model.LabelValue(fmt.Sprintf("%d", server.ServerType.Disk)),

			model.AddressLabel: model.LabelValue(net.JoinHostPort(server.PublicNet.IPv4.IP.String(), strconv.FormatUint(uint64(d.port), 10))),
		}

		if server.Image.Name == "" {
			labels[hcloudLabelImage] = model.LabelValue(server.Image.Description)
		} else {
			labels[hcloudLabelImage] = model.LabelValue(server.Image.Name)
		}

		if d.network != nil {
			for _, privateNet := range server.PrivateNet {
				if privateNet.Network.ID == d.network.ID {
					labels[hcloudLabelPrivateIPv4] = model.LabelValue(privateNet.IP.String())
					labels[hcloudLabelNetwork] = model.LabelValue(d.network.Name)
				}
			}
		}
		targets = append(targets, labels)
	}
	return []*targetgroup.Group{&targetgroup.Group{Source: "hcloud", Targets: targets}}, nil
}
