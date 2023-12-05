/*
 * Copyright (c) 2023 Snowflake Computing Inc. All rights reserved.
 */

package snowflake

import (
	"os"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"gopkg.in/yaml.v2"
)

// Constants for test cases.
const (
	TestAccountIdentifer = "testOrganization-testAccount"
	TestAccount          = "testAccount"
	TestHost             = "testHost"
	TestUser             = "testUser"
	TestPassword         = "testPassword"
	TestRole             = "testRole"
	TestTokenPath        = "/path/to/token"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNewDiscovery(t *testing.T) {
	config := SPCSSDConfig{
		Account:         TestAccount,
		Host:            TestHost,
		Authenticator:   AuthTypePassword,
		User:            TestUser,
		Password:        TestPassword,
		Role:            TestRole,
		Port:            DefaultSPCSSDConfig.Port,
		Protocol:        DefaultSPCSSDConfig.Protocol,
		RefreshInterval: DefaultSPCSSDConfig.RefreshInterval,
	}
	d := NewDiscovery(&config, log.NewNopLogger())
	require.NotNil(t, d)
	require.Equal(t, config, *d.cfg)
}

func TestGetComputePoolsToScrape(t *testing.T) {
	// Test case with a single row, where cpState is active.
	rows := [][]string{
		{"active", "pool1", "admin"},
	}
	columns := []string{"state", "name", "owner"}
	role := "admin"
	expected := []string{"pool1"}
	result, err := getComputePoolsToScrape(rows, columns, role)
	verifyResult(t, err, result, expected)

	// Test case with multiple rows, some active and some inactive.
	rows = [][]string{
		{"active", "pool1", "admin"},
		{"inactive", "pool2", "admin"},
		{"active", "pool3", "admin"},
	}
	expected = []string{"pool1", "pool3"}
	result, err = getComputePoolsToScrape(rows, columns, role)
	verifyResult(t, err, result, expected)

	// Test case with no active compute pools.
	rows = [][]string{
		{"inactive", "pool1", "admin"},
		{"inactive", "pool2", "admin"},
	}
	expected = []string{}
	result, err = getComputePoolsToScrape(rows, columns, role)
	verifyResult(t, err, result, expected)

	// Test case with active compute pools but different owner.
	// Should list only the compute pools that are created from same role as service.
	rows = [][]string{
		{"active", "pool1", "admin"},
		{"active", "pool2", "guest"},
	}
	expected = []string{"pool1"}
	result, err = getComputePoolsToScrape(rows, columns, role)
	verifyResult(t, err, result, expected)

	// Test case with no state column.
	rows = [][]string{
		{"pool1", "admin"},
		{"pool2", "admin"},
	}
	columns = []string{"name", "owner"}
	_, err = getComputePoolsToScrape(rows, columns, role)
	if err == nil {
		t.Errorf("Expected an error, but got no error")
	}
	require.Equal(t, "SPCS discovery plugin: error retrieving compute pool state. SPCS discovery plugin: column not found. column_name: state", err.Error(), "Error message mismatch")
}

func TestGetColumnValue(t *testing.T) {
	row := []string{"active", "pool1", "admin"}
	columns := []string{"state", "name", "owner"}

	// Test case with an existing column name.
	columnName := "state"
	expected := "active"
	result, err := getColumnValue(row, columns, columnName)
	if err != nil {
		t.Errorf("Expected no error, but got an error: %v", err)
	}
	if result != expected {
		t.Errorf("Expected %v, but got %v", expected, result)
	}

	// Test case with a non-existing column name.
	columnName = "nonexistent"
	_, err = getColumnValue(row, columns, columnName)
	if err == nil {
		t.Errorf("Expected an error, but got no error")
	}
	require.Equal(t, "SPCS discovery plugin: column not found. column_name: nonexistent", err.Error(), "Error message mismatch")
}

func TestGetTargets(t *testing.T) {
	// Create a mock resolver for testing.
	mockResolver := &MockEndpointResolver{}

	// Resolving a compute pool with one node.
	computePool := "Pool1"
	mockResolver.ipAddresses = []string{"1.2.3.4"}
	resultTargets, err := getTargets(computePool, mockResolver)
	expectedTargets := []model.LabelSet{
		{
			model.LabelName(model.AddressLabel): model.LabelValue("1.2.3.4:" + MetricsPort),
			MetaComputePoolName:                 model.LabelValue("pool1"),
		},
	}
	verifyTargets(t, err, resultTargets, expectedTargets)

	// Resolving a compute pool with two nodes.
	computePool = "Pool1"
	mockResolver.ipAddresses = []string{"1.2.3.4", "5.6.7.8"}
	resultTargets, err = getTargets(computePool, mockResolver)
	expectedTargets = []model.LabelSet{
		{
			model.LabelName(model.AddressLabel): model.LabelValue("1.2.3.4:" + MetricsPort),
			MetaComputePoolName:                 model.LabelValue("pool1"),
		},
		{
			model.LabelName(model.AddressLabel): model.LabelValue("5.6.7.8:" + MetricsPort),
			MetaComputePoolName:                 model.LabelValue("pool1"),
		},
	}
	verifyTargets(t, err, resultTargets, expectedTargets)

	// Empty compute pool.
	computePool = ""
	_, err = getTargets(computePool, &DNSResolver{})
	if err == nil {
		t.Errorf("Expected an error, but got no error")
	}
	require.Contains(t, err.Error(), "SPCS discovery plugin: error resolving endpoint.", "Error message mismatch")

	// DNS Resolution Failed.
	computePool = "Pool1"
	_, err = getTargets(computePool, &DNSResolver{})
	if err == nil {
		t.Errorf("Expected an error, but got no error")
	}
	require.Contains(t, err.Error(), "SPCS discovery plugin: error resolving endpoint.", "Error message mismatch")
}

func TestSPCSSDConfigUnmarshalYAML(t *testing.T) {
	os.Setenv("SNOWFLAKE_ACCOUNT", TestAccountIdentifer)
	os.Setenv("SNOWFLAKE_HOST", TestHost)
	defer func() {
		// Unset the environment variable.
		os.Unsetenv("SNOWFLAKE_ACCOUNT")
		os.Unsetenv("SNOWFLAKE_HOST")
	}()

	tests := []struct {
		name     string
		yamlData string
		expected SPCSSDConfig
		errMsg   string
	}{
		{
			name: "Valid YAML with Authenticator",
			yamlData: `
						 authenticator: "AuthTypeToken"
						 `,
			expected: SPCSSDConfig{
				Account:         TestAccountIdentifer,
				Host:            TestHost,
				Authenticator:   AuthTypeToken,
				Port:            DefaultSPCSSDConfig.Port,
				Protocol:        DefaultSPCSSDConfig.Protocol,
				RefreshInterval: DefaultSPCSSDConfig.RefreshInterval,
			},
			errMsg: "",
		},
		{
			name: "Valid YAML with Custom Authenticator Token",
			yamlData: `
						 account: "testAccount"
						 token_path: "/path/to/token"
						 authenticator: "AuthTypeToken"
						 `,
			expected: SPCSSDConfig{
				Account:         TestAccount,
				Host:            TestHost,
				TokenPath:       TestTokenPath,
				Authenticator:   AuthTypeToken,
				Port:            DefaultSPCSSDConfig.Port,
				Protocol:        DefaultSPCSSDConfig.Protocol,
				RefreshInterval: DefaultSPCSSDConfig.RefreshInterval,
			},
			errMsg: "",
		},
		{
			name: "Valid YAML with Authenticator Password",
			yamlData: `
						 account: "testAccount"
						 host: "testHost"
						 user: "testUser"
						 password: "testPassword"
						 role: "testRole"
						 authenticator: "AuthTypePassword"						
						 `,
			expected: SPCSSDConfig{
				Account:         TestAccount,
				Host:            TestHost,
				Authenticator:   AuthTypePassword,
				User:            TestUser,
				Password:        TestPassword,
				Role:            TestRole,
				Port:            DefaultSPCSSDConfig.Port,
				Protocol:        DefaultSPCSSDConfig.Protocol,
				RefreshInterval: DefaultSPCSSDConfig.RefreshInterval,
			},
			errMsg: "",
		},
		{
			name: "Missing Authenticator",
			yamlData: `
						 account: "myAccount"
						 host: "myHost"						
						 token_path: "/path/to/token"
						 `,
			expected: SPCSSDConfig{},
			errMsg:   "Spcs SD configuration requires an Authenticator to be specified. Supported Authenticator types are AuthTypeToken, AuthTypePassword",
		},
		{
			name: "Unsupported Authenticator",
			yamlData: `
						 account: "testOrganization-testAccount"
						 authenticator: "InvalidAuthenticator"
						 `,
			expected: SPCSSDConfig{},
			errMsg:   "Spcs SD configuration unsupported Authenticator InvalidAuthenticator. Supported Authenticator types are AuthTypeToken, AuthTypePassword",
		},
		{
			name: "Missing User, Password, and Role for AuthTypePassword",
			yamlData: `
						 account: "myAccount"
						 host: "myHost"
						 user: "myUser"
						 authenticator: "AuthTypePassword"						
						 `,
			expected: SPCSSDConfig{},
			errMsg:   "Spcs SD configuration requires { User, Password, Role } when Authenticator type is AuthTypePassword",
		},
	}

	unmarshal := func(d []byte) func(interface{}) error {
		return func(o interface{}) error {
			return yaml.Unmarshal(d, o)
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var config SPCSSDConfig
			err := config.UnmarshalYAML(unmarshal([]byte(removeLeadingSpaces(test.yamlData))))
			if test.errMsg != "" {
				require.NotNil(t, err, "Expected an error")
				require.Equal(t, test.errMsg, err.Error(), "Error message mismatch")
			} else {
				require.Nil(t, err, "Expected no error")
				require.Equal(t, test.expected, config, "Config mismatch")
			}
		})
	}
}

// Implement a mock resolver for testing.
type MockEndpointResolver struct {
	ipAddresses []string
}

func (m *MockEndpointResolver) ResolveEndPoint(endPoint string) ([]string, error) {
	// Return predefined IP addresses for testing.
	return m.ipAddresses, nil
}

func removeLeadingSpaces(input string) string {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimLeft(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func verifyResult(t *testing.T, err error, result, expected []string) {
	if err != nil {
		t.Errorf("Expected no error, but got an error: %v", err)
	}
	if len(result) != len(expected) {
		t.Errorf("Expected %v, but got %v", expected, result)
	}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("Expected %v, but got %v", expected[i], result[i])
		}
	}
}

func verifyTargets(t *testing.T, err error, resultTargets, expectedTargets []model.LabelSet) {
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}

	if !labelSetsEqual(resultTargets, expectedTargets) {
		t.Errorf("Expected %v, but got %v", expectedTargets, resultTargets)
	}
}

func labelSetsEqual(a, b []model.LabelSet) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !labelSetEqual(a[i], b[i]) {
			return false
		}
	}

	return true
}

func labelSetEqual(a, b model.LabelSet) bool {
	if len(a) != len(b) {
		return false
	}

	for nameA, valueA := range a {
		valueB, exists := b[nameA]
		if !exists || valueA != valueB {
			return false
		}
	}

	// Check if all labels in b also exist in a.
	for nameB := range b {
		_, exists := a[nameB]
		if !exists {
			return false
		}
	}

	return true
}
