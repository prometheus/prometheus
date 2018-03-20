package api

// PacketFilterAPI パケットフィルターAPI
type PacketFilterAPI struct {
	*baseAPI
}

// NewPacketFilterAPI パケットフィルターAPI作成
func NewPacketFilterAPI(client *Client) *PacketFilterAPI {
	return &PacketFilterAPI{
		&baseAPI{
			client: client,
			FuncGetResourceURL: func() string {
				return "packetfilter"
			},
		},
	}
}
