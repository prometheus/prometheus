// Copyright 2019 The Prometheus Authors
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

package uyuni

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolo/xmlrpc"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	uyuniLabel             = model.MetaLabelPrefix + "uyuni_"
	uyuniLabelEntitlements = uyuniLabel + "entitlements"
)

// DefaultSDConfig is the default Uyuni SD configuration.
var DefaultSDConfig = SDConfig{
	RefreshInterval: model.Duration(1 * time.Minute),
}

// Regular expression to extract port from formula data
var monFormulaRegex = regexp.MustCompile(`--(?:telemetry\.address|web\.listen-address)=\":([0-9]*)\"`)

// SDConfig is the configuration for Uyuni based service discovery.
type SDConfig struct {
	Host            string         `yaml:"host"`
	User            string         `yaml:"username"`
	Pass            string         `yaml:"password"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
}

// Uyuni API Response structures
type clientRef struct {
	ID   int    `xmlrpc:"id"`
	Name string `xmlrpc:"name"`
}

type systemDetail struct {
	ID           int      `xmlrpc:"id"`
	Hostname     string   `xmlrpc:"hostname"`
	Entitlements []string `xmlrpc:"addon_entitlements"`
}

type groupDetail struct {
	ID              int    `xmlrpc:"id"`
	Subscribed      int    `xmlrpc:"subscribed"`
	SystemGroupName string `xmlrpc:"system_group_name"`
}

type networkInfo struct {
	IP string `xmlrpc:"ip"`
}

type exporterConfig struct {
	Args    string `xmlrpc:"args"`
	Enabled bool   `xmlrpc:"enabled"`
}

// Discovery periodically performs Uyuni API requests. It implements the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	client   *http.Client
	interval time.Duration
	sdConfig *SDConfig
	logger   log.Logger
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))

	if err != nil {
		return err
	}
	if c.Host == "" {
		return errors.New("Uyuni SD configuration requires a Host")
	}
	if c.User == "" {
		return errors.New("Uyuni SD configuration requires a Username")
	}
	if c.Pass == "" {
		return errors.New("Uyuni SD configuration requires a Password")
	}
	if c.RefreshInterval <= 0 {
		return errors.New("Uyuni SD configuration requires RefreshInterval to be a positive integer")
	}
	return nil
}

// Attempt to login in Uyuni Server and get an auth token
func login(rpcclient *xmlrpc.Client, user string, pass string) (string, error) {
	var result string
	err := rpcclient.Call("auth.login", []interface{}{user, pass}, &result)
	return result, err
}

// Logout from Uyuni API
func logout(rpcclient *xmlrpc.Client, token string) error {
	err := rpcclient.Call("auth.logout", token, nil)
	return err
}

// Get system list
func listSystems(rpcclient *xmlrpc.Client, token string) ([]clientRef, error) {
	var result []clientRef
	err := rpcclient.Call("system.listSystems", token, &result)
	return result, err
}

// Get system details
func getSystemDetails(rpcclient *xmlrpc.Client, token string, systemID int) (systemDetail, error) {
	var result systemDetail
	err := rpcclient.Call("system.getDetails", []interface{}{token, systemID}, &result)
	return result, err
}

// Get list of groups a system belongs to
func listSystemGroups(rpcclient *xmlrpc.Client, token string, systemID int) ([]groupDetail, error) {
	var result []groupDetail
	err := rpcclient.Call("system.listGroups", []interface{}{token, systemID}, &result)
	return result, err
}

// GetSystemNetworkInfo lists client FQDNs
func getSystemNetworkInfo(rpcclient *xmlrpc.Client, token string, systemID int) (networkInfo, error) {
	var result networkInfo
	err := rpcclient.Call("system.getNetwork", []interface{}{token, systemID}, &result)
	return result, err
}

// Get formula data for a given system
func getSystemFormulaData(rpcclient *xmlrpc.Client, token string, systemID int, formulaName string) (map[string]exporterConfig, error) {
	var result map[string]exporterConfig
	err := rpcclient.Call("formula.getSystemFormulaData", []interface{}{token, systemID, formulaName}, &result)
	return result, err
}

// Get formula data for a given group
func getGroupFormulaData(rpcclient *xmlrpc.Client, token string, groupID int, formulaName string) (map[string]exporterConfig, error) {
	var result map[string]exporterConfig
	err := rpcclient.Call("formula.getGroupFormulaData", []interface{}{token, groupID, formulaName}, &result)
	return result, err
}

// Get exporter port configuration from Formula
func extractPortFromFormulaData(args string) (string, error) {
	tokens := monFormulaRegex.FindStringSubmatch(args)
	if len(tokens) < 1 {
		return "", errors.New("Unable to find port in args: " + args)
	}
	return tokens[1], nil
}

// Take a current formula structure and override values if the new config is set
// Used for calculating final formula values when using groups
func getCombinedFormula(combined map[string]exporterConfig, new map[string]exporterConfig) map[string]exporterConfig {
	for k, v := range new {
		if v.Enabled {
			combined[k] = v
		}
	}
	return combined
}

// NewDiscovery returns a new file discovery for the given paths.
func NewDiscovery(conf *SDConfig, logger log.Logger) *Discovery {
	d := &Discovery{
		interval: time.Duration(conf.RefreshInterval),
		sdConfig: conf,
		logger:   logger,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		"uyuni",
		time.Duration(conf.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {

	config := d.sdConfig
	apiURL := config.Host + "/rpc/api"

	// Check if the URL is valid and create rpc client
	_, err := url.ParseRequestURI(apiURL)
	if err != nil {
		return nil, errors.Wrap(err, "Uyuni Server URL is not valid")
	}
	rpc, _ := xmlrpc.NewClient(apiURL, nil)
	tg := &targetgroup.Group{Source: config.Host}

	// Login into Uyuni API and get auth token
	token, err := login(rpc, config.User, config.Pass)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to login to Uyuni API")
	}
	// Get list of managed clients from Uyuni API
	clientList, err := listSystems(rpc, token)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to get list of systems")
	}

	// Iterate list of clients
	if len(clientList) == 0 {
		fmt.Printf("\tFound 0 systems.\n")
	} else {
		startTime := time.Now()
		var wg sync.WaitGroup
		wg.Add(len(clientList))

		for _, cl := range clientList {

			go func(client clientRef) {
				defer wg.Done()
				rpcclient, _ := xmlrpc.NewClient(apiURL, nil)
				netInfo := networkInfo{}
				formulas := map[string]exporterConfig{}
				groups := []groupDetail{}

				// Get the system details
				details, err := getSystemDetails(rpcclient, token, client.ID)

				if err != nil {
					level.Error(d.logger).Log("msg", "Unable to get system details", "clientId", client.ID, "err", err)
					return
				}
				jsonDetails, _ := json.Marshal(details)
				level.Debug(d.logger).Log("msg", "System details", "details", jsonDetails)

				// Check if system is monitoring entitled
				for _, v := range details.Entitlements {
					if v == "monitoring_entitled" { // golang has no native method to check if an element is part of a slice

						// Get network details
						netInfo, err = getSystemNetworkInfo(rpcclient, token, client.ID)
						if err != nil {
							level.Error(d.logger).Log("msg", "getSystemNetworkInfo failed", "clientId", client.ID, "err", err)
							return
						}

						// Get list of groups this system is assigned to
						candidateGroups, err := listSystemGroups(rpcclient, token, client.ID)
						if err != nil {
							level.Error(d.logger).Log("msg", "listSystemGroups failed", "clientId", client.ID, "err", err)
							return
						}
						groups := []string{}
						for _, g := range candidateGroups {
							// get list of group formulas
							// TODO: Put the resulting data on a map so that we do not have to repeat the call below for every system
							if g.Subscribed == 1 {
								groupFormulas, err := getGroupFormulaData(rpcclient, token, g.ID, "prometheus-exporters")
								if err != nil {
									level.Error(d.logger).Log("msg", "getGroupFormulaData failed", "groupId", client.ID, "err", err)
									return
								}
								formulas = getCombinedFormula(formulas, groupFormulas)
								// replace spaces with dashes on all group names
								groups = append(groups, strings.ToLower(strings.ReplaceAll(g.SystemGroupName, " ", "-")))
							}
						}

						// Get system formula list
						systemFormulas, err := getSystemFormulaData(rpcclient, token, client.ID, "prometheus-exporters")
						if err != nil {
							level.Error(d.logger).Log("msg", "getSystemFormulaData failed", "clientId", client.ID, "err", err)
							return
						}
						formulas = getCombinedFormula(formulas, systemFormulas)

						// Iterate list of formulas and check for enabled exporters
						for k, v := range formulas {
							if v.Enabled {
								port, err := extractPortFromFormulaData(v.Args)
								if err != nil {
									level.Error(d.logger).Log("msg", "Invalid exporter port", "clientId", client.ID, "err", err)
									return
								}
								targets := model.LabelSet{}
								addr := fmt.Sprintf("%s:%s", netInfo.IP, port)
								targets[model.AddressLabel] = model.LabelValue(addr)
								targets["exporter"] = model.LabelValue(k)
								targets["hostname"] = model.LabelValue(details.Hostname)
								targets["groups"] = model.LabelValue(strings.Join(groups, ","))
								for _, g := range groups {
									gname := fmt.Sprintf("grp_%s", g)
									targets[model.LabelName(gname)] = model.LabelValue("active")
								}
								tg.Targets = append(tg.Targets, targets)
							}
						}
					}
				}
				// Log debug information
				if netInfo.IP != "" {
					level.Info(d.logger).Log("msg", "Found monitored system", "Host", details.Hostname,
						"Entitlements", fmt.Sprintf("%+v", details.Entitlements),
						"Network", fmt.Sprintf("%+v", netInfo), "Groups",
						fmt.Sprintf("%+v", groups), "Formulas", fmt.Sprintf("%+v", formulas))
				}
				rpcclient.Close()
			}(cl)
		}
		wg.Wait()
		level.Info(d.logger).Log("msg", "Total discovery time", "time", time.Since(startTime))
	}
	logout(rpc, token)
	rpc.Close()
	return []*targetgroup.Group{tg}, nil
}
