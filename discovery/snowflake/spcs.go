/*
 * Copyright (c) 2023 Snowflake Computing Inc. All rights reserved.
 */

package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/model"
	sf "github.com/snowflakedb/gosnowflake"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/refresh"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

const (
	SpcsPluginName      = "spcs"
	SpcsTokenFilePath   = "/snowflake/session/token"
	ComputePoolQuery    = "SHOW COMPUTE POOLS;"
	RoleQuery           = "SELECT CURRENT_ROLE();"
	NameColumn          = "name"
	StateColumn         = "state"
	OwnerColumn         = "owner"
	StateActiveValue    = "active"
	DNSPrefix           = "metrics"
	DNSSuffix           = "snowflakecomputing.internal"
	MetricsPort         = "9001"
	MetaLabelPrefix     = model.MetaLabelPrefix
	MetaSpcsPrefix      = SpcsPluginName + "_"
	MetaComputePoolName = MetaLabelPrefix + MetaSpcsPrefix + "compute_pool_name"
)

type AuthType string

const (
	AuthTypeToken    AuthType = "AuthTypeToken"
	AuthTypePassword AuthType = "AuthTypePassword"
)

// EndpointResolver interface definition.
type EndpointResolver interface {
	ResolveEndPoint(endPoint string) ([]string, error)
}

// DNSResolver is a real implementation of EndpointResolver.
type DNSResolver struct{}

// DefaultSPCSSDConfig is the default SPCS SD configuration.
var DefaultSPCSSDConfig = SPCSSDConfig{
	Port:            443,
	Protocol:        "https",
	RefreshInterval: model.Duration(60 * time.Second),
}

func init() {
	discovery.RegisterConfig(&SPCSSDConfig{})
}

// SPCSSDConfig is the configuration for applications running on spcs.
type SPCSSDConfig struct {
	Account         string         `yaml:"account"`
	Host            string         `yaml:"host,omitempty"`
	Port            int            `yaml:"port,omitempty"`
	Protocol        string         `yaml:"protocol,omitempty"`
	User            string         `yaml:"user,omitempty"`
	Password        string         `yaml:"password,omitempty"`
	Role            string         `yaml:"role,omitempty"`
	TokenPath       string         `yaml:"token_path,omitempty"`
	RefreshInterval model.Duration `yaml:"refresh_interval,omitempty"`
	Authenticator   AuthType       `yaml:"authenticator,omitempty"`
}

// Name returns the name of the Config.
func (*SPCSSDConfig) Name() string { return SpcsPluginName }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SPCSSDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return NewDiscovery(c, opts.Logger), nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SPCSSDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSPCSSDConfig
	type plain SPCSSDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if len(c.Authenticator) == 0 {
		return errors.New("Spcs SD configuration requires an Authenticator to be specified. Supported Authenticator types are AuthTypeToken, AuthTypePassword")
	} else {
		switch c.Authenticator {
		case AuthTypeToken:
			// TokenPath is optional for AuthTypeToken, no need to validate.
		case AuthTypePassword:
			if len(c.User) == 0 || len(c.Password) == 0 || len(c.Role) == 0 {
				return errors.New("Spcs SD configuration requires { User, Password, Role } when Authenticator type is AuthTypePassword")
			}
		default:
			return errors.New("Spcs SD configuration unsupported Authenticator " + string(c.Authenticator) + ". Supported Authenticator types are AuthTypeToken, AuthTypePassword")
		}
	}

	if len(c.Account) == 0 {
		// Get Account from env variable.
		c.Account = os.Getenv("SNOWFLAKE_ACCOUNT")
	}
	if len(c.Host) == 0 {
		// Get host from env variable.
		c.Host = os.Getenv("SNOWFLAKE_HOST")
	}
	return nil
}

// Discovery provides service discovery based on a spcs instance.
type Discovery struct {
	*refresh.Discovery
	logger log.Logger
	cfg    *SPCSSDConfig
}

// New creates a new spcs discovery for the given role.
func NewDiscovery(conf *SPCSSDConfig, logger log.Logger) *Discovery {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	d := &Discovery{
		logger: logger,
		cfg:    conf,
	}
	d.Discovery = refresh.NewDiscovery(
		logger,
		SpcsPluginName,
		time.Duration(d.cfg.RefreshInterval),
		d.refresh,
	)
	return d
}

func (d *Discovery) refresh(ctx context.Context) ([]*targetgroup.Group, error) {
	cfg, err := getSfConfig(d.cfg)
	if err != nil {
		return nil, fmt.Errorf("SPCS discovery plugin: Error getting snowflake config to scrape. err: %w", err)
	}

	rows, columns, err := getDataFromSnowflake(cfg, ComputePoolQuery)
	if err != nil {
		return nil, fmt.Errorf("SPCS discovery plugin: Error getting data from snowflake. err: %w", err)
	}

	rolesRows, _, err := getDataFromSnowflake(cfg, RoleQuery)
	if err != nil {
		return nil, fmt.Errorf("SPCS discovery plugin: Error getting data from snowflake. err: %w", err)
	}

	role, err := getCurrentRole(rolesRows)
	if err != nil {
		return nil, fmt.Errorf("SPCS discovery plugin: Error getting current role. err: %w", err)
	}

	computePools, err := getComputePoolsToScrape(rows, columns, role)
	if err != nil {
		return nil, fmt.Errorf("SPCS discovery plugin: Error getting compute pools to scrape. err: %w", err)
	}

	targetgroups, err := getTargetgroups(computePools, d.logger)
	if err != nil {
		return nil, fmt.Errorf("SPCS discovery plugin: Error getting targetgroups to scrape. err: %w", err)
	}
	return targetgroups, nil
}

func getSfConfig(config *SPCSSDConfig) (sf.Config, error) {
	var cfg sf.Config
	var err error
	var token string

	switch config.Authenticator {
	case AuthTypePassword:
		cfg = sf.Config{
			Account:  config.Account,
			Host:     config.Host,
			Port:     config.Port,
			Protocol: config.Protocol,
			User:     config.User,
			Password: config.Password,
			Role:     config.Role,
		}
	case AuthTypeToken:
		if len(config.TokenPath) > 0 {
			token, err = getTokenFromFile(config.TokenPath)
			cfg = sf.Config{
				Account:       config.Account,
				Host:          config.Host,
				Token:         token,
				Authenticator: sf.AuthTypeOAuth,
			}
		} else {
			token, err = getTokenFromFile(SpcsTokenFilePath)
			cfg = sf.Config{
				Account:       config.Account,
				Host:          config.Host,
				Token:         token,
				Authenticator: sf.AuthTypeOAuth,
			}
		}
	}

	cfg.InsecureMode = true
	if err != nil {
		return cfg, fmt.Errorf("SPCS discovery plugin: Error getting credentials from token file. err: %w", err)
	}
	return cfg, nil
}

func getTokenFromFile(filePath string) (string, error) {
	// Open the file.
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read the file contents into a string.
	var fileContentsBuilder strings.Builder
	_, err = io.Copy(&fileContentsBuilder, file)
	if err != nil {
		return "", err
	}

	return fileContentsBuilder.String(), nil
}

// getDataFromSnowflake establishes a connection with a Snowflake database, executes a query,
// and retrieves the result in the form of rows and column names.
//
// Parameters:
//   - cfg: An sf.Config struct containing Snowflake database configuration.
//   - query: Query to run in Snowflake.
//
// Returns:
//   - rows: A 2D string slice representing the result rows of the executed query.
//   - columns: A string slice containing the names of the columns in the result set.
//   - error: An error, if any, encountered during the database connection, query execution, or data retrieval.
func getDataFromSnowflake(cfg sf.Config, query string) ([][]string, []string, error) {
	// Create DSN from config
	dsn, err := sf.DSN(&cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("SPCS discovery plugin: failed to create DSN from Config. err: %w", err)
	}

	// Establish connection with snowflake.
	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("SPCS discovery plugin: failed to connect. err: %w", err)
	}
	defer db.Close()

	// Execute query.
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, fmt.Errorf("SPCS discovery plugin: failed to run query. %s, err: %w", query, err)
	}
	defer rows.Close()

	// Get column names and types.
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("SPCS discovery plugin: error getting column names. err: %w", err)
	}

	// Create a slice to hold the values.
	values := make([]interface{}, len(columns))
	for i := range values {
		var v interface{}
		values[i] = &v
	}

	// Create a slice to store the rows.
	var result [][]string

	// Iterate through the rows.
	for rows.Next() {
		// Scan the current row into the values slice.
		if err := rows.Scan(values...); err != nil {
			return nil, nil, fmt.Errorf("SPCS discovery plugin: error scanning row. err: %w", err)
		}

		// Convert each value to a string and append it to the result slice.
		rowValues := make([]string, len(columns))
		for i := range columns {
			if values[i] != nil {
				// Convert the underlying value to a string.
				strVal := fmt.Sprintf("%v", *values[i].(*interface{}))
				rowValues[i] = strVal
			}
		}
		result = append(result, rowValues)
	}

	// Check for errors during iteration.
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("SPCS discovery plugin: Error iterating over rows. err: %w", err)
	}

	return result, columns, nil
}

func getCurrentRole(rows [][]string) (string, error) {
	// Expected to return a single row with one column.
	for _, row := range rows {
		role := row[0]
		return role, nil
	}
	return "", fmt.Errorf("SPCS discovery plugin: error retrieving role.")
}

func getComputePoolsToScrape(rows [][]string, columns []string, role string) ([]string, error) {
	// Create a slice to hold the computepools.
	computePools := []string{}

	// Iterate through the rows.
	for _, row := range rows {
		cpState, err := getColumnValue(row, columns, StateColumn)
		if err != nil {
			return nil, fmt.Errorf("SPCS discovery plugin: error retrieving compute pool state. %w", err)
		}

		ownerName, err := getColumnValue(row, columns, OwnerColumn)
		if err != nil {
			return nil, fmt.Errorf("SPCS discovery plugin: error retrieving compute pool owner name. %w", err)
		}

		cpName, err := getColumnValue(row, columns, NameColumn)
		if err != nil {
			return nil, fmt.Errorf("SPCS discovery plugin: error retrieving compute pool name. %w", err)
		}

		if strings.EqualFold(strings.ToLower(cpState), StateActiveValue) && strings.EqualFold(strings.ToLower(ownerName), strings.ToLower(role)) {
			computePools = append(computePools, cpName)
		}
	}
	return computePools, nil
}

func getColumnValue(row, columns []string, columnName string) (string, error) {
	// Get the column index.
	columnIndex := indexOf(columnName, columns)
	if columnIndex == -1 {
		return "", fmt.Errorf("SPCS discovery plugin: column not found. column_name: %s", columnName)
	}

	return row[columnIndex], nil
}

func indexOf(target string, slice []string) int {
	for i, value := range slice {
		if value == target {
			return i
		}
	}
	return -1 // Element not found.
}

func getTargetgroups(computePools []string, logger log.Logger) ([]*targetgroup.Group, error) {
	targets := []model.LabelSet{}
	for _, computePool := range computePools {
		var resolver EndpointResolver = &DNSResolver{}
		targetsForCP, err := getTargets(computePool, resolver)
		if err != nil {
			level.Error(logger).Log("msg", "SPCS discovery plugin: Warning, error resolving endpoint for compute pool.", "computePool", computePool, "error", err)
		} else {
			targets = append(targets, targetsForCP...)
		}
	}

	tg := &targetgroup.Group{
		Source:  SpcsPluginName,
		Targets: targets,
	}

	level.Info(logger).Log("msg", "SPCS discovery plugin: Targets discovered are:", "targets", targets)
	return []*targetgroup.Group{tg}, nil
}

func getTargets(computePool string, resolver EndpointResolver) ([]model.LabelSet, error) {
	targetsForCP := []model.LabelSet{}
	endPoint := getEndPointFromComputePool(computePool)
	ipAddresses, err := resolver.ResolveEndPoint(endPoint)
	if err != nil {
		return nil, fmt.Errorf("SPCS discovery plugin: error resolving endpoint. %w", err)
	}

	for _, ipaddress := range ipAddresses {
		targetForCP := model.LabelSet{
			model.LabelName(model.AddressLabel): model.LabelValue(ipaddress + ":" + MetricsPort),
			MetaComputePoolName:                 model.LabelValue(strings.ToLower(computePool)),
		}
		targetsForCP = append(targetsForCP, targetForCP)
	}

	return targetsForCP, nil
}

func getEndPointFromComputePool(computePool string) string {
	return DNSPrefix + "." + strings.ToLower(computePool) + "." + DNSSuffix
}

func (d *DNSResolver) ResolveEndPoint(endPoint string) ([]string, error) {
	// Resolve the DNS entry to IP addresses.
	ips, err := net.LookupIP(endPoint)
	if err != nil {
		return nil, err
	}

	ipAddresses := []string{}
	for _, ip := range ips {
		ipAddresses = append(ipAddresses, ip.String())
	}

	return ipAddresses, nil
}
