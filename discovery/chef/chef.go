package main

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"

	"github.com/go-chef/chef"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	chefLabel                   = model.MetaLabelPrefix + "chef_"
	chefLabelNodeID             = chefLabel + "node_id"
	chefLabelNodeURL            = chefLabel + "node_url"
	chefLabelMachineName        = chefLabel + "machine_name"
	chefLabelMachineOSType      = chefLabel + "machine_os_type"
	chefLabelMachineEnvironment = chefLabel + "machine_environment"
	chefLabelMachinePrivateIP   = chefLabel + "machine_private_ip"
	chefLabelMachinePublicIP    = chefLabel + "machine_public_ip"
	chefLabelMachineAttribute   = chefLabel + "machine_attribute_"
	chefLabelMachineTag         = chefLabel + "machine_tag"
	chefLabelMachineRole        = chefLabel + "machine_role"
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// DefaultSDConfig is the default Azure SD configuration.
var DefaultSDConfig = SDConfig{
	Port:            9090,
	RefreshInterval: model.Duration(5 * time.Minute),
	IgnoreSSL:       false,
}

// SDConfig is the configuration for Azure based service discovery.
type SDConfig struct {
	Port            int                `yaml:"port"`
	ChefServer      string             `yaml:"chef_server"`
	UserID          string             `yaml:"user_id,omitempty"`
	UserKey         config_util.Secret `yaml:"user_key,omitempty"`
	RefreshInterval model.Duration     `yaml:"refresh_interval,omitempty"`
	IgnoreSSL       bool               `yaml:"ignore_ssl,omitempty"`
}

type ChefClient struct {
	*chef.Client
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "chef" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger), nil
}

func validateAuthParam(param, name string) error {
	if len(param) == 0 {
		return errors.Errorf("chef SD configuration requires a %s", name)
	}
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}

	if err = validateAuthParam(c.UserID, "user_id"); err != nil {
		return err
	}
	if err = validateAuthParam(string(c.UserKey), "user_key"); err != nil {
		return err
	}
	if err = validateAuthParam(c.ChefServer, "chef_server"); err != nil {
		return err
	}

	return nil
}

type Discovery struct {
	*refresh.Discovery
	logger log.Logger
	cfg    *SDConfig
	port   int
}

// NewDiscovery returns a new AzureDiscovery which periodically refreshes its targets.
func NewDiscovery(cfg *SDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	d := &Discovery{
		cfg:    cfg,
		port:   cfg.Port,
		logger: logger,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"chef",
		time.Duration(cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

// createAzureClient is a helper function for creating an Azure compute client to ARM.
func createChefClient(cfg SDConfig) (*ChefClient, error) {
	client, err := chef.NewClient(&chef.Config{
		Name:    cfg.UserID,
		Key:     string(cfg.UserKey),
		SkipSSL: cfg.IgnoreSSL,
		BaseURL: cfg.ChefServer,
	})
	if err != nil {
		return &ChefClient{}, err
	}

	return &ChefClient{client}, nil
}

// virtualMachine represents an Azure virtual machine (which can also be created by a VMSS)
type virtualMachine struct {
	ID        string
	URL       string
	Attribute map[string]interface{}
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	defer level.Debug(d.logger).Log("msg", "Azure discovery completed")

	client, err := createChefClient(*d.cfg)
	if err != nil {
		return nil, errors.Wrap(err, "could not create Azure client")
	}

	nodes, err := client.getNodes(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get virtual machines")
	}

	level.Debug(d.logger).Log("msg", "Found virtual machines during Azure discovery.", "count", len(nodes))

	tg := &targetgroup.Group{}
	for _, node := range nodes {
		tg.Targets = append(tg.Targets, model.LabelSet{
			model.AddressLabel:          model.LabelValue(net.JoinHostPort(node.Attribute["ipaddress"].(string), fmt.Sprintf("%d", d.port))),
			chefLabelNodeID:             model.LabelValue(node.ID),
			chefLabelNodeURL:            model.LabelValue(node.URL),
			chefLabelMachineName:        model.LabelValue(node.Attribute["hostname"].(string)),
			chefLabelMachineOSType:      model.LabelValue(node.Attribute["os"].(string)),
			chefLabelMachineEnvironment: model.LabelValue(node.Attribute["chef_environment"].(string)),
			chefLabelMachineTag:         model.LabelValue(strings.Join(unwrapArray(node.Attribute["tags"]), ",")),
			chefLabelMachineRole:        model.LabelValue(strings.Join(unwrapArray(node.Attribute["roles"]), ",")),
		})
	}

	return []*targetgroup.Group{tg}, nil
}

func (client *ChefClient) getNodes(ctx context.Context) ([]virtualMachine, error) {
	var nodes []virtualMachine

	result, err := client.Nodes.List()
	if err != nil {
		return nil, errors.Wrap(err, "could not list virtual machines")
	}

	for node, url := range result {
		v, err := client.mapFromNode(node, url)
		if err != nil {
			return nil, errors.Wrap(err, "could not list virtual machines")
		}
		nodes = append(nodes, v)
	}

	return nodes, nil
}

func (client *ChefClient) mapFromNode(node string, url string) (virtualMachine, error) {
	nodeCheck, err := client.Nodes.Get("azl-prd-hyb-03")
	if err != nil {
		return virtualMachine{}, errors.Wrap(err, "could not get node attributes")
	}

	// All Chef attribute types ordered by precedence (Last one wins)
	attributeTypes := [4]map[string]interface{}{nodeCheck.DefaultAttributes, nodeCheck.NormalAttributes, nodeCheck.OverrideAttributes, nodeCheck.AutomaticAttributes}

	chefAttribute := map[string]interface{}{}
	for _, value := range attributeTypes {
		for k, v := range value {
			chefAttribute[k] = v
		}
	}

	return virtualMachine{
		ID:        node,
		URL:       url,
		Attribute: chefAttribute,
	}, nil
}

func unwrapArray(t interface{}) []string {
	arr := []string{}
	switch reflect.TypeOf(t).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(t)

		for i := 0; i < s.Len(); i++ {
			fmt.Println(s.Index(i))
			arr = append(arr, s.Index(i).Interface().(string))
		}
	}
	return arr
}
