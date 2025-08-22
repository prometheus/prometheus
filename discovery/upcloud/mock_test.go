// Copyright 2025 The Prometheus Authors
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

package upcloud

import (
	"context"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
)

// MockUpCloudClient is a mock implementation of the UpCloudClient interface for testing.
type MockUpCloudClient struct {
	servers       *upcloud.Servers
	serverDetails map[string]*upcloud.ServerDetails
	err           error
}

// NewMockUpCloudClient creates a new mock client.
func NewMockUpCloudClient() *MockUpCloudClient {
	return &MockUpCloudClient{
		serverDetails: make(map[string]*upcloud.ServerDetails),
	}
}

// SetServers sets the servers that will be returned by GetServers.
func (m *MockUpCloudClient) SetServers(servers *upcloud.Servers) {
	m.servers = servers
}

// SetServerDetails sets the server details for a specific UUID.
func (m *MockUpCloudClient) SetServerDetails(uuid string, details *upcloud.ServerDetails) {
	m.serverDetails[uuid] = details
}

// SetError sets an error that will be returned by GetServers.
func (m *MockUpCloudClient) SetError(err error) {
	m.err = err
}

// GetServers implements the UpCloudClient interface.
func (m *MockUpCloudClient) GetServers(_ context.Context) (*upcloud.Servers, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.servers, nil
}

// GetServerDetails implements the UpCloudClient interface.
func (m *MockUpCloudClient) GetServerDetails(_ context.Context, uuid string) (*upcloud.ServerDetails, error) {
	if m.err != nil {
		return nil, m.err
	}
	if details, ok := m.serverDetails[uuid]; ok {
		return details, nil
	}
	return nil, nil
}

// CreateTestServers creates a set of test servers for testing.
func CreateTestServers() *upcloud.Servers {
	return &upcloud.Servers{
		Servers: []upcloud.Server{
			{
				UUID:         "00798b85-efdc-41ca-8021-f6ef457b8531",
				Title:        "web-server-1",
				Hostname:     "web1.example.com",
				Zone:         "fi-hel1",
				Plan:         "1xCPU-1GB",
				State:        "started",
				CoreNumber:   1,
				MemoryAmount: 1024,
				Tags:         upcloud.ServerTagSlice{"production", "web"},
			},
			{
				UUID:         "01798b85-efdc-41ca-8021-f6ef457b8532",
				Title:        "db-server-1",
				Hostname:     "db1.example.com",
				Zone:         "fi-hel2",
				Plan:         "2xCPU-4GB",
				State:        "started",
				CoreNumber:   2,
				MemoryAmount: 4096,
				Tags:         upcloud.ServerTagSlice{"production", "database"},
			},
			{
				UUID:         "02798b85-efdc-41ca-8021-f6ef457b8533",
				Title:        "test-server-1",
				Hostname:     "test1.example.com",
				Zone:         "us-chi1",
				Plan:         "1xCPU-2GB",
				State:        "stopped",
				CoreNumber:   1,
				MemoryAmount: 2048,
				Tags:         upcloud.ServerTagSlice{"development"},
			},
		},
	}
}

// CreateEmptyServers creates an empty server list for testing.
func CreateEmptyServers() *upcloud.Servers {
	return &upcloud.Servers{
		Servers: []upcloud.Server{},
	}
}

// CreateSingleTestServer creates a single test server for testing.
func CreateSingleTestServer() *upcloud.Servers {
	return &upcloud.Servers{
		Servers: []upcloud.Server{
			{
				UUID:         "test-server",
				Title:        "test-server",
				Hostname:     "test.example.com",
				Zone:         "fi-hel1",
				Plan:         "1xCPU-1GB",
				State:        "started",
				CoreNumber:   1,
				MemoryAmount: 1024,
				Tags:         upcloud.ServerTagSlice{"test"},
			},
		},
	}
}

// CreateServersWithoutIPs creates servers that have no IP addresses.
func CreateServersWithoutIPs() *upcloud.Servers {
	return &upcloud.Servers{
		Servers: []upcloud.Server{
			{
				UUID:         "no-ip-server",
				Title:        "no-ip-server",
				Hostname:     "noip.example.com",
				Zone:         "fi-hel1",
				Plan:         "1xCPU-1GB",
				State:        "started",
				CoreNumber:   1,
				MemoryAmount: 1024,
				Tags:         upcloud.ServerTagSlice{"test"},
			},
		},
	}
}

// CreateTestServerDetails creates detailed server information for testing.
func CreateTestServerDetails() map[string]*upcloud.ServerDetails {
	return map[string]*upcloud.ServerDetails{
		"00798b85-efdc-41ca-8021-f6ef457b8531": {
			Server: upcloud.Server{
				UUID:         "00798b85-efdc-41ca-8021-f6ef457b8531",
				Title:        "web-server-1",
				Hostname:     "web1.example.com",
				Zone:         "fi-hel1",
				Plan:         "1xCPU-1GB",
				State:        "started",
				CoreNumber:   1,
				MemoryAmount: 1024,
				Tags:         upcloud.ServerTagSlice{"production", "web"},
			},
			IPAddresses: upcloud.IPAddressSlice{
				{
					Access:  upcloud.IPAddressAccessPublic,
					Address: "192.0.2.1",
					Family:  upcloud.IPAddressFamilyIPv4,
				},
				{
					Access:  upcloud.IPAddressAccessPrivate,
					Address: "10.0.0.1",
					Family:  upcloud.IPAddressFamilyIPv4,
				},
				{
					Access:  upcloud.IPAddressAccessPublic,
					Address: "2001:db8::1",
					Family:  upcloud.IPAddressFamilyIPv6,
				},
			},
			Labels: upcloud.LabelSlice{
				{Key: "environment", Value: "production"},
				{Key: "team", Value: "platform"},
			},
		},
		"01798b85-efdc-41ca-8021-f6ef457b8532": {
			Server: upcloud.Server{
				UUID:         "01798b85-efdc-41ca-8021-f6ef457b8532",
				Title:        "db-server-1",
				Hostname:     "db1.example.com",
				Zone:         "fi-hel2",
				Plan:         "2xCPU-4GB",
				State:        "started",
				CoreNumber:   2,
				MemoryAmount: 4096,
				Tags:         upcloud.ServerTagSlice{"production", "database"},
			},
			IPAddresses: upcloud.IPAddressSlice{
				{
					Access:  upcloud.IPAddressAccessPublic,
					Address: "192.0.2.2",
					Family:  upcloud.IPAddressFamilyIPv4,
				},
				{
					Access:  upcloud.IPAddressAccessPrivate,
					Address: "10.0.0.2",
					Family:  upcloud.IPAddressFamilyIPv4,
				},
			},
		},
		"02798b85-efdc-41ca-8021-f6ef457b8533": {
			Server: upcloud.Server{
				UUID:         "02798b85-efdc-41ca-8021-f6ef457b8533",
				Title:        "test-server-1",
				Hostname:     "test1.example.com",
				Zone:         "us-chi1",
				Plan:         "1xCPU-2GB",
				State:        "stopped",
				CoreNumber:   1,
				MemoryAmount: 2048,
				Tags:         upcloud.ServerTagSlice{"development"},
			},
			IPAddresses: upcloud.IPAddressSlice{
				{
					Access:  upcloud.IPAddressAccessPrivate,
					Address: "10.0.0.3",
					Family:  upcloud.IPAddressFamilyIPv4,
				},
			},
			Labels: upcloud.LabelSlice{
				{Key: "environment", Value: "development"},
			},
		},
	}
}

// CreateServerDetailsWithoutIPs creates server details with no IP addresses.
func CreateServerDetailsWithoutIPs() map[string]*upcloud.ServerDetails {
	return map[string]*upcloud.ServerDetails{
		"no-ip-server": {
			Server: upcloud.Server{
				UUID:         "no-ip-server",
				Title:        "no-ip-server",
				Hostname:     "noip.example.com",
				Zone:         "fi-hel1",
				Plan:         "1xCPU-1GB",
				State:        "started",
				CoreNumber:   1,
				MemoryAmount: 1024,
				Tags:         upcloud.ServerTagSlice{"test"},
			},
			IPAddresses: upcloud.IPAddressSlice{},
			Labels:      upcloud.LabelSlice{},
		},
	}
}

// CreateServerDetailsWithIPv6Only creates server details with only IPv6 addresses.
func CreateServerDetailsWithIPv6Only() map[string]*upcloud.ServerDetails {
	return map[string]*upcloud.ServerDetails{
		"test-server": {
			Server: upcloud.Server{
				UUID:         "test-server",
				Title:        "test-server",
				Hostname:     "test.example.com",
				Zone:         "fi-hel1",
				Plan:         "1xCPU-1GB",
				State:        "started",
				CoreNumber:   1,
				MemoryAmount: 1024,
				Tags:         upcloud.ServerTagSlice{"test"},
			},
			IPAddresses: upcloud.IPAddressSlice{
				{
					Access:  upcloud.IPAddressAccessPublic,
					Address: "2001:db8::1",
					Family:  upcloud.IPAddressFamilyIPv6,
				},
			},
			Labels: upcloud.LabelSlice{},
		},
	}
}

// CreateServerDetailsWithPrivateOnly creates server details with only private IP addresses.
func CreateServerDetailsWithPrivateOnly() map[string]*upcloud.ServerDetails {
	return map[string]*upcloud.ServerDetails{
		"test-server": {
			Server: upcloud.Server{
				UUID:         "test-server",
				Title:        "test-server",
				Hostname:     "test.example.com",
				Zone:         "fi-hel1",
				Plan:         "1xCPU-1GB",
				State:        "started",
				CoreNumber:   1,
				MemoryAmount: 1024,
				Tags:         upcloud.ServerTagSlice{"test"},
			},
			IPAddresses: upcloud.IPAddressSlice{
				{
					Access:  upcloud.IPAddressAccessPrivate,
					Address: "10.0.0.1",
					Family:  upcloud.IPAddressFamilyIPv4,
				},
			},
			Labels: upcloud.LabelSlice{},
		},
	}
}
