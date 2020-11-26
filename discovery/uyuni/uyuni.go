// Copyright 2020 The Prometheus Authors
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
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolo/xmlrpc"
	"github.com/pkg/errors"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	monitoringEntitlementLabel = "monitoring_entitled"
	uyuniXMLRPCAPIPath         = "/rpc/api"
	uyuniMetaLabelPrefix       = model.MetaLabelPrefix + "uyuni_"
)

// DefaultSDConfig is the default Uyuni SD configuration.
var DefaultSDConfig = SDConfig{
	ExporterFormulas:  []string{"prometheus-exporters"},
	FormulasSeparator: ",",
	RefreshInterval:   model.Duration(1 * time.Minute),
}

// Regular expression to extract port from formula data
var monFormulaRegex = regexp.MustCompile(`--(?:telemetry\.address|web\.listen-address)=\":([0-9]*)\"`)

func init() {
	discovery.RegisterConfig(&SDConfig{})
}

// SDConfig is the configuration for Uyuni based service discovery.
type SDConfig struct {
	Host              string         `yaml:"host"`
	User              string         `yaml:"username"`
	Pass              config.Secret  `yaml:"password"`
	ExporterFormulas  []string       `yaml:"exporter_formulas,omitempty"`
	FormulasSeparator string         `yaml:"formulas_separator,omitempty"`
	RefreshInterval   model.Duration `yaml:"refresh_interval,omitempty"`
}

// Uyuni API Response structures
type systemGroupID struct {
	GroupID   int    `xmlrpc:"id"`
	GroupName string `xmlrpc:"name"`
}

type networkInfo struct {
	SystemID int    `xmlrpc:"system_id"`
	Hostname string `xmlrpc:"hostname"`
	IP       string `xmlrpc:"ip"`
}

type exporterConfig struct {
	Address     string `xmlrpc:"address"`
	Args        string `xmlrpc:"args"`
	Enabled     bool   `xmlrpc:"enabled"`
	ProxyModule string `xmlrpc:"proxy_module"`
}

type proxiedExporterConfig struct {
	ProxyIsEnabled  bool                      `xmlrpc:"proxy_enabled"`
	ProxyPort       float32                   `xmlrpc:"proxy_port"`
	ExporterConfigs map[string]exporterConfig `xmlrpc:"exporters"`
}

// Discovery periodically performs Uyuni API requests. It implements the Discoverer interface.
type Discovery struct {
	*refresh.Discovery
	interval time.Duration
	sdConfig *SDConfig
	logger   log.Logger
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "uyuni" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger), nil
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

// Get the system groups information of monitored clients
func getSystemGroupsInfoOfMonitoredClients(rpcclient *xmlrpc.Client, token string) (map[int][]systemGroupID, error) {
	var systemGroupsInfos []struct {
		SystemID     int             `xmlrpc:"id"`
		SystemGroups []systemGroupID `xmlrpc:"system_groups"`
	}
	err := rpcclient.Call("system.listSystemGroupsForSystemsWithEntitlement", []interface{}{token, monitoringEntitlementLabel}, &systemGroupsInfos)
	if err != nil {
		return nil, err
	}
	result := make(map[int][]systemGroupID)
	for _, systemGroupsInfo := range systemGroupsInfos {
		result[systemGroupsInfo.SystemID] = systemGroupsInfo.SystemGroups
	}
	return result, nil
}

// GetSystemNetworkInfo lists client FQDNs
func getNetworkInformationForSystems(rpcclient *xmlrpc.Client, token string, systemIDs []int) (map[int]networkInfo, error) {
	var networkInfos []networkInfo
	err := rpcclient.Call("system.getNetworkForSystems", []interface{}{token, systemIDs}, &networkInfos)
	if err != nil {
		return nil, err
	}
	result := make(map[int]networkInfo)
	for _, networkInfo := range networkInfos {
		result[networkInfo.SystemID] = networkInfo
	}
	return result, nil
}

// Get formula data for a given system
func getExporterDataForSystems(
	rpcclient *xmlrpc.Client,
	token string,
	formulaName string,
	systemIDs []int,
) (map[int]proxiedExporterConfig, error) {
	var combinedFormulaData []struct {
		SystemID        int                   `xmlrpc:"system_id"`
		ExporterConfigs proxiedExporterConfig `xmlrpc:"formula_values"`
	}
	err := rpcclient.Call(
		"formula.getCombinedFormulaDataByServerIds",
		[]interface{}{token, formulaName, systemIDs}, &combinedFormulaData)
	if err != nil {
		return nil, err
	}
	result := make(map[int]proxiedExporterConfig)
	for _, combinedFormulaData := range combinedFormulaData {
		result[combinedFormulaData.SystemID] = combinedFormulaData.ExporterConfigs
	}
	return result, nil
}

// Get list of formulas a server and all his groups have
func getFormulas(
	rpcclient *xmlrpc.Client,
	token string,
	systemID int,
) ([]string, error) {
	var formulas []string
	err := rpcclient.Call(
		"formula.getCombinedFormulasByServerId",
		[]interface{}{token, systemID}, &formulas)
	if err != nil {
		return nil, err
	}
	return formulas, nil
}

// extractPortFromFormulaData gets exporter port configuration from the formula.
// args takes precedence over address.
func extractPortFromFormulaData(args string, address string) (string, error) {
	// first try args
	var port string
	tokens := monFormulaRegex.FindStringSubmatch(args)
	if len(tokens) < 1 {
		err := "Unable to find port in args: " + args
		// now try address
		_, addrPort, addrErr := net.SplitHostPort(address)
		if addrErr != nil || len(addrPort) == 0 {
			if addrErr != nil {
				err = strings.Join([]string{addrErr.Error(), err}, " ")
			}
			return "", errors.New(err)
		}
		port = addrPort
	} else {
		port = tokens[1]
	}

	return port, nil
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

func initializeExporterTargets(
	targets *[]model.LabelSet,
	exporterName string, config exporterConfig,
	proxyPort string,
	errors *[]error,
) {
	if !(config.Enabled) {
		return
	}
	var port string
	if len(proxyPort) == 0 {
		exporterPort, err := extractPortFromFormulaData(config.Args, config.Address)
		if err != nil {
			*errors = append(*errors, err)
			return
		}
		port = exporterPort
	} else {
		port = proxyPort
	}

	labels := model.LabelSet{}
	labels[uyuniMetaLabelPrefix+"exporter"] = model.LabelValue(exporterName)
	// for now set only port number here
	labels[model.AddressLabel] = model.LabelValue(port)
	if len(proxyPort) > 0 && len(config.ProxyModule) > 0 {
		labels[uyuniMetaLabelPrefix+"proxy_module"] = model.LabelValue(config.ProxyModule)
	} else if len(proxyPort) > 0 {
		labels[uyuniMetaLabelPrefix+"proxy_module"] = model.LabelValue(exporterName)
	}
	*targets = append(*targets, labels)
}

func (d *Discovery) getTargetsForSystem(
	systemID int,
	systemGroupsIDs []systemGroupID,
	networkInfo networkInfo,
	formulas []string,
	combinedFormulaData proxiedExporterConfig,
) []model.LabelSet {

	var labelSets []model.LabelSet
	var errors []error
	var proxyPortNumber string
	if combinedFormulaData.ProxyIsEnabled {
		proxyPortNumber = fmt.Sprintf("%d", int(combinedFormulaData.ProxyPort))
	}
	for exporterName, formulaValues := range combinedFormulaData.ExporterConfigs {
		initializeExporterTargets(&labelSets, exporterName, formulaValues, proxyPortNumber, &errors)
	}
	// Initialize targets without exporters
	if len(labelSets) == 0 {
		labels := model.LabelSet{}
		labelSets = append(labelSets, labels)
	}
	managedGroupNames := getSystemGroupNames(systemGroupsIDs)
	for _, labels := range labelSets {
		// add hostname to the address label
		addr := fmt.Sprintf("%s:%s", networkInfo.IP, labels[model.AddressLabel])
		labels[model.AddressLabel] = model.LabelValue(addr)
		labels["hostname"] = model.LabelValue(networkInfo.Hostname)
		labels[uyuniMetaLabelPrefix+"system_id"] = model.LabelValue(fmt.Sprintf("%d", systemID))
		labels[uyuniMetaLabelPrefix+"groups"] = model.LabelValue(strings.Join(managedGroupNames, ","))
		var formulasLabelValue = d.sdConfig.FormulasSeparator +
			strings.Join(formulas, d.sdConfig.FormulasSeparator) +
			d.sdConfig.FormulasSeparator
		labels[uyuniMetaLabelPrefix+"formulas"] = model.LabelValue(formulasLabelValue)
		if combinedFormulaData.ProxyIsEnabled {
			labels[uyuniMetaLabelPrefix+"metrics_path"] = "/proxy"
		}
		_ = level.Debug(d.logger).Log("msg", "Configured target", "Labels", fmt.Sprintf("%+v", labels))
	}
	for _, err := range errors {
		level.Error(d.logger).Log("msg", "Invalid exporter port", "clientId", systemID, "err", err)
	}

	return labelSets
}

func getSystemGroupNames(systemGroupsIDs []systemGroupID) []string {
	managedGroupNames := make([]string, 0, len(systemGroupsIDs))
	for _, systemGroupInfo := range systemGroupsIDs {
		managedGroupNames = append(managedGroupNames, systemGroupInfo.GroupName)
	}

	if len(managedGroupNames) == 0 {
		managedGroupNames = []string{"No group"}
	}
	return managedGroupNames
}

func (d *Discovery) getTargetsForSystems(
	rpcClient *xmlrpc.Client,
	token string,
	systemGroupIDsBySystemID map[int][]systemGroupID,
) ([]model.LabelSet, error) {

	result := make([]model.LabelSet, 0)

	systemIDs := make([]int, 0, len(systemGroupIDsBySystemID))
	formulas := make(map[int][]string)
	for systemID := range systemGroupIDsBySystemID {
		systemIDs = append(systemIDs, systemID)
		systemFormulas, err := getFormulas(rpcClient, token, systemID)
		if err != nil {
			return nil, errors.Wrap(err, "Unable to get list of system formulas")
		}
		formulas[systemID] = systemFormulas
	}

	for _, exportersFormulaName := range d.sdConfig.ExporterFormulas {
		combinedFormulaDataBySystemID, err := getExporterDataForSystems(rpcClient, token, exportersFormulaName, systemIDs)
		if err != nil {
			return nil, errors.Wrap(err, "Unable to get systems combined formula data")
		}
		networkInfoBySystemID, err := getNetworkInformationForSystems(rpcClient, token, systemIDs)
		if err != nil {
			return nil, errors.Wrap(err, "Unable to get the systems network information")
		}

		for _, systemID := range systemIDs {
			targets := d.getTargetsForSystem(
				systemID,
				systemGroupIDsBySystemID[systemID],
				networkInfoBySystemID[systemID],
				formulas[systemID],
				combinedFormulaDataBySystemID[systemID])
			result = append(result, targets...)

			// Log debug information
			if networkInfoBySystemID[systemID].IP != "" {
				level.Debug(d.logger).Log("msg", "Found monitored system",
					"Host", networkInfoBySystemID[systemID].Hostname,
					"Network", fmt.Sprintf("%+v", networkInfoBySystemID[systemID]),
					"Groups", fmt.Sprintf("%+v", systemGroupIDsBySystemID[systemID]),
					"Formulas", fmt.Sprintf("%+v", formulas[systemID]),
					"FormulaValues", fmt.Sprintf("%+v", combinedFormulaDataBySystemID[systemID]))
			}
		}
	}
	return result, nil
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	config := d.sdConfig
	apiURL := config.Host + uyuniXMLRPCAPIPath

	startTime := time.Now()

	// Check if the URL is valid and create rpc client
	_, err := url.ParseRequestURI(apiURL)
	if err != nil {
		return nil, errors.Wrap(err, "Uyuni Server URL is not valid")
	}

	rpcClient, err := xmlrpc.NewClient(apiURL, nil)
	if err != nil {
		return nil, err
	}
	defer rpcClient.Close()

	token, err := login(rpcClient, config.User, string(config.Pass))
	if err != nil {
		return nil, errors.Wrap(err, "Unable to login to Uyuni API")
	}
	defer func() {
		if err := logout(rpcClient, token); err != nil {
			level.Warn(d.logger).Log("msg", "Failed to log out from Uyuni API", "err", err)
		}
	}()

	systemGroupIDsBySystemID, err := getSystemGroupsInfoOfMonitoredClients(rpcClient, token)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to get the managed system groups information of monitored clients")
	}

	targets := make([]model.LabelSet, 0)
	if len(systemGroupIDsBySystemID) > 0 {
		targetsForSystems, err := d.getTargetsForSystems(rpcClient, token, systemGroupIDsBySystemID)
		if err != nil {
			return nil, err
		}
		targets = append(targets, targetsForSystems...)
		level.Info(d.logger).Log("msg", "Total discovery time", "time", time.Since(startTime))
	} else {
		level.Debug(d.logger).Log("msg", "Found 0 systems")
	}

	return []*targetgroup.Group{&targetgroup.Group{Targets: targets, Source: config.Host}}, nil
}
