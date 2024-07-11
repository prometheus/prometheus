package model

import "github.com/google/uuid"

// Metadata request metadata.
type Metadata struct {
	TenantID               string
	BlockID                uuid.UUID
	ShardID                uint16
	ShardsLog              uint8
	SegmentEncodingVersion uint8
	ProtocolVersion        uint8
	MediaType              string
	ProductName            string
	AgentHostname          string
	AgentUUID              uuid.UUID
	RelabelerID            string
}
