package api

import (
	"fmt"
	"github.com/sacloud/libsacloud/sacloud"
)

// InterfaceAPI インターフェースAPI
type InterfaceAPI struct {
	*baseAPI
}

// NewInterfaceAPI インターフェースAPI作成
func NewInterfaceAPI(client *Client) *InterfaceAPI {
	return &InterfaceAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "interface"
			},
		},
	}
}

// CreateAndConnectToServer 新規作成しサーバーへ接続する
func (api *InterfaceAPI) CreateAndConnectToServer(serverID int64) (*sacloud.Interface, error) {
	iface := api.New()
	iface.Server = &sacloud.Server{
		// Resource
		Resource: &sacloud.Resource{ID: serverID},
	}
	return api.Create(iface)
}

// ConnectToSwitch スイッチへ接続する
func (api *InterfaceAPI) ConnectToSwitch(interfaceID int64, switchID int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/to/switch/%d", api.getResourceURL(), interfaceID, switchID)
	)
	return api.modify(method, uri, nil)
}

// ConnectToSharedSegment 共有セグメントへ接続する
func (api *InterfaceAPI) ConnectToSharedSegment(interfaceID int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("%s/%d/to/switch/shared", api.getResourceURL(), interfaceID)
	)
	return api.modify(method, uri, nil)
}

// DisconnectFromSwitch スイッチと切断する
func (api *InterfaceAPI) DisconnectFromSwitch(interfaceID int64) (bool, error) {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("%s/%d/to/switch", api.getResourceURL(), interfaceID)
	)
	return api.modify(method, uri, nil)
}

// Monitor アクティビティーモニター取得
func (api *InterfaceAPI) Monitor(id int64, body *sacloud.ResourceMonitorRequest) (*sacloud.MonitorValues, error) {
	return api.baseAPI.monitor(id, body)
}

// ConnectToPacketFilter パケットフィルター適用
func (api *InterfaceAPI) ConnectToPacketFilter(interfaceID int64, packetFilterID int64) (bool, error) {
	var (
		method = "PUT"
		uri    = fmt.Sprintf("/%s/%d/to/packetfilter/%d", api.getResourceURL(), interfaceID, packetFilterID)
	)
	return api.modify(method, uri, nil)
}

// DisconnectFromPacketFilter パケットフィルター切断
func (api *InterfaceAPI) DisconnectFromPacketFilter(interfaceID int64) (bool, error) {
	var (
		method = "DELETE"
		uri    = fmt.Sprintf("/%s/%d/to/packetfilter", api.getResourceURL(), interfaceID)
	)
	return api.modify(method, uri, nil)
}
