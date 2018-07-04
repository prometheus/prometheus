// Copyright (c) 2016, 2018, Oracle and/or its affiliates. All rights reserved.
// Code generated. DO NOT EDIT.

// Core Services API
//
// APIs for Networking Service, Compute Service, and Block Volume Service.
//

package core

import (
	"context"
	"fmt"
	"github.com/oracle/oci-go-sdk/common"
	"net/http"
)

//VirtualNetworkClient a client for VirtualNetwork
type VirtualNetworkClient struct {
	common.BaseClient
	config *common.ConfigurationProvider
}

// NewVirtualNetworkClientWithConfigurationProvider Creates a new default VirtualNetwork client with the given configuration provider.
// the configuration provider will be used for the default signer as well as reading the region
func NewVirtualNetworkClientWithConfigurationProvider(configProvider common.ConfigurationProvider) (client VirtualNetworkClient, err error) {
	baseClient, err := common.NewClientWithConfig(configProvider)
	if err != nil {
		return
	}

	client = VirtualNetworkClient{BaseClient: baseClient}
	client.BasePath = "20160918"
	err = client.setConfigurationProvider(configProvider)
	return
}

// SetRegion overrides the region of this client.
func (client *VirtualNetworkClient) SetRegion(region string) {
	client.Host = fmt.Sprintf(common.DefaultHostURLTemplate, "iaas", region)
}

// SetConfigurationProvider sets the configuration provider including the region, returns an error if is not valid
func (client *VirtualNetworkClient) setConfigurationProvider(configProvider common.ConfigurationProvider) error {
	if ok, err := common.IsConfigurationProviderValid(configProvider); !ok {
		return err
	}

	// Error has been checked already
	region, _ := configProvider.Region()
	client.SetRegion(region)
	client.config = &configProvider
	return nil
}

// ConfigurationProvider the ConfigurationProvider used in this client, or null if none set
func (client *VirtualNetworkClient) ConfigurationProvider() *common.ConfigurationProvider {
	return client.config
}

// AttachServiceId Enables the specified service on the specified gateway. In other words, enables the service
// gateway to send traffic to the specified service. You must also set up a route rule with the
// service's `cidrBlock` as the rule's destination CIDR and the gateway as the rule's target.
// See RouteTable.
// **Note:** The `AttachServiceId` operation is an easy way to enable an individual service on
// the service gateway. Compare it with
// UpdateServiceGateway, which also
// lets you enable an individual service. However, with `UpdateServiceGateway`, you must specify
// the *entire* list of services you want enabled on the service gateway.
func (client VirtualNetworkClient) AttachServiceId(ctx context.Context, request AttachServiceIdRequest) (response AttachServiceIdResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.attachServiceId, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(AttachServiceIdResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into AttachServiceIdResponse")
	}
	return
}

// attachServiceId implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) attachServiceId(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/serviceGateways/{serviceGatewayId}/actions/attachService")
	if err != nil {
		return nil, err
	}

	var response AttachServiceIdResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// BulkAddVirtualCircuitPublicPrefixes Adds one or more customer public IP prefixes to the specified public virtual circuit.
// Use this operation (and not UpdateVirtualCircuit)
// to add prefixes to the virtual circuit. Oracle must verify the customer's ownership
// of each prefix before traffic for that prefix will flow across the virtual circuit.
func (client VirtualNetworkClient) BulkAddVirtualCircuitPublicPrefixes(ctx context.Context, request BulkAddVirtualCircuitPublicPrefixesRequest) (err error) {
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	_, err = common.Retry(ctx, request, client.bulkAddVirtualCircuitPublicPrefixes, policy)
	return
}

// bulkAddVirtualCircuitPublicPrefixes implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) bulkAddVirtualCircuitPublicPrefixes(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/virtualCircuits/{virtualCircuitId}/actions/bulkAddPublicPrefixes")
	if err != nil {
		return nil, err
	}

	var response BulkAddVirtualCircuitPublicPrefixesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// BulkDeleteVirtualCircuitPublicPrefixes Removes one or more customer public IP prefixes from the specified public virtual circuit.
// Use this operation (and not UpdateVirtualCircuit)
// to remove prefixes from the virtual circuit. When the virtual circuit's state switches
// back to PROVISIONED, Oracle stops advertising the specified prefixes across the connection.
func (client VirtualNetworkClient) BulkDeleteVirtualCircuitPublicPrefixes(ctx context.Context, request BulkDeleteVirtualCircuitPublicPrefixesRequest) (err error) {
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	_, err = common.Retry(ctx, request, client.bulkDeleteVirtualCircuitPublicPrefixes, policy)
	return
}

// bulkDeleteVirtualCircuitPublicPrefixes implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) bulkDeleteVirtualCircuitPublicPrefixes(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/virtualCircuits/{virtualCircuitId}/actions/bulkDeletePublicPrefixes")
	if err != nil {
		return nil, err
	}

	var response BulkDeleteVirtualCircuitPublicPrefixesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ConnectLocalPeeringGateways Connects this local peering gateway (LPG) to another one in the same region.
// This operation must be called by the VCN administrator who is designated as
// the *requestor* in the peering relationship. The *acceptor* must implement
// an Identity and Access Management (IAM) policy that gives the requestor permission
// to connect to LPGs in the acceptor's compartment. Without that permission, this
// operation will fail. For more information, see
// VCN Peering (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/VCNpeering.htm).
func (client VirtualNetworkClient) ConnectLocalPeeringGateways(ctx context.Context, request ConnectLocalPeeringGatewaysRequest) (response ConnectLocalPeeringGatewaysResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.connectLocalPeeringGateways, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ConnectLocalPeeringGatewaysResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ConnectLocalPeeringGatewaysResponse")
	}
	return
}

// connectLocalPeeringGateways implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) connectLocalPeeringGateways(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/localPeeringGateways/{localPeeringGatewayId}/actions/connect")
	if err != nil {
		return nil, err
	}

	var response ConnectLocalPeeringGatewaysResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ConnectRemotePeeringConnections Connects this RPC to another one in a different region.
// This operation must be called by the VCN administrator who is designated as
// the *requestor* in the peering relationship. The *acceptor* must implement
// an Identity and Access Management (IAM) policy that gives the requestor permission
// to connect to RPCs in the acceptor's compartment. Without that permission, this
// operation will fail. For more information, see
// VCN Peering (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/VCNpeering.htm).
func (client VirtualNetworkClient) ConnectRemotePeeringConnections(ctx context.Context, request ConnectRemotePeeringConnectionsRequest) (response ConnectRemotePeeringConnectionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.connectRemotePeeringConnections, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ConnectRemotePeeringConnectionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ConnectRemotePeeringConnectionsResponse")
	}
	return
}

// connectRemotePeeringConnections implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) connectRemotePeeringConnections(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/remotePeeringConnections/{remotePeeringConnectionId}/actions/connect")
	if err != nil {
		return nil, err
	}

	var response ConnectRemotePeeringConnectionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateCpe Creates a new virtual Customer-Premises Equipment (CPE) object in the specified compartment. For
// more information, see IPSec VPNs (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingIPsec.htm).
// For the purposes of access control, you must provide the OCID of the compartment where you want
// the CPE to reside. Notice that the CPE doesn't have to be in the same compartment as the IPSec
// connection or other Networking Service components. If you're not sure which compartment to
// use, put the CPE in the same compartment as the DRG. For more information about
// compartments and access control, see Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
// For information about OCIDs, see Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You must provide the public IP address of your on-premises router. See
// Configuring Your On-Premises Router for an IPSec VPN (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/configuringCPE.htm).
// You may optionally specify a *display name* for the CPE, otherwise a default is provided. It does not have to
// be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateCpe(ctx context.Context, request CreateCpeRequest) (response CreateCpeResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createCpe, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateCpeResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateCpeResponse")
	}
	return
}

// createCpe implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createCpe(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/cpes")
	if err != nil {
		return nil, err
	}

	var response CreateCpeResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateCrossConnect Creates a new cross-connect. Oracle recommends you create each cross-connect in a
// CrossConnectGroup so you can use link aggregation
// with the connection.
// After creating the `CrossConnect` object, you need to go the FastConnect location
// and request to have the physical cable installed. For more information, see
// FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
// For the purposes of access control, you must provide the OCID of the
// compartment where you want the cross-connect to reside. If you're
// not sure which compartment to use, put the cross-connect in the
// same compartment with your VCN. For more information about
// compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
// For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the cross-connect.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateCrossConnect(ctx context.Context, request CreateCrossConnectRequest) (response CreateCrossConnectResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createCrossConnect, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateCrossConnectResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateCrossConnectResponse")
	}
	return
}

// createCrossConnect implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createCrossConnect(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/crossConnects")
	if err != nil {
		return nil, err
	}

	var response CreateCrossConnectResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateCrossConnectGroup Creates a new cross-connect group to use with Oracle Cloud Infrastructure
// FastConnect. For more information, see
// FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
// For the purposes of access control, you must provide the OCID of the
// compartment where you want the cross-connect group to reside. If you're
// not sure which compartment to use, put the cross-connect group in the
// same compartment with your VCN. For more information about
// compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
// For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the cross-connect group.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateCrossConnectGroup(ctx context.Context, request CreateCrossConnectGroupRequest) (response CreateCrossConnectGroupResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createCrossConnectGroup, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateCrossConnectGroupResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateCrossConnectGroupResponse")
	}
	return
}

// createCrossConnectGroup implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createCrossConnectGroup(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/crossConnectGroups")
	if err != nil {
		return nil, err
	}

	var response CreateCrossConnectGroupResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateDhcpOptions Creates a new set of DHCP options for the specified VCN. For more information, see
// DhcpOptions.
// For the purposes of access control, you must provide the OCID of the compartment where you want the set of
// DHCP options to reside. Notice that the set of options doesn't have to be in the same compartment as the VCN,
// subnets, or other Networking Service components. If you're not sure which compartment to use, put the set
// of DHCP options in the same compartment as the VCN. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm). For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the set of DHCP options, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateDhcpOptions(ctx context.Context, request CreateDhcpOptionsRequest) (response CreateDhcpOptionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createDhcpOptions, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateDhcpOptionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateDhcpOptionsResponse")
	}
	return
}

// createDhcpOptions implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createDhcpOptions(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/dhcps")
	if err != nil {
		return nil, err
	}

	var response CreateDhcpOptionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateDrg Creates a new Dynamic Routing Gateway (DRG) in the specified compartment. For more information,
// see Dynamic Routing Gateways (DRGs) (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingDRGs.htm).
// For the purposes of access control, you must provide the OCID of the compartment where you want
// the DRG to reside. Notice that the DRG doesn't have to be in the same compartment as the VCN,
// the DRG attachment, or other Networking Service components. If you're not sure which compartment
// to use, put the DRG in the same compartment as the VCN. For more information about compartments
// and access control, see Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
// For information about OCIDs, see Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the DRG, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateDrg(ctx context.Context, request CreateDrgRequest) (response CreateDrgResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createDrg, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateDrgResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateDrgResponse")
	}
	return
}

// createDrg implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createDrg(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/drgs")
	if err != nil {
		return nil, err
	}

	var response CreateDrgResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateDrgAttachment Attaches the specified DRG to the specified VCN. A VCN can be attached to only one DRG at a time,
// and vice versa. The response includes a `DrgAttachment` object with its own OCID. For more
// information about DRGs, see
// Dynamic Routing Gateways (DRGs) (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingDRGs.htm).
// You may optionally specify a *display name* for the attachment, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
// For the purposes of access control, the DRG attachment is automatically placed into the same compartment
// as the VCN. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
func (client VirtualNetworkClient) CreateDrgAttachment(ctx context.Context, request CreateDrgAttachmentRequest) (response CreateDrgAttachmentResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createDrgAttachment, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateDrgAttachmentResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateDrgAttachmentResponse")
	}
	return
}

// createDrgAttachment implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createDrgAttachment(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/drgAttachments")
	if err != nil {
		return nil, err
	}

	var response CreateDrgAttachmentResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateIPSecConnection Creates a new IPSec connection between the specified DRG and CPE. For more information, see
// IPSec VPNs (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingIPsec.htm).
// In the request, you must include at least one static route to the CPE object (you're allowed a maximum
// of 10). For example: 10.0.8.0/16.
// For the purposes of access control, you must provide the OCID of the compartment where you want the
// IPSec connection to reside. Notice that the IPSec connection doesn't have to be in the same compartment
// as the DRG, CPE, or other Networking Service components. If you're not sure which compartment to
// use, put the IPSec connection in the same compartment as the DRG. For more information about
// compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
// For information about OCIDs, see Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the IPSec connection, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
// After creating the IPSec connection, you need to configure your on-premises router
// with tunnel-specific information returned by
// GetIPSecConnectionDeviceConfig.
// For each tunnel, that operation gives you the IP address of Oracle's VPN headend and the shared secret
// (that is, the pre-shared key). For more information, see
// Configuring Your On-Premises Router for an IPSec VPN (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/configuringCPE.htm).
// To get the status of the tunnels (whether they're up or down), use
// GetIPSecConnectionDeviceStatus.
func (client VirtualNetworkClient) CreateIPSecConnection(ctx context.Context, request CreateIPSecConnectionRequest) (response CreateIPSecConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createIPSecConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateIPSecConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateIPSecConnectionResponse")
	}
	return
}

// createIPSecConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createIPSecConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/ipsecConnections")
	if err != nil {
		return nil, err
	}

	var response CreateIPSecConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateInternetGateway Creates a new Internet Gateway for the specified VCN. For more information, see
// Access to the Internet (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingIGs.htm).
// For the purposes of access control, you must provide the OCID of the compartment where you want the Internet
// Gateway to reside. Notice that the Internet Gateway doesn't have to be in the same compartment as the VCN or
// other Networking Service components. If you're not sure which compartment to use, put the Internet
// Gateway in the same compartment with the VCN. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm). For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the Internet Gateway, otherwise a default is provided. It
// does not have to be unique, and you can change it. Avoid entering confidential information.
// For traffic to flow between a subnet and an Internet Gateway, you must create a route rule accordingly in
// the subnet's route table (for example, 0.0.0.0/0 > Internet Gateway). See
// UpdateRouteTable.
// You must specify whether the Internet Gateway is enabled when you create it. If it's disabled, that means no
// traffic will flow to/from the internet even if there's a route rule that enables that traffic. You can later
// use UpdateInternetGateway to easily disable/enable
// the gateway without changing the route rule.
func (client VirtualNetworkClient) CreateInternetGateway(ctx context.Context, request CreateInternetGatewayRequest) (response CreateInternetGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createInternetGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateInternetGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateInternetGatewayResponse")
	}
	return
}

// createInternetGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createInternetGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/internetGateways")
	if err != nil {
		return nil, err
	}

	var response CreateInternetGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateLocalPeeringGateway Creates a new local peering gateway (LPG) for the specified VCN.
func (client VirtualNetworkClient) CreateLocalPeeringGateway(ctx context.Context, request CreateLocalPeeringGatewayRequest) (response CreateLocalPeeringGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createLocalPeeringGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateLocalPeeringGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateLocalPeeringGatewayResponse")
	}
	return
}

// createLocalPeeringGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createLocalPeeringGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/localPeeringGateways")
	if err != nil {
		return nil, err
	}

	var response CreateLocalPeeringGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreatePrivateIp Creates a secondary private IP for the specified VNIC.
// For more information about secondary private IPs, see
// IP Addresses (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingIPaddresses.htm).
func (client VirtualNetworkClient) CreatePrivateIp(ctx context.Context, request CreatePrivateIpRequest) (response CreatePrivateIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createPrivateIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreatePrivateIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreatePrivateIpResponse")
	}
	return
}

// createPrivateIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createPrivateIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/privateIps")
	if err != nil {
		return nil, err
	}

	var response CreatePrivateIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreatePublicIp Creates a public IP. Use the `lifetime` property to specify whether it's an ephemeral or
// reserved public IP. For information about limits on how many you can create, see
// Public IP Addresses (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingpublicIPs.htm).
// * **For an ephemeral public IP:** You must also specify a `privateIpId` with the OCID of
// the primary private IP you want to assign the public IP to. The public IP is created in
// the same Availability Domain as the private IP. An ephemeral public IP must always be
// assigned to a private IP, and only to the *primary* private IP on a VNIC, not a secondary
// private IP.
// * **For a reserved public IP:** You may also optionally assign the public IP to a private
// IP by specifying `privateIpId`. Or you can later assign the public IP with
// UpdatePublicIp.
// **Note:** When assigning a public IP to a private IP, the private IP must not already have
// a public IP with `lifecycleState` = ASSIGNING or ASSIGNED. If it does, an error is returned.
// Also, for reserved public IPs, the optional assignment part of this operation is
// asynchronous. Poll the public IP's `lifecycleState` to determine if the assignment
// succeeded.
func (client VirtualNetworkClient) CreatePublicIp(ctx context.Context, request CreatePublicIpRequest) (response CreatePublicIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createPublicIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreatePublicIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreatePublicIpResponse")
	}
	return
}

// createPublicIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createPublicIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/publicIps")
	if err != nil {
		return nil, err
	}

	var response CreatePublicIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateRemotePeeringConnection Creates a new remote peering connection (RPC) for the specified DRG.
func (client VirtualNetworkClient) CreateRemotePeeringConnection(ctx context.Context, request CreateRemotePeeringConnectionRequest) (response CreateRemotePeeringConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createRemotePeeringConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateRemotePeeringConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateRemotePeeringConnectionResponse")
	}
	return
}

// createRemotePeeringConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createRemotePeeringConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/remotePeeringConnections")
	if err != nil {
		return nil, err
	}

	var response CreateRemotePeeringConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateRouteTable Creates a new route table for the specified VCN. In the request you must also include at least one route
// rule for the new route table. For information on the number of rules you can have in a route table, see
// Service Limits (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/servicelimits.htm). For general information about route
// tables in your VCN and the types of targets you can use in route rules,
// see Route Tables (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingroutetables.htm).
// For the purposes of access control, you must provide the OCID of the compartment where you want the route
// table to reside. Notice that the route table doesn't have to be in the same compartment as the VCN, subnets,
// or other Networking Service components. If you're not sure which compartment to use, put the route
// table in the same compartment as the VCN. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm). For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the route table, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateRouteTable(ctx context.Context, request CreateRouteTableRequest) (response CreateRouteTableResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createRouteTable, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateRouteTableResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateRouteTableResponse")
	}
	return
}

// createRouteTable implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createRouteTable(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/routeTables")
	if err != nil {
		return nil, err
	}

	var response CreateRouteTableResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateSecurityList Creates a new security list for the specified VCN. For more information
// about security lists, see Security Lists (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/securitylists.htm).
// For information on the number of rules you can have in a security list, see
// Service Limits (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/servicelimits.htm).
// For the purposes of access control, you must provide the OCID of the compartment where you want the security
// list to reside. Notice that the security list doesn't have to be in the same compartment as the VCN, subnets,
// or other Networking Service components. If you're not sure which compartment to use, put the security
// list in the same compartment as the VCN. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm). For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the security list, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateSecurityList(ctx context.Context, request CreateSecurityListRequest) (response CreateSecurityListResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createSecurityList, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateSecurityListResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateSecurityListResponse")
	}
	return
}

// createSecurityList implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createSecurityList(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/securityLists")
	if err != nil {
		return nil, err
	}

	var response CreateSecurityListResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateServiceGateway Creates a new service gateway in the specified compartment.
// For the purposes of access control, you must provide the OCID of the compartment where you want
// the service gateway to reside. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
// For information about OCIDs, see Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the service gateway, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
func (client VirtualNetworkClient) CreateServiceGateway(ctx context.Context, request CreateServiceGatewayRequest) (response CreateServiceGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createServiceGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateServiceGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateServiceGatewayResponse")
	}
	return
}

// createServiceGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createServiceGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/serviceGateways")
	if err != nil {
		return nil, err
	}

	var response CreateServiceGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateSubnet Creates a new subnet in the specified VCN. You can't change the size of the subnet after creation,
// so it's important to think about the size of subnets you need before creating them.
// For more information, see VCNs and Subnets (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingVCNs.htm).
// For information on the number of subnets you can have in a VCN, see
// Service Limits (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/servicelimits.htm).
// For the purposes of access control, you must provide the OCID of the compartment where you want the subnet
// to reside. Notice that the subnet doesn't have to be in the same compartment as the VCN, route tables, or
// other Networking Service components. If you're not sure which compartment to use, put the subnet in
// the same compartment as the VCN. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm). For information about OCIDs,
// see Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally associate a route table with the subnet. If you don't, the subnet will use the
// VCN's default route table. For more information about route tables, see
// Route Tables (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingroutetables.htm).
// You may optionally associate a security list with the subnet. If you don't, the subnet will use the
// VCN's default security list. For more information about security lists, see
// Security Lists (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/securitylists.htm).
// You may optionally associate a set of DHCP options with the subnet. If you don't, the subnet will use the
// VCN's default set. For more information about DHCP options, see
// DHCP Options (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingDHCP.htm).
// You may optionally specify a *display name* for the subnet, otherwise a default is provided.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
// You can also add a DNS label for the subnet, which is required if you want the Internet and
// VCN Resolver to resolve hostnames for instances in the subnet. For more information, see
// DNS in Your Virtual Cloud Network (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/dns.htm).
func (client VirtualNetworkClient) CreateSubnet(ctx context.Context, request CreateSubnetRequest) (response CreateSubnetResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createSubnet, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateSubnetResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateSubnetResponse")
	}
	return
}

// createSubnet implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createSubnet(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/subnets")
	if err != nil {
		return nil, err
	}

	var response CreateSubnetResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateVcn Creates a new Virtual Cloud Network (VCN). For more information, see
// VCNs and Subnets (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingVCNs.htm).
// For the VCN you must specify a single, contiguous IPv4 CIDR block. Oracle recommends using one of the
// private IP address ranges specified in RFC 1918 (https://tools.ietf.org/html/rfc1918) (10.0.0.0/8,
// 172.16/12, and 192.168/16). Example: 172.16.0.0/16. The CIDR block can range from /16 to /30, and it
// must not overlap with your on-premises network. You can't change the size of the VCN after creation.
// For the purposes of access control, you must provide the OCID of the compartment where you want the VCN to
// reside. Consult an Oracle Cloud Infrastructure administrator in your organization if you're not sure which
// compartment to use. Notice that the VCN doesn't have to be in the same compartment as the subnets or other
// Networking Service components. For more information about compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm). For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the VCN, otherwise a default is provided. It does not have to
// be unique, and you can change it. Avoid entering confidential information.
// You can also add a DNS label for the VCN, which is required if you want the instances to use the
// Interent and VCN Resolver option for DNS in the VCN. For more information, see
// DNS in Your Virtual Cloud Network (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/dns.htm).
// The VCN automatically comes with a default route table, default security list, and default set of DHCP options.
// The OCID for each is returned in the response. You can't delete these default objects, but you can change their
// contents (that is, change the route rules, security list rules, and so on).
// The VCN and subnets you create are not accessible until you attach an Internet Gateway or set up an IPSec VPN
// or FastConnect. For more information, see
// Overview of the Networking Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/overview.htm).
func (client VirtualNetworkClient) CreateVcn(ctx context.Context, request CreateVcnRequest) (response CreateVcnResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createVcn, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateVcnResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateVcnResponse")
	}
	return
}

// createVcn implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createVcn(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/vcns")
	if err != nil {
		return nil, err
	}

	var response CreateVcnResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// CreateVirtualCircuit Creates a new virtual circuit to use with Oracle Cloud
// Infrastructure FastConnect. For more information, see
// FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
// For the purposes of access control, you must provide the OCID of the
// compartment where you want the virtual circuit to reside. If you're
// not sure which compartment to use, put the virtual circuit in the
// same compartment with the DRG it's using. For more information about
// compartments and access control, see
// Overview of the IAM Service (https://docs.us-phoenix-1.oraclecloud.com/Content/Identity/Concepts/overview.htm).
// For information about OCIDs, see
// Resource Identifiers (https://docs.us-phoenix-1.oraclecloud.com/Content/General/Concepts/identifiers.htm).
// You may optionally specify a *display name* for the virtual circuit.
// It does not have to be unique, and you can change it. Avoid entering confidential information.
// **Important:** When creating a virtual circuit, you specify a DRG for
// the traffic to flow through. Make sure you attach the DRG to your
// VCN and confirm the VCN's routing sends traffic to the DRG. Otherwise
// traffic will not flow. For more information, see
// Route Tables (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingroutetables.htm).
func (client VirtualNetworkClient) CreateVirtualCircuit(ctx context.Context, request CreateVirtualCircuitRequest) (response CreateVirtualCircuitResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.createVirtualCircuit, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(CreateVirtualCircuitResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into CreateVirtualCircuitResponse")
	}
	return
}

// createVirtualCircuit implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) createVirtualCircuit(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/virtualCircuits")
	if err != nil {
		return nil, err
	}

	var response CreateVirtualCircuitResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteCpe Deletes the specified CPE object. The CPE must not be connected to a DRG. This is an asynchronous
// operation. The CPE's `lifecycleState` will change to TERMINATING temporarily until the CPE is completely
// removed.
func (client VirtualNetworkClient) DeleteCpe(ctx context.Context, request DeleteCpeRequest) (response DeleteCpeResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteCpe, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteCpeResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteCpeResponse")
	}
	return
}

// deleteCpe implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteCpe(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/cpes/{cpeId}")
	if err != nil {
		return nil, err
	}

	var response DeleteCpeResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteCrossConnect Deletes the specified cross-connect. It must not be mapped to a
// VirtualCircuit.
func (client VirtualNetworkClient) DeleteCrossConnect(ctx context.Context, request DeleteCrossConnectRequest) (response DeleteCrossConnectResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteCrossConnect, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteCrossConnectResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteCrossConnectResponse")
	}
	return
}

// deleteCrossConnect implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteCrossConnect(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/crossConnects/{crossConnectId}")
	if err != nil {
		return nil, err
	}

	var response DeleteCrossConnectResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteCrossConnectGroup Deletes the specified cross-connect group. It must not contain any
// cross-connects, and it cannot be mapped to a
// VirtualCircuit.
func (client VirtualNetworkClient) DeleteCrossConnectGroup(ctx context.Context, request DeleteCrossConnectGroupRequest) (response DeleteCrossConnectGroupResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteCrossConnectGroup, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteCrossConnectGroupResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteCrossConnectGroupResponse")
	}
	return
}

// deleteCrossConnectGroup implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteCrossConnectGroup(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/crossConnectGroups/{crossConnectGroupId}")
	if err != nil {
		return nil, err
	}

	var response DeleteCrossConnectGroupResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteDhcpOptions Deletes the specified set of DHCP options, but only if it's not associated with a subnet. You can't delete a
// VCN's default set of DHCP options.
// This is an asynchronous operation. The state of the set of options will switch to TERMINATING temporarily
// until the set is completely removed.
func (client VirtualNetworkClient) DeleteDhcpOptions(ctx context.Context, request DeleteDhcpOptionsRequest) (response DeleteDhcpOptionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteDhcpOptions, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteDhcpOptionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteDhcpOptionsResponse")
	}
	return
}

// deleteDhcpOptions implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteDhcpOptions(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/dhcps/{dhcpId}")
	if err != nil {
		return nil, err
	}

	var response DeleteDhcpOptionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteDrg Deletes the specified DRG. The DRG must not be attached to a VCN or be connected to your on-premise
// network. Also, there must not be a route table that lists the DRG as a target. This is an asynchronous
// operation. The DRG's `lifecycleState` will change to TERMINATING temporarily until the DRG is completely
// removed.
func (client VirtualNetworkClient) DeleteDrg(ctx context.Context, request DeleteDrgRequest) (response DeleteDrgResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteDrg, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteDrgResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteDrgResponse")
	}
	return
}

// deleteDrg implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteDrg(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/drgs/{drgId}")
	if err != nil {
		return nil, err
	}

	var response DeleteDrgResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteDrgAttachment Detaches a DRG from a VCN by deleting the corresponding `DrgAttachment`. This is an asynchronous
// operation. The attachment's `lifecycleState` will change to DETACHING temporarily until the attachment
// is completely removed.
func (client VirtualNetworkClient) DeleteDrgAttachment(ctx context.Context, request DeleteDrgAttachmentRequest) (response DeleteDrgAttachmentResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteDrgAttachment, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteDrgAttachmentResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteDrgAttachmentResponse")
	}
	return
}

// deleteDrgAttachment implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteDrgAttachment(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/drgAttachments/{drgAttachmentId}")
	if err != nil {
		return nil, err
	}

	var response DeleteDrgAttachmentResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteIPSecConnection Deletes the specified IPSec connection. If your goal is to disable the IPSec VPN between your VCN and
// on-premises network, it's easiest to simply detach the DRG but keep all the IPSec VPN components intact.
// If you were to delete all the components and then later need to create an IPSec VPN again, you would
// need to configure your on-premises router again with the new information returned from
// CreateIPSecConnection.
// This is an asynchronous operation. The connection's `lifecycleState` will change to TERMINATING temporarily
// until the connection is completely removed.
func (client VirtualNetworkClient) DeleteIPSecConnection(ctx context.Context, request DeleteIPSecConnectionRequest) (response DeleteIPSecConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteIPSecConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteIPSecConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteIPSecConnectionResponse")
	}
	return
}

// deleteIPSecConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteIPSecConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/ipsecConnections/{ipscId}")
	if err != nil {
		return nil, err
	}

	var response DeleteIPSecConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteInternetGateway Deletes the specified Internet Gateway. The Internet Gateway does not have to be disabled, but
// there must not be a route table that lists it as a target.
// This is an asynchronous operation. The gateway's `lifecycleState` will change to TERMINATING temporarily
// until the gateway is completely removed.
func (client VirtualNetworkClient) DeleteInternetGateway(ctx context.Context, request DeleteInternetGatewayRequest) (response DeleteInternetGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteInternetGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteInternetGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteInternetGatewayResponse")
	}
	return
}

// deleteInternetGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteInternetGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/internetGateways/{igId}")
	if err != nil {
		return nil, err
	}

	var response DeleteInternetGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteLocalPeeringGateway Deletes the specified local peering gateway (LPG).
// This is an asynchronous operation; the local peering gateway's `lifecycleState` changes to TERMINATING temporarily
// until the local peering gateway is completely removed.
func (client VirtualNetworkClient) DeleteLocalPeeringGateway(ctx context.Context, request DeleteLocalPeeringGatewayRequest) (response DeleteLocalPeeringGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteLocalPeeringGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteLocalPeeringGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteLocalPeeringGatewayResponse")
	}
	return
}

// deleteLocalPeeringGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteLocalPeeringGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/localPeeringGateways/{localPeeringGatewayId}")
	if err != nil {
		return nil, err
	}

	var response DeleteLocalPeeringGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeletePrivateIp Unassigns and deletes the specified private IP. You must
// specify the object's OCID. The private IP address is returned to
// the subnet's pool of available addresses.
// This operation cannot be used with primary private IPs, which are
// automatically unassigned and deleted when the VNIC is terminated.
// **Important:** If a secondary private IP is the
// target of a route rule (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingroutetables.htm#privateip),
// unassigning it from the VNIC causes that route rule to blackhole and the traffic
// will be dropped.
func (client VirtualNetworkClient) DeletePrivateIp(ctx context.Context, request DeletePrivateIpRequest) (response DeletePrivateIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deletePrivateIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeletePrivateIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeletePrivateIpResponse")
	}
	return
}

// deletePrivateIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deletePrivateIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/privateIps/{privateIpId}")
	if err != nil {
		return nil, err
	}

	var response DeletePrivateIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeletePublicIp Unassigns and deletes the specified public IP (either ephemeral or reserved).
// You must specify the object's OCID. The public IP address is returned to the
// Oracle Cloud Infrastructure public IP pool.
// For an assigned reserved public IP, the initial unassignment portion of this operation
// is asynchronous. Poll the public IP's `lifecycleState` to determine
// if the operation succeeded.
// If you want to simply unassign a reserved public IP and return it to your pool
// of reserved public IPs, instead use
// UpdatePublicIp.
func (client VirtualNetworkClient) DeletePublicIp(ctx context.Context, request DeletePublicIpRequest) (response DeletePublicIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deletePublicIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeletePublicIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeletePublicIpResponse")
	}
	return
}

// deletePublicIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deletePublicIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/publicIps/{publicIpId}")
	if err != nil {
		return nil, err
	}

	var response DeletePublicIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteRemotePeeringConnection Deletes the remote peering connection (RPC).
// This is an asynchronous operation; the RPC's `lifecycleState` changes to TERMINATING temporarily
// until the RPC is completely removed.
func (client VirtualNetworkClient) DeleteRemotePeeringConnection(ctx context.Context, request DeleteRemotePeeringConnectionRequest) (response DeleteRemotePeeringConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteRemotePeeringConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteRemotePeeringConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteRemotePeeringConnectionResponse")
	}
	return
}

// deleteRemotePeeringConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteRemotePeeringConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/remotePeeringConnections/{remotePeeringConnectionId}")
	if err != nil {
		return nil, err
	}

	var response DeleteRemotePeeringConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteRouteTable Deletes the specified route table, but only if it's not associated with a subnet. You can't delete a
// VCN's default route table.
// This is an asynchronous operation. The route table's `lifecycleState` will change to TERMINATING temporarily
// until the route table is completely removed.
func (client VirtualNetworkClient) DeleteRouteTable(ctx context.Context, request DeleteRouteTableRequest) (response DeleteRouteTableResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteRouteTable, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteRouteTableResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteRouteTableResponse")
	}
	return
}

// deleteRouteTable implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteRouteTable(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/routeTables/{rtId}")
	if err != nil {
		return nil, err
	}

	var response DeleteRouteTableResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteSecurityList Deletes the specified security list, but only if it's not associated with a subnet. You can't delete
// a VCN's default security list.
// This is an asynchronous operation. The security list's `lifecycleState` will change to TERMINATING temporarily
// until the security list is completely removed.
func (client VirtualNetworkClient) DeleteSecurityList(ctx context.Context, request DeleteSecurityListRequest) (response DeleteSecurityListResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteSecurityList, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteSecurityListResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteSecurityListResponse")
	}
	return
}

// deleteSecurityList implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteSecurityList(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/securityLists/{securityListId}")
	if err != nil {
		return nil, err
	}

	var response DeleteSecurityListResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteServiceGateway Deletes the specified service gateway. There must not be a route table that lists the service
// gateway as a target.
func (client VirtualNetworkClient) DeleteServiceGateway(ctx context.Context, request DeleteServiceGatewayRequest) (response DeleteServiceGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteServiceGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteServiceGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteServiceGatewayResponse")
	}
	return
}

// deleteServiceGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteServiceGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/serviceGateways/{serviceGatewayId}")
	if err != nil {
		return nil, err
	}

	var response DeleteServiceGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteSubnet Deletes the specified subnet, but only if there are no instances in the subnet. This is an asynchronous
// operation. The subnet's `lifecycleState` will change to TERMINATING temporarily. If there are any
// instances in the subnet, the state will instead change back to AVAILABLE.
func (client VirtualNetworkClient) DeleteSubnet(ctx context.Context, request DeleteSubnetRequest) (response DeleteSubnetResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteSubnet, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteSubnetResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteSubnetResponse")
	}
	return
}

// deleteSubnet implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteSubnet(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/subnets/{subnetId}")
	if err != nil {
		return nil, err
	}

	var response DeleteSubnetResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteVcn Deletes the specified VCN. The VCN must be empty and have no attached gateways. This is an asynchronous
// operation. The VCN's `lifecycleState` will change to TERMINATING temporarily until the VCN is completely
// removed.
func (client VirtualNetworkClient) DeleteVcn(ctx context.Context, request DeleteVcnRequest) (response DeleteVcnResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteVcn, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteVcnResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteVcnResponse")
	}
	return
}

// deleteVcn implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteVcn(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/vcns/{vcnId}")
	if err != nil {
		return nil, err
	}

	var response DeleteVcnResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DeleteVirtualCircuit Deletes the specified virtual circuit.
// **Important:** If you're using FastConnect via a provider,
// make sure to also terminate the connection with
// the provider, or else the provider may continue to bill you.
func (client VirtualNetworkClient) DeleteVirtualCircuit(ctx context.Context, request DeleteVirtualCircuitRequest) (response DeleteVirtualCircuitResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.deleteVirtualCircuit, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DeleteVirtualCircuitResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DeleteVirtualCircuitResponse")
	}
	return
}

// deleteVirtualCircuit implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) deleteVirtualCircuit(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodDelete, "/virtualCircuits/{virtualCircuitId}")
	if err != nil {
		return nil, err
	}

	var response DeleteVirtualCircuitResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// DetachServiceId Disables the specified service on the specified gateway. In other words, stops the service
// gateway from sending traffic to the specified service. You do not need to remove any route
// rules that specify this service's `cidrBlock` as the destination CIDR. However, consider
// removing the rules if your intent is to permanently disable use of the service through this
// service gateway.
// **Note:** The `DetachServiceId` operation is an easy way to disable an individual service on
// the service gateway. Compare it with
// UpdateServiceGateway, which also
// lets you disable an individual service. However, with `UpdateServiceGateway`, you must specify
// the *entire* list of services you want enabled on the service gateway. `UpdateServiceGateway`
// also lets you block all traffic through the service gateway without having to disable each of
// the individual services.
func (client VirtualNetworkClient) DetachServiceId(ctx context.Context, request DetachServiceIdRequest) (response DetachServiceIdResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.detachServiceId, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(DetachServiceIdResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into DetachServiceIdResponse")
	}
	return
}

// detachServiceId implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) detachServiceId(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/serviceGateways/{serviceGatewayId}/actions/detachService")
	if err != nil {
		return nil, err
	}

	var response DetachServiceIdResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetCpe Gets the specified CPE's information.
func (client VirtualNetworkClient) GetCpe(ctx context.Context, request GetCpeRequest) (response GetCpeResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getCpe, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetCpeResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetCpeResponse")
	}
	return
}

// getCpe implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getCpe(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/cpes/{cpeId}")
	if err != nil {
		return nil, err
	}

	var response GetCpeResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetCrossConnect Gets the specified cross-connect's information.
func (client VirtualNetworkClient) GetCrossConnect(ctx context.Context, request GetCrossConnectRequest) (response GetCrossConnectResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getCrossConnect, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetCrossConnectResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetCrossConnectResponse")
	}
	return
}

// getCrossConnect implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getCrossConnect(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnects/{crossConnectId}")
	if err != nil {
		return nil, err
	}

	var response GetCrossConnectResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetCrossConnectGroup Gets the specified cross-connect group's information.
func (client VirtualNetworkClient) GetCrossConnectGroup(ctx context.Context, request GetCrossConnectGroupRequest) (response GetCrossConnectGroupResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getCrossConnectGroup, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetCrossConnectGroupResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetCrossConnectGroupResponse")
	}
	return
}

// getCrossConnectGroup implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getCrossConnectGroup(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnectGroups/{crossConnectGroupId}")
	if err != nil {
		return nil, err
	}

	var response GetCrossConnectGroupResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetCrossConnectLetterOfAuthority Gets the Letter of Authority for the specified cross-connect.
func (client VirtualNetworkClient) GetCrossConnectLetterOfAuthority(ctx context.Context, request GetCrossConnectLetterOfAuthorityRequest) (response GetCrossConnectLetterOfAuthorityResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getCrossConnectLetterOfAuthority, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetCrossConnectLetterOfAuthorityResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetCrossConnectLetterOfAuthorityResponse")
	}
	return
}

// getCrossConnectLetterOfAuthority implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getCrossConnectLetterOfAuthority(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnects/{crossConnectId}/letterOfAuthority")
	if err != nil {
		return nil, err
	}

	var response GetCrossConnectLetterOfAuthorityResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetCrossConnectStatus Gets the status of the specified cross-connect.
func (client VirtualNetworkClient) GetCrossConnectStatus(ctx context.Context, request GetCrossConnectStatusRequest) (response GetCrossConnectStatusResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getCrossConnectStatus, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetCrossConnectStatusResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetCrossConnectStatusResponse")
	}
	return
}

// getCrossConnectStatus implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getCrossConnectStatus(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnects/{crossConnectId}/status")
	if err != nil {
		return nil, err
	}

	var response GetCrossConnectStatusResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetDhcpOptions Gets the specified set of DHCP options.
func (client VirtualNetworkClient) GetDhcpOptions(ctx context.Context, request GetDhcpOptionsRequest) (response GetDhcpOptionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getDhcpOptions, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetDhcpOptionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetDhcpOptionsResponse")
	}
	return
}

// getDhcpOptions implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getDhcpOptions(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/dhcps/{dhcpId}")
	if err != nil {
		return nil, err
	}

	var response GetDhcpOptionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetDrg Gets the specified DRG's information.
func (client VirtualNetworkClient) GetDrg(ctx context.Context, request GetDrgRequest) (response GetDrgResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getDrg, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetDrgResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetDrgResponse")
	}
	return
}

// getDrg implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getDrg(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/drgs/{drgId}")
	if err != nil {
		return nil, err
	}

	var response GetDrgResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetDrgAttachment Gets the information for the specified `DrgAttachment`.
func (client VirtualNetworkClient) GetDrgAttachment(ctx context.Context, request GetDrgAttachmentRequest) (response GetDrgAttachmentResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getDrgAttachment, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetDrgAttachmentResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetDrgAttachmentResponse")
	}
	return
}

// getDrgAttachment implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getDrgAttachment(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/drgAttachments/{drgAttachmentId}")
	if err != nil {
		return nil, err
	}

	var response GetDrgAttachmentResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetFastConnectProviderService Gets the specified provider service.
// For more information, see FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
func (client VirtualNetworkClient) GetFastConnectProviderService(ctx context.Context, request GetFastConnectProviderServiceRequest) (response GetFastConnectProviderServiceResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getFastConnectProviderService, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetFastConnectProviderServiceResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetFastConnectProviderServiceResponse")
	}
	return
}

// getFastConnectProviderService implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getFastConnectProviderService(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/fastConnectProviderServices/{providerServiceId}")
	if err != nil {
		return nil, err
	}

	var response GetFastConnectProviderServiceResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetIPSecConnection Gets the specified IPSec connection's basic information, including the static routes for the
// on-premises router. If you want the status of the connection (whether it's up or down), use
// GetIPSecConnectionDeviceStatus.
func (client VirtualNetworkClient) GetIPSecConnection(ctx context.Context, request GetIPSecConnectionRequest) (response GetIPSecConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getIPSecConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetIPSecConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetIPSecConnectionResponse")
	}
	return
}

// getIPSecConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getIPSecConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/ipsecConnections/{ipscId}")
	if err != nil {
		return nil, err
	}

	var response GetIPSecConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetIPSecConnectionDeviceConfig Gets the configuration information for the specified IPSec connection. For each tunnel, the
// response includes the IP address of Oracle's VPN headend and the shared secret.
func (client VirtualNetworkClient) GetIPSecConnectionDeviceConfig(ctx context.Context, request GetIPSecConnectionDeviceConfigRequest) (response GetIPSecConnectionDeviceConfigResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getIPSecConnectionDeviceConfig, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetIPSecConnectionDeviceConfigResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetIPSecConnectionDeviceConfigResponse")
	}
	return
}

// getIPSecConnectionDeviceConfig implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getIPSecConnectionDeviceConfig(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/ipsecConnections/{ipscId}/deviceConfig")
	if err != nil {
		return nil, err
	}

	var response GetIPSecConnectionDeviceConfigResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetIPSecConnectionDeviceStatus Gets the status of the specified IPSec connection (whether it's up or down).
func (client VirtualNetworkClient) GetIPSecConnectionDeviceStatus(ctx context.Context, request GetIPSecConnectionDeviceStatusRequest) (response GetIPSecConnectionDeviceStatusResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getIPSecConnectionDeviceStatus, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetIPSecConnectionDeviceStatusResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetIPSecConnectionDeviceStatusResponse")
	}
	return
}

// getIPSecConnectionDeviceStatus implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getIPSecConnectionDeviceStatus(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/ipsecConnections/{ipscId}/deviceStatus")
	if err != nil {
		return nil, err
	}

	var response GetIPSecConnectionDeviceStatusResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetInternetGateway Gets the specified Internet Gateway's information.
func (client VirtualNetworkClient) GetInternetGateway(ctx context.Context, request GetInternetGatewayRequest) (response GetInternetGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getInternetGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetInternetGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetInternetGatewayResponse")
	}
	return
}

// getInternetGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getInternetGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/internetGateways/{igId}")
	if err != nil {
		return nil, err
	}

	var response GetInternetGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetLocalPeeringGateway Gets the specified local peering gateway's information.
func (client VirtualNetworkClient) GetLocalPeeringGateway(ctx context.Context, request GetLocalPeeringGatewayRequest) (response GetLocalPeeringGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getLocalPeeringGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetLocalPeeringGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetLocalPeeringGatewayResponse")
	}
	return
}

// getLocalPeeringGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getLocalPeeringGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/localPeeringGateways/{localPeeringGatewayId}")
	if err != nil {
		return nil, err
	}

	var response GetLocalPeeringGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetPrivateIp Gets the specified private IP. You must specify the object's OCID.
// Alternatively, you can get the object by using
// ListPrivateIps
// with the private IP address (for example, 10.0.3.3) and subnet OCID.
func (client VirtualNetworkClient) GetPrivateIp(ctx context.Context, request GetPrivateIpRequest) (response GetPrivateIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getPrivateIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetPrivateIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetPrivateIpResponse")
	}
	return
}

// getPrivateIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getPrivateIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/privateIps/{privateIpId}")
	if err != nil {
		return nil, err
	}

	var response GetPrivateIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetPublicIp Gets the specified public IP. You must specify the object's OCID.
// Alternatively, you can get the object by using GetPublicIpByIpAddress
// with the public IP address (for example, 129.146.2.1).
// Or you can use GetPublicIpByPrivateIpId
// with the OCID of the private IP that the public IP is assigned to.
// **Note:** If you're fetching a reserved public IP that is in the process of being
// moved to a different private IP, the service returns the public IP object with
// `lifecycleState` = ASSIGNING and `privateIpId` = OCID of the target private IP.
func (client VirtualNetworkClient) GetPublicIp(ctx context.Context, request GetPublicIpRequest) (response GetPublicIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getPublicIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetPublicIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetPublicIpResponse")
	}
	return
}

// getPublicIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getPublicIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/publicIps/{publicIpId}")
	if err != nil {
		return nil, err
	}

	var response GetPublicIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetPublicIpByIpAddress Gets the public IP based on the public IP address (for example, 129.146.2.1).
// **Note:** If you're fetching a reserved public IP that is in the process of being
// moved to a different private IP, the service returns the public IP object with
// `lifecycleState` = ASSIGNING and `privateIpId` = OCID of the target private IP.
func (client VirtualNetworkClient) GetPublicIpByIpAddress(ctx context.Context, request GetPublicIpByIpAddressRequest) (response GetPublicIpByIpAddressResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getPublicIpByIpAddress, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetPublicIpByIpAddressResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetPublicIpByIpAddressResponse")
	}
	return
}

// getPublicIpByIpAddress implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getPublicIpByIpAddress(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/publicIps/actions/getByIpAddress")
	if err != nil {
		return nil, err
	}

	var response GetPublicIpByIpAddressResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetPublicIpByPrivateIpId Gets the public IP assigned to the specified private IP. You must specify the OCID
// of the private IP. If no public IP is assigned, a 404 is returned.
// **Note:** If you're fetching a reserved public IP that is in the process of being
// moved to a different private IP, and you provide the OCID of the original private
// IP, this operation returns a 404. If you instead provide the OCID of the target
// private IP, or if you instead call
// GetPublicIp or
// GetPublicIpByIpAddress, the
// service returns the public IP object with `lifecycleState` = ASSIGNING and `privateIpId` = OCID
// of the target private IP.
func (client VirtualNetworkClient) GetPublicIpByPrivateIpId(ctx context.Context, request GetPublicIpByPrivateIpIdRequest) (response GetPublicIpByPrivateIpIdResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getPublicIpByPrivateIpId, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetPublicIpByPrivateIpIdResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetPublicIpByPrivateIpIdResponse")
	}
	return
}

// getPublicIpByPrivateIpId implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getPublicIpByPrivateIpId(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPost, "/publicIps/actions/getByPrivateIpId")
	if err != nil {
		return nil, err
	}

	var response GetPublicIpByPrivateIpIdResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetRemotePeeringConnection Get the specified remote peering connection's information.
func (client VirtualNetworkClient) GetRemotePeeringConnection(ctx context.Context, request GetRemotePeeringConnectionRequest) (response GetRemotePeeringConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getRemotePeeringConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetRemotePeeringConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetRemotePeeringConnectionResponse")
	}
	return
}

// getRemotePeeringConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getRemotePeeringConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/remotePeeringConnections/{remotePeeringConnectionId}")
	if err != nil {
		return nil, err
	}

	var response GetRemotePeeringConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetRouteTable Gets the specified route table's information.
func (client VirtualNetworkClient) GetRouteTable(ctx context.Context, request GetRouteTableRequest) (response GetRouteTableResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getRouteTable, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetRouteTableResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetRouteTableResponse")
	}
	return
}

// getRouteTable implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getRouteTable(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/routeTables/{rtId}")
	if err != nil {
		return nil, err
	}

	var response GetRouteTableResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetSecurityList Gets the specified security list's information.
func (client VirtualNetworkClient) GetSecurityList(ctx context.Context, request GetSecurityListRequest) (response GetSecurityListResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getSecurityList, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetSecurityListResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetSecurityListResponse")
	}
	return
}

// getSecurityList implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getSecurityList(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/securityLists/{securityListId}")
	if err != nil {
		return nil, err
	}

	var response GetSecurityListResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetService Gets the specified service's information.
func (client VirtualNetworkClient) GetService(ctx context.Context, request GetServiceRequest) (response GetServiceResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getService, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetServiceResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetServiceResponse")
	}
	return
}

// getService implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getService(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/services/{serviceId}")
	if err != nil {
		return nil, err
	}

	var response GetServiceResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetServiceGateway Gets the specified service gateway's information.
func (client VirtualNetworkClient) GetServiceGateway(ctx context.Context, request GetServiceGatewayRequest) (response GetServiceGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getServiceGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetServiceGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetServiceGatewayResponse")
	}
	return
}

// getServiceGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getServiceGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/serviceGateways/{serviceGatewayId}")
	if err != nil {
		return nil, err
	}

	var response GetServiceGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetSubnet Gets the specified subnet's information.
func (client VirtualNetworkClient) GetSubnet(ctx context.Context, request GetSubnetRequest) (response GetSubnetResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getSubnet, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetSubnetResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetSubnetResponse")
	}
	return
}

// getSubnet implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getSubnet(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/subnets/{subnetId}")
	if err != nil {
		return nil, err
	}

	var response GetSubnetResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetVcn Gets the specified VCN's information.
func (client VirtualNetworkClient) GetVcn(ctx context.Context, request GetVcnRequest) (response GetVcnResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getVcn, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetVcnResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetVcnResponse")
	}
	return
}

// getVcn implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getVcn(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/vcns/{vcnId}")
	if err != nil {
		return nil, err
	}

	var response GetVcnResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetVirtualCircuit Gets the specified virtual circuit's information.
func (client VirtualNetworkClient) GetVirtualCircuit(ctx context.Context, request GetVirtualCircuitRequest) (response GetVirtualCircuitResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getVirtualCircuit, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetVirtualCircuitResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetVirtualCircuitResponse")
	}
	return
}

// getVirtualCircuit implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getVirtualCircuit(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/virtualCircuits/{virtualCircuitId}")
	if err != nil {
		return nil, err
	}

	var response GetVirtualCircuitResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// GetVnic Gets the information for the specified virtual network interface card (VNIC).
// You can get the VNIC OCID from the
// ListVnicAttachments
// operation.
func (client VirtualNetworkClient) GetVnic(ctx context.Context, request GetVnicRequest) (response GetVnicResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.getVnic, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(GetVnicResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into GetVnicResponse")
	}
	return
}

// getVnic implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) getVnic(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/vnics/{vnicId}")
	if err != nil {
		return nil, err
	}

	var response GetVnicResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListAllowedPeerRegionsForRemotePeering Lists the regions that support remote VCN peering (which is peering across regions).
// For more information, see VCN Peering (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/VCNpeering.htm).
func (client VirtualNetworkClient) ListAllowedPeerRegionsForRemotePeering(ctx context.Context, request ListAllowedPeerRegionsForRemotePeeringRequest) (response ListAllowedPeerRegionsForRemotePeeringResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listAllowedPeerRegionsForRemotePeering, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListAllowedPeerRegionsForRemotePeeringResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListAllowedPeerRegionsForRemotePeeringResponse")
	}
	return
}

// listAllowedPeerRegionsForRemotePeering implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listAllowedPeerRegionsForRemotePeering(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/allowedPeerRegionsForRemotePeering")
	if err != nil {
		return nil, err
	}

	var response ListAllowedPeerRegionsForRemotePeeringResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListCpes Lists the Customer-Premises Equipment objects (CPEs) in the specified compartment.
func (client VirtualNetworkClient) ListCpes(ctx context.Context, request ListCpesRequest) (response ListCpesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listCpes, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListCpesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListCpesResponse")
	}
	return
}

// listCpes implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listCpes(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/cpes")
	if err != nil {
		return nil, err
	}

	var response ListCpesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListCrossConnectGroups Lists the cross-connect groups in the specified compartment.
func (client VirtualNetworkClient) ListCrossConnectGroups(ctx context.Context, request ListCrossConnectGroupsRequest) (response ListCrossConnectGroupsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listCrossConnectGroups, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListCrossConnectGroupsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListCrossConnectGroupsResponse")
	}
	return
}

// listCrossConnectGroups implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listCrossConnectGroups(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnectGroups")
	if err != nil {
		return nil, err
	}

	var response ListCrossConnectGroupsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListCrossConnectLocations Lists the available FastConnect locations for cross-connect installation. You need
// this information so you can specify your desired location when you create a cross-connect.
func (client VirtualNetworkClient) ListCrossConnectLocations(ctx context.Context, request ListCrossConnectLocationsRequest) (response ListCrossConnectLocationsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listCrossConnectLocations, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListCrossConnectLocationsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListCrossConnectLocationsResponse")
	}
	return
}

// listCrossConnectLocations implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listCrossConnectLocations(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnectLocations")
	if err != nil {
		return nil, err
	}

	var response ListCrossConnectLocationsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListCrossConnects Lists the cross-connects in the specified compartment. You can filter the list
// by specifying the OCID of a cross-connect group.
func (client VirtualNetworkClient) ListCrossConnects(ctx context.Context, request ListCrossConnectsRequest) (response ListCrossConnectsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listCrossConnects, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListCrossConnectsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListCrossConnectsResponse")
	}
	return
}

// listCrossConnects implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listCrossConnects(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnects")
	if err != nil {
		return nil, err
	}

	var response ListCrossConnectsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListCrossconnectPortSpeedShapes Lists the available port speeds for cross-connects. You need this information
// so you can specify your desired port speed (that is, shape) when you create a
// cross-connect.
func (client VirtualNetworkClient) ListCrossconnectPortSpeedShapes(ctx context.Context, request ListCrossconnectPortSpeedShapesRequest) (response ListCrossconnectPortSpeedShapesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listCrossconnectPortSpeedShapes, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListCrossconnectPortSpeedShapesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListCrossconnectPortSpeedShapesResponse")
	}
	return
}

// listCrossconnectPortSpeedShapes implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listCrossconnectPortSpeedShapes(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/crossConnectPortSpeedShapes")
	if err != nil {
		return nil, err
	}

	var response ListCrossconnectPortSpeedShapesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListDhcpOptions Lists the sets of DHCP options in the specified VCN and specified compartment.
// The response includes the default set of options that automatically comes with each VCN,
// plus any other sets you've created.
func (client VirtualNetworkClient) ListDhcpOptions(ctx context.Context, request ListDhcpOptionsRequest) (response ListDhcpOptionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listDhcpOptions, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListDhcpOptionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListDhcpOptionsResponse")
	}
	return
}

// listDhcpOptions implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listDhcpOptions(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/dhcps")
	if err != nil {
		return nil, err
	}

	var response ListDhcpOptionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListDrgAttachments Lists the `DrgAttachment` objects for the specified compartment. You can filter the
// results by VCN or DRG.
func (client VirtualNetworkClient) ListDrgAttachments(ctx context.Context, request ListDrgAttachmentsRequest) (response ListDrgAttachmentsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listDrgAttachments, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListDrgAttachmentsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListDrgAttachmentsResponse")
	}
	return
}

// listDrgAttachments implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listDrgAttachments(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/drgAttachments")
	if err != nil {
		return nil, err
	}

	var response ListDrgAttachmentsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListDrgs Lists the DRGs in the specified compartment.
func (client VirtualNetworkClient) ListDrgs(ctx context.Context, request ListDrgsRequest) (response ListDrgsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listDrgs, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListDrgsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListDrgsResponse")
	}
	return
}

// listDrgs implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listDrgs(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/drgs")
	if err != nil {
		return nil, err
	}

	var response ListDrgsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListFastConnectProviderServices Lists the service offerings from supported providers. You need this
// information so you can specify your desired provider and service
// offering when you create a virtual circuit.
// For the compartment ID, provide the OCID of your tenancy (the root compartment).
// For more information, see FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
func (client VirtualNetworkClient) ListFastConnectProviderServices(ctx context.Context, request ListFastConnectProviderServicesRequest) (response ListFastConnectProviderServicesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listFastConnectProviderServices, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListFastConnectProviderServicesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListFastConnectProviderServicesResponse")
	}
	return
}

// listFastConnectProviderServices implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listFastConnectProviderServices(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/fastConnectProviderServices")
	if err != nil {
		return nil, err
	}

	var response ListFastConnectProviderServicesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListFastConnectProviderVirtualCircuitBandwidthShapes Gets the list of available virtual circuit bandwidth levels for a provider.
// You need this information so you can specify your desired bandwidth level (shape) when you create a virtual circuit.
// For more information about virtual circuits, see FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
func (client VirtualNetworkClient) ListFastConnectProviderVirtualCircuitBandwidthShapes(ctx context.Context, request ListFastConnectProviderVirtualCircuitBandwidthShapesRequest) (response ListFastConnectProviderVirtualCircuitBandwidthShapesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listFastConnectProviderVirtualCircuitBandwidthShapes, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListFastConnectProviderVirtualCircuitBandwidthShapesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListFastConnectProviderVirtualCircuitBandwidthShapesResponse")
	}
	return
}

// listFastConnectProviderVirtualCircuitBandwidthShapes implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listFastConnectProviderVirtualCircuitBandwidthShapes(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/fastConnectProviderServices/{providerServiceId}/virtualCircuitBandwidthShapes")
	if err != nil {
		return nil, err
	}

	var response ListFastConnectProviderVirtualCircuitBandwidthShapesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListIPSecConnections Lists the IPSec connections for the specified compartment. You can filter the
// results by DRG or CPE.
func (client VirtualNetworkClient) ListIPSecConnections(ctx context.Context, request ListIPSecConnectionsRequest) (response ListIPSecConnectionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listIPSecConnections, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListIPSecConnectionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListIPSecConnectionsResponse")
	}
	return
}

// listIPSecConnections implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listIPSecConnections(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/ipsecConnections")
	if err != nil {
		return nil, err
	}

	var response ListIPSecConnectionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListInternetGateways Lists the Internet Gateways in the specified VCN and the specified compartment.
func (client VirtualNetworkClient) ListInternetGateways(ctx context.Context, request ListInternetGatewaysRequest) (response ListInternetGatewaysResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listInternetGateways, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListInternetGatewaysResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListInternetGatewaysResponse")
	}
	return
}

// listInternetGateways implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listInternetGateways(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/internetGateways")
	if err != nil {
		return nil, err
	}

	var response ListInternetGatewaysResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListLocalPeeringGateways Lists the local peering gateways (LPGs) for the specified VCN and compartment
// (the LPG's compartment).
func (client VirtualNetworkClient) ListLocalPeeringGateways(ctx context.Context, request ListLocalPeeringGatewaysRequest) (response ListLocalPeeringGatewaysResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listLocalPeeringGateways, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListLocalPeeringGatewaysResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListLocalPeeringGatewaysResponse")
	}
	return
}

// listLocalPeeringGateways implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listLocalPeeringGateways(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/localPeeringGateways")
	if err != nil {
		return nil, err
	}

	var response ListLocalPeeringGatewaysResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListPrivateIps Lists the PrivateIp objects based
// on one of these filters:
//   - Subnet OCID.
//   - VNIC OCID.
//   - Both private IP address and subnet OCID: This lets
//   you get a `privateIP` object based on its private IP
//   address (for example, 10.0.3.3) and not its OCID. For comparison,
//   GetPrivateIp
//   requires the OCID.
// If you're listing all the private IPs associated with a given subnet
// or VNIC, the response includes both primary and secondary private IPs.
func (client VirtualNetworkClient) ListPrivateIps(ctx context.Context, request ListPrivateIpsRequest) (response ListPrivateIpsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listPrivateIps, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListPrivateIpsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListPrivateIpsResponse")
	}
	return
}

// listPrivateIps implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listPrivateIps(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/privateIps")
	if err != nil {
		return nil, err
	}

	var response ListPrivateIpsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListPublicIps Lists either the ephemeral or reserved PublicIp objects
// in the specified compartment.
// To list your reserved public IPs, set `scope` = `REGION`, and leave the
// `availabilityDomain` parameter empty.
// To list your ephemeral public IPs, set `scope` = `AVAILABILITY_DOMAIN`, and set the
// `availabilityDomain` parameter to the desired Availability Domain. An ephemeral public IP
// is always in the same Availability Domain and compartment as the private IP it's assigned to.
func (client VirtualNetworkClient) ListPublicIps(ctx context.Context, request ListPublicIpsRequest) (response ListPublicIpsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listPublicIps, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListPublicIpsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListPublicIpsResponse")
	}
	return
}

// listPublicIps implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listPublicIps(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/publicIps")
	if err != nil {
		return nil, err
	}

	var response ListPublicIpsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListRemotePeeringConnections Lists the remote peering connections (RPCs) for the specified DRG and compartment
// (the RPC's compartment).
func (client VirtualNetworkClient) ListRemotePeeringConnections(ctx context.Context, request ListRemotePeeringConnectionsRequest) (response ListRemotePeeringConnectionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listRemotePeeringConnections, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListRemotePeeringConnectionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListRemotePeeringConnectionsResponse")
	}
	return
}

// listRemotePeeringConnections implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listRemotePeeringConnections(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/remotePeeringConnections")
	if err != nil {
		return nil, err
	}

	var response ListRemotePeeringConnectionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListRouteTables Lists the route tables in the specified VCN and specified compartment. The response
// includes the default route table that automatically comes with each VCN, plus any route tables
// you've created.
func (client VirtualNetworkClient) ListRouteTables(ctx context.Context, request ListRouteTablesRequest) (response ListRouteTablesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listRouteTables, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListRouteTablesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListRouteTablesResponse")
	}
	return
}

// listRouteTables implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listRouteTables(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/routeTables")
	if err != nil {
		return nil, err
	}

	var response ListRouteTablesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListSecurityLists Lists the security lists in the specified VCN and compartment.
func (client VirtualNetworkClient) ListSecurityLists(ctx context.Context, request ListSecurityListsRequest) (response ListSecurityListsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listSecurityLists, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListSecurityListsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListSecurityListsResponse")
	}
	return
}

// listSecurityLists implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listSecurityLists(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/securityLists")
	if err != nil {
		return nil, err
	}

	var response ListSecurityListsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListServiceGateways Lists the service gateways in the specified compartment. You may optionally specify a VCN OCID
// to filter the results by VCN.
func (client VirtualNetworkClient) ListServiceGateways(ctx context.Context, request ListServiceGatewaysRequest) (response ListServiceGatewaysResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listServiceGateways, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListServiceGatewaysResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListServiceGatewaysResponse")
	}
	return
}

// listServiceGateways implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listServiceGateways(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/serviceGateways")
	if err != nil {
		return nil, err
	}

	var response ListServiceGatewaysResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListServices Lists the available services that you can access through a service gateway in this region.
func (client VirtualNetworkClient) ListServices(ctx context.Context, request ListServicesRequest) (response ListServicesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listServices, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListServicesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListServicesResponse")
	}
	return
}

// listServices implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listServices(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/services")
	if err != nil {
		return nil, err
	}

	var response ListServicesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListSubnets Lists the subnets in the specified VCN and the specified compartment.
func (client VirtualNetworkClient) ListSubnets(ctx context.Context, request ListSubnetsRequest) (response ListSubnetsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listSubnets, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListSubnetsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListSubnetsResponse")
	}
	return
}

// listSubnets implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listSubnets(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/subnets")
	if err != nil {
		return nil, err
	}

	var response ListSubnetsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListVcns Lists the Virtual Cloud Networks (VCNs) in the specified compartment.
func (client VirtualNetworkClient) ListVcns(ctx context.Context, request ListVcnsRequest) (response ListVcnsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listVcns, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListVcnsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListVcnsResponse")
	}
	return
}

// listVcns implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listVcns(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/vcns")
	if err != nil {
		return nil, err
	}

	var response ListVcnsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListVirtualCircuitBandwidthShapes The deprecated operation lists available bandwidth levels for virtual circuits. For the compartment ID, provide the OCID of your tenancy (the root compartment).
func (client VirtualNetworkClient) ListVirtualCircuitBandwidthShapes(ctx context.Context, request ListVirtualCircuitBandwidthShapesRequest) (response ListVirtualCircuitBandwidthShapesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listVirtualCircuitBandwidthShapes, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListVirtualCircuitBandwidthShapesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListVirtualCircuitBandwidthShapesResponse")
	}
	return
}

// listVirtualCircuitBandwidthShapes implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listVirtualCircuitBandwidthShapes(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/virtualCircuitBandwidthShapes")
	if err != nil {
		return nil, err
	}

	var response ListVirtualCircuitBandwidthShapesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListVirtualCircuitPublicPrefixes Lists the public IP prefixes and their details for the specified
// public virtual circuit.
func (client VirtualNetworkClient) ListVirtualCircuitPublicPrefixes(ctx context.Context, request ListVirtualCircuitPublicPrefixesRequest) (response ListVirtualCircuitPublicPrefixesResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listVirtualCircuitPublicPrefixes, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListVirtualCircuitPublicPrefixesResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListVirtualCircuitPublicPrefixesResponse")
	}
	return
}

// listVirtualCircuitPublicPrefixes implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listVirtualCircuitPublicPrefixes(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/virtualCircuits/{virtualCircuitId}/publicPrefixes")
	if err != nil {
		return nil, err
	}

	var response ListVirtualCircuitPublicPrefixesResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// ListVirtualCircuits Lists the virtual circuits in the specified compartment.
func (client VirtualNetworkClient) ListVirtualCircuits(ctx context.Context, request ListVirtualCircuitsRequest) (response ListVirtualCircuitsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.listVirtualCircuits, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(ListVirtualCircuitsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into ListVirtualCircuitsResponse")
	}
	return
}

// listVirtualCircuits implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) listVirtualCircuits(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodGet, "/virtualCircuits")
	if err != nil {
		return nil, err
	}

	var response ListVirtualCircuitsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateCpe Updates the specified CPE's display name or tags.
// Avoid entering confidential information.
func (client VirtualNetworkClient) UpdateCpe(ctx context.Context, request UpdateCpeRequest) (response UpdateCpeResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateCpe, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateCpeResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateCpeResponse")
	}
	return
}

// updateCpe implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateCpe(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/cpes/{cpeId}")
	if err != nil {
		return nil, err
	}

	var response UpdateCpeResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateCrossConnect Updates the specified cross-connect.
func (client VirtualNetworkClient) UpdateCrossConnect(ctx context.Context, request UpdateCrossConnectRequest) (response UpdateCrossConnectResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateCrossConnect, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateCrossConnectResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateCrossConnectResponse")
	}
	return
}

// updateCrossConnect implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateCrossConnect(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/crossConnects/{crossConnectId}")
	if err != nil {
		return nil, err
	}

	var response UpdateCrossConnectResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateCrossConnectGroup Updates the specified cross-connect group's display name.
// Avoid entering confidential information.
func (client VirtualNetworkClient) UpdateCrossConnectGroup(ctx context.Context, request UpdateCrossConnectGroupRequest) (response UpdateCrossConnectGroupResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateCrossConnectGroup, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateCrossConnectGroupResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateCrossConnectGroupResponse")
	}
	return
}

// updateCrossConnectGroup implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateCrossConnectGroup(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/crossConnectGroups/{crossConnectGroupId}")
	if err != nil {
		return nil, err
	}

	var response UpdateCrossConnectGroupResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateDhcpOptions Updates the specified set of DHCP options. You can update the display name or the options
// themselves. Avoid entering confidential information.
// Note that the `options` object you provide replaces the entire existing set of options.
func (client VirtualNetworkClient) UpdateDhcpOptions(ctx context.Context, request UpdateDhcpOptionsRequest) (response UpdateDhcpOptionsResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateDhcpOptions, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateDhcpOptionsResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateDhcpOptionsResponse")
	}
	return
}

// updateDhcpOptions implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateDhcpOptions(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/dhcps/{dhcpId}")
	if err != nil {
		return nil, err
	}

	var response UpdateDhcpOptionsResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateDrg Updates the specified DRG's display name or tags. Avoid entering confidential information.
func (client VirtualNetworkClient) UpdateDrg(ctx context.Context, request UpdateDrgRequest) (response UpdateDrgResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateDrg, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateDrgResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateDrgResponse")
	}
	return
}

// updateDrg implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateDrg(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/drgs/{drgId}")
	if err != nil {
		return nil, err
	}

	var response UpdateDrgResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateDrgAttachment Updates the display name for the specified `DrgAttachment`.
// Avoid entering confidential information.
func (client VirtualNetworkClient) UpdateDrgAttachment(ctx context.Context, request UpdateDrgAttachmentRequest) (response UpdateDrgAttachmentResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateDrgAttachment, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateDrgAttachmentResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateDrgAttachmentResponse")
	}
	return
}

// updateDrgAttachment implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateDrgAttachment(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/drgAttachments/{drgAttachmentId}")
	if err != nil {
		return nil, err
	}

	var response UpdateDrgAttachmentResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateIPSecConnection Updates the display name or tags for the specified IPSec connection.
// Avoid entering confidential information.
func (client VirtualNetworkClient) UpdateIPSecConnection(ctx context.Context, request UpdateIPSecConnectionRequest) (response UpdateIPSecConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateIPSecConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateIPSecConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateIPSecConnectionResponse")
	}
	return
}

// updateIPSecConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateIPSecConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/ipsecConnections/{ipscId}")
	if err != nil {
		return nil, err
	}

	var response UpdateIPSecConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateInternetGateway Updates the specified Internet Gateway. You can disable/enable it, or change its display name
// or tags. Avoid entering confidential information.
// If the gateway is disabled, that means no traffic will flow to/from the internet even if there's
// a route rule that enables that traffic.
func (client VirtualNetworkClient) UpdateInternetGateway(ctx context.Context, request UpdateInternetGatewayRequest) (response UpdateInternetGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateInternetGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateInternetGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateInternetGatewayResponse")
	}
	return
}

// updateInternetGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateInternetGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/internetGateways/{igId}")
	if err != nil {
		return nil, err
	}

	var response UpdateInternetGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateLocalPeeringGateway Updates the specified local peering gateway (LPG).
func (client VirtualNetworkClient) UpdateLocalPeeringGateway(ctx context.Context, request UpdateLocalPeeringGatewayRequest) (response UpdateLocalPeeringGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateLocalPeeringGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateLocalPeeringGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateLocalPeeringGatewayResponse")
	}
	return
}

// updateLocalPeeringGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateLocalPeeringGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/localPeeringGateways/{localPeeringGatewayId}")
	if err != nil {
		return nil, err
	}

	var response UpdateLocalPeeringGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdatePrivateIp Updates the specified private IP. You must specify the object's OCID.
// Use this operation if you want to:
//   - Move a secondary private IP to a different VNIC in the same subnet.
//   - Change the display name for a secondary private IP.
//   - Change the hostname for a secondary private IP.
// This operation cannot be used with primary private IPs.
// To update the hostname for the primary IP on a VNIC, use
// UpdateVnic.
func (client VirtualNetworkClient) UpdatePrivateIp(ctx context.Context, request UpdatePrivateIpRequest) (response UpdatePrivateIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updatePrivateIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdatePrivateIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdatePrivateIpResponse")
	}
	return
}

// updatePrivateIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updatePrivateIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/privateIps/{privateIpId}")
	if err != nil {
		return nil, err
	}

	var response UpdatePrivateIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdatePublicIp Updates the specified public IP. You must specify the object's OCID. Use this operation if you want to:
// * Assign a reserved public IP in your pool to a private IP.
// * Move a reserved public IP to a different private IP.
// * Unassign a reserved public IP from a private IP (which returns it to your pool
// of reserved public IPs).
// * Change the display name or tags for a public IP.
// Assigning, moving, and unassigning a reserved public IP are asynchronous
// operations. Poll the public IP's `lifecycleState` to determine if the operation
// succeeded.
// **Note:** When moving a reserved public IP, the target private IP
// must not already have a public IP with `lifecycleState` = ASSIGNING or ASSIGNED. If it
// does, an error is returned. Also, the initial unassignment from the original
// private IP always succeeds, but the assignment to the target private IP is asynchronous and
// could fail silently (for example, if the target private IP is deleted or has a different public IP
// assigned to it in the interim). If that occurs, the public IP remains unassigned and its
// `lifecycleState` switches to AVAILABLE (it is not reassigned to its original private IP).
// You must poll the public IP's `lifecycleState` to determine if the move succeeded.
// Regarding ephemeral public IPs:
// * If you want to assign an ephemeral public IP to a primary private IP, use
// CreatePublicIp.
// * You can't move an ephemeral public IP to a different private IP.
// * If you want to unassign an ephemeral public IP from its private IP, use
// DeletePublicIp, which
// unassigns and deletes the ephemeral public IP.
// **Note:** If a public IP (either ephemeral or reserved) is assigned to a secondary private
// IP (see PrivateIp), and you move that secondary
// private IP to another VNIC, the public IP moves with it.
// **Note:** There's a limit to the number of PublicIp
// a VNIC or instance can have. If you try to move a reserved public IP
// to a VNIC or instance that has already reached its public IP limit, an error is
// returned. For information about the public IP limits, see
// Public IP Addresses (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Tasks/managingpublicIPs.htm).
func (client VirtualNetworkClient) UpdatePublicIp(ctx context.Context, request UpdatePublicIpRequest) (response UpdatePublicIpResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updatePublicIp, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdatePublicIpResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdatePublicIpResponse")
	}
	return
}

// updatePublicIp implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updatePublicIp(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/publicIps/{publicIpId}")
	if err != nil {
		return nil, err
	}

	var response UpdatePublicIpResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateRemotePeeringConnection Updates the specified remote peering connection (RPC).
func (client VirtualNetworkClient) UpdateRemotePeeringConnection(ctx context.Context, request UpdateRemotePeeringConnectionRequest) (response UpdateRemotePeeringConnectionResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateRemotePeeringConnection, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateRemotePeeringConnectionResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateRemotePeeringConnectionResponse")
	}
	return
}

// updateRemotePeeringConnection implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateRemotePeeringConnection(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/remotePeeringConnections/{remotePeeringConnectionId}")
	if err != nil {
		return nil, err
	}

	var response UpdateRemotePeeringConnectionResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateRouteTable Updates the specified route table's display name or route rules.
// Avoid entering confidential information.
// Note that the `routeRules` object you provide replaces the entire existing set of rules.
func (client VirtualNetworkClient) UpdateRouteTable(ctx context.Context, request UpdateRouteTableRequest) (response UpdateRouteTableResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateRouteTable, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateRouteTableResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateRouteTableResponse")
	}
	return
}

// updateRouteTable implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateRouteTable(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/routeTables/{rtId}")
	if err != nil {
		return nil, err
	}

	var response UpdateRouteTableResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateSecurityList Updates the specified security list's display name or rules.
// Avoid entering confidential information.
// Note that the `egressSecurityRules` or `ingressSecurityRules` objects you provide replace the entire
// existing objects.
func (client VirtualNetworkClient) UpdateSecurityList(ctx context.Context, request UpdateSecurityListRequest) (response UpdateSecurityListResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateSecurityList, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateSecurityListResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateSecurityListResponse")
	}
	return
}

// updateSecurityList implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateSecurityList(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/securityLists/{securityListId}")
	if err != nil {
		return nil, err
	}

	var response UpdateSecurityListResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateServiceGateway Updates the specified service gateway. The information you provide overwrites the existing
// attributes of the gateway.
func (client VirtualNetworkClient) UpdateServiceGateway(ctx context.Context, request UpdateServiceGatewayRequest) (response UpdateServiceGatewayResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateServiceGateway, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateServiceGatewayResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateServiceGatewayResponse")
	}
	return
}

// updateServiceGateway implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateServiceGateway(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/serviceGateways/{serviceGatewayId}")
	if err != nil {
		return nil, err
	}

	var response UpdateServiceGatewayResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateSubnet Updates the specified subnet's display name. Avoid entering confidential information.
func (client VirtualNetworkClient) UpdateSubnet(ctx context.Context, request UpdateSubnetRequest) (response UpdateSubnetResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateSubnet, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateSubnetResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateSubnetResponse")
	}
	return
}

// updateSubnet implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateSubnet(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/subnets/{subnetId}")
	if err != nil {
		return nil, err
	}

	var response UpdateSubnetResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateVcn Updates the specified VCN's display name.
// Avoid entering confidential information.
func (client VirtualNetworkClient) UpdateVcn(ctx context.Context, request UpdateVcnRequest) (response UpdateVcnResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateVcn, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateVcnResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateVcnResponse")
	}
	return
}

// updateVcn implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateVcn(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/vcns/{vcnId}")
	if err != nil {
		return nil, err
	}

	var response UpdateVcnResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateVirtualCircuit Updates the specified virtual circuit. This can be called by
// either the customer who owns the virtual circuit, or the
// provider (when provisioning or de-provisioning the virtual
// circuit from their end). The documentation for
// UpdateVirtualCircuitDetails
// indicates who can update each property of the virtual circuit.
// **Important:** If the virtual circuit is working and in the
// PROVISIONED state, updating any of the network-related properties
// (such as the DRG being used, the BGP ASN, and so on) will cause the virtual
// circuit's state to switch to PROVISIONING and the related BGP
// session to go down. After Oracle re-provisions the virtual circuit,
// its state will return to PROVISIONED. Make sure you confirm that
// the associated BGP session is back up. For more information
// about the various states and how to test connectivity, see
// FastConnect Overview (https://docs.us-phoenix-1.oraclecloud.com/Content/Network/Concepts/fastconnect.htm).
// To change the list of public IP prefixes for a public virtual circuit,
// use BulkAddVirtualCircuitPublicPrefixes
// and
// BulkDeleteVirtualCircuitPublicPrefixes.
// Updating the list of prefixes does NOT cause the BGP session to go down. However,
// Oracle must verify the customer's ownership of each added prefix before
// traffic for that prefix will flow across the virtual circuit.
func (client VirtualNetworkClient) UpdateVirtualCircuit(ctx context.Context, request UpdateVirtualCircuitRequest) (response UpdateVirtualCircuitResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateVirtualCircuit, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateVirtualCircuitResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateVirtualCircuitResponse")
	}
	return
}

// updateVirtualCircuit implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateVirtualCircuit(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/virtualCircuits/{virtualCircuitId}")
	if err != nil {
		return nil, err
	}

	var response UpdateVirtualCircuitResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}

// UpdateVnic Updates the specified VNIC.
func (client VirtualNetworkClient) UpdateVnic(ctx context.Context, request UpdateVnicRequest) (response UpdateVnicResponse, err error) {
	var ociResponse common.OCIResponse
	policy := common.NoRetryPolicy()
	if request.RetryPolicy() != nil {
		policy = *request.RetryPolicy()
	}
	ociResponse, err = common.Retry(ctx, request, client.updateVnic, policy)
	if err != nil {
		return
	}
	if convertedResponse, ok := ociResponse.(UpdateVnicResponse); ok {
		response = convertedResponse
	} else {
		err = fmt.Errorf("failed to convert OCIResponse into UpdateVnicResponse")
	}
	return
}

// updateVnic implements the OCIOperation interface (enables retrying operations)
func (client VirtualNetworkClient) updateVnic(ctx context.Context, request common.OCIRequest) (common.OCIResponse, error) {
	httpRequest, err := request.HTTPRequest(http.MethodPut, "/vnics/{vnicId}")
	if err != nil {
		return nil, err
	}

	var response UpdateVnicResponse
	var httpResponse *http.Response
	httpResponse, err = client.Call(ctx, &httpRequest)
	defer common.CloseBodyIfValid(httpResponse)
	response.RawResponse = httpResponse
	if err != nil {
		return response, err
	}

	err = common.UnmarshalResponse(httpResponse, &response)
	return response, err
}
