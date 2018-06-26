// Copyright 2015 The Prometheus Authors
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

package oci

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oracle/oci-go-sdk/common"
	"github.com/oracle/oci-go-sdk/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/targetgroup"
	"io/ioutil"
)

const (
	ociLabel                   = model.MetaLabelPrefix + "oci_"
	ociLabelCompartment        = ociLabel + "compartment"
	ociLabelAvailabilityDomain = ociLabel + "availability_domain"
	ociLabelInstanceID         = ociLabel + "instance_id"
	ociLabelInstanceState      = ociLabel + "instance_state"
	ociLabelPublicIP           = ociLabel + "public_ip"
	ociLabelPrivateIP          = ociLabel + "private_ip"
	ociLabelTag                = ociLabel + "tag_"
)

var (
	ociSDRefreshFailuresCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prometheus_sd_oci_refresh_failures_total",
			Help: "The number of OCI-SD scrape failures.",
		})
	ociSDRefreshDuration = prometheus.NewSummary(
		prometheus.SummaryOpts{
			Name: "prometheus_sd_oci_refresh_duration_seconds",
			Help: "The duration of a OCI-SD refresh in seconds.",
		})
	// DefaultSDConfig is the default OCI SD configuration
	DefaultSDConfig = SDConfig{}
)

// Filter is the configuration for filtering OCI instances.
type Filter struct {
	Name   string   `yaml:"name"`
	Values []string `yaml:"values"`
}

// SDConfig is the configuration for OCI based service discovery.
type SDConfig struct {
	User            string         `yaml:"user"`
	FingerPrint     string         `yaml:"fingerprint"`
	KeyFile         string         `yaml:"key_file"`
	PassPhrase      string         `yaml:"pass_phrase,omitempty"`
	Tenancy         string         `yaml:"tenancy"`
	Region          string         `yaml:"region"`
	Compartment     string         `yaml:"compartment"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.User == "" {
		return fmt.Errorf("OCI SD configuration requires a user")
	}
	if c.FingerPrint == "" {
		return fmt.Errorf("OCI SD configuration requires a fingerprint")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("OCI SD configuration requires a key file")
	}
	if c.Tenancy == "" {
		return fmt.Errorf("OCI SD configuration requires a tenancy")
	}
	if c.Region == "" {
		return fmt.Errorf("OCI SD configuration requires a region")
	}
	return nil
}

func init() {
	prometheus.MustRegister(ociSDRefreshFailuresCount)
	prometheus.MustRegister(ociSDRefreshDuration)
}

// Discovery periodically performs OCI-SD requests. It implements
// the Discoverer interface.
type Discovery struct {
	sdConfig  *SDConfig
	ociConfig common.ConfigurationProvider
	interval  time.Duration
	logger    log.Logger
}

// NewDiscovery returns a new OCI Discovery which periodically refreshes its targets.
func NewDiscovery(conf *SDConfig, logger log.Logger) (*Discovery, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	privateKey, err := loadKey(conf.KeyFile)
	if err != nil {
		return nil, err
	}
	ociConfig := common.NewRawConfigurationProvider(
		conf.Tenancy,
		conf.User,
		conf.Region,
		conf.FingerPrint,
		privateKey,
		&conf.PassPhrase,
	)
	return &Discovery{
		sdConfig:  conf,
		ociConfig: ociConfig,
		interval:  time.Duration(conf.RefreshInterval),
		logger:    logger,
	}, nil
}

// Run implements the Discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	// Get an initial set right away.
	tg, err := d.refresh()
	if err != nil {
		level.Error(d.logger).Log("msg", "Refreshing targets failed", "err", err)
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
				level.Error(d.logger).Log("msg", "Refreshing targets failed", "err", err)
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

func (d *Discovery) refresh() (tg *targetgroup.Group, err error) {
	t0 := time.Now()
	defer func() {
		ociSDRefreshDuration.Observe(time.Since(t0).Seconds())
		if err != nil {
			ociSDRefreshFailuresCount.Inc()
		}
	}()

	computeClient, err := core.NewComputeClientWithConfigurationProvider(d.ociConfig)
	if err != nil {
		return nil, err
	}
	vnicClient, err := core.NewVirtualNetworkClientWithConfigurationProvider(d.ociConfig)
	if err != nil {
		return nil, err
	}
	res, err := computeClient.ListInstances(
		context.Background(),
		core.ListInstancesRequest{CompartmentId: &d.sdConfig.Compartment},
	)
	if err != nil {
		return nil, fmt.Errorf("could not obtain list of instances: %s", err)
	}

	for _, instance := range res.Items {
		// TODO Guido: filter instances for defined, or free tags
		res, err := computeClient.ListVnicAttachments(
			context.Background(),
			core.ListVnicAttachmentsRequest{
				CompartmentId: &d.sdConfig.Compartment,
				InstanceId:    instance.Id,
			},
		)
		if err != nil {
			level.Error(d.logger).Log("msg", "could not obtain attached vnic", "ocid", instance.Id)
		}
		for _, vnic := range res.Items {
			res, err := vnicClient.GetVnic(
				context.Background(),
				core.GetVnicRequest{VnicId: vnic.VnicId},
			)
			if err != nil {
				level.Error(d.logger).Log("msg", "could not obtain vnic", "ocid", vnic.VnicId)
			}
			if *res.IsPrimary {
				labels := model.LabelSet{
					ociLabelInstanceID:         model.LabelValue(*instance.Id),
					ociLabelInstanceState:      model.LabelValue(instance.LifecycleState),
					ociLabelCompartment:        model.LabelValue(*instance.CompartmentId),
					ociLabelAvailabilityDomain: model.LabelValue(*instance.AvailabilityDomain),
					ociLabelPrivateIP:          model.LabelValue(*res.PrivateIp),
				}
				if *res.PublicIp != "" {
					labels[ociLabelPublicIP] = model.LabelValue(*res.PublicIp)
				}
				// TODO Guido: add tags as labels
				tg.Targets = append(tg.Targets, labels)
			}
		}
	}

	return tg, nil
}

func loadKey(keyFile string) (string, error) {
	data, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
