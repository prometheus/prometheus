package structs

import (
	"time"

	"github.com/hashicorp/raft"
	"github.com/hashicorp/serf/serf"
)

// AutopilotConfig holds the Autopilot configuration for a cluster.
type AutopilotConfig struct {
	// CleanupDeadServers controls whether to remove dead servers when a new
	// server is added to the Raft peers.
	CleanupDeadServers bool

	// LastContactThreshold is the limit on the amount of time a server can go
	// without leader contact before being considered unhealthy.
	LastContactThreshold time.Duration

	// MaxTrailingLogs is the amount of entries in the Raft Log that a server can
	// be behind before being considered unhealthy.
	MaxTrailingLogs uint64

	// ServerStabilizationTime is the minimum amount of time a server must be
	// in a stable, healthy state before it can be added to the cluster. Only
	// applicable with Raft protocol version 3 or higher.
	ServerStabilizationTime time.Duration

	// (Enterprise-only) RedundancyZoneTag is the node tag to use for separating
	// servers into zones for redundancy. If left blank, this feature will be disabled.
	RedundancyZoneTag string

	// (Enterprise-only) DisableUpgradeMigration will disable Autopilot's upgrade migration
	// strategy of waiting until enough newer-versioned servers have been added to the
	// cluster before promoting them to voters.
	DisableUpgradeMigration bool

	// RaftIndex stores the create/modify indexes of this configuration.
	RaftIndex
}

// RaftServer has information about a server in the Raft configuration.
type RaftServer struct {
	// ID is the unique ID for the server. These are currently the same
	// as the address, but they will be changed to a real GUID in a future
	// release of Consul.
	ID raft.ServerID

	// Node is the node name of the server, as known by Consul, or this
	// will be set to "(unknown)" otherwise.
	Node string

	// Address is the IP:port of the server, used for Raft communications.
	Address raft.ServerAddress

	// Leader is true if this server is the current cluster leader.
	Leader bool

	// Voter is true if this server has a vote in the cluster. This might
	// be false if the server is staging and still coming online, or if
	// it's a non-voting server, which will be added in a future release of
	// Consul.
	Voter bool
}

// RaftConfigrationResponse is returned when querying for the current Raft
// configuration.
type RaftConfigurationResponse struct {
	// Servers has the list of servers in the Raft configuration.
	Servers []*RaftServer

	// Index has the Raft index of this configuration.
	Index uint64
}

// RaftRemovePeerRequest is used by the Operator endpoint to apply a Raft
// operation on a specific Raft peer by address in the form of "IP:port".
type RaftRemovePeerRequest struct {
	// Datacenter is the target this request is intended for.
	Datacenter string

	// Address is the peer to remove, in the form "IP:port".
	Address raft.ServerAddress

	// ID is the peer ID to remove.
	ID raft.ServerID

	// WriteRequest holds the ACL token to go along with this request.
	WriteRequest
}

// RequestDatacenter returns the datacenter for a given request.
func (op *RaftRemovePeerRequest) RequestDatacenter() string {
	return op.Datacenter
}

// AutopilotSetConfigRequest is used by the Operator endpoint to update the
// current Autopilot configuration of the cluster.
type AutopilotSetConfigRequest struct {
	// Datacenter is the target this request is intended for.
	Datacenter string

	// Config is the new Autopilot configuration to use.
	Config AutopilotConfig

	// CAS controls whether to use check-and-set semantics for this request.
	CAS bool

	// WriteRequest holds the ACL token to go along with this request.
	WriteRequest
}

// RequestDatacenter returns the datacenter for a given request.
func (op *AutopilotSetConfigRequest) RequestDatacenter() string {
	return op.Datacenter
}

// ServerHealth is the health (from the leader's point of view) of a server.
type ServerHealth struct {
	// ID is the raft ID of the server.
	ID string

	// Name is the node name of the server.
	Name string

	// Address is the address of the server.
	Address string

	// The status of the SerfHealth check for the server.
	SerfStatus serf.MemberStatus

	// Version is the Consul version of the server.
	Version string

	// Leader is whether this server is currently the leader.
	Leader bool

	// LastContact is the time since this node's last contact with the leader.
	LastContact time.Duration

	// LastTerm is the highest leader term this server has a record of in its Raft log.
	LastTerm uint64

	// LastIndex is the last log index this server has a record of in its Raft log.
	LastIndex uint64

	// Healthy is whether or not the server is healthy according to the current
	// Autopilot config.
	Healthy bool

	// Voter is whether this is a voting server.
	Voter bool

	// StableSince is the last time this server's Healthy value changed.
	StableSince time.Time
}

// IsHealthy determines whether this ServerHealth is considered healthy
// based on the given Autopilot config
func (h *ServerHealth) IsHealthy(lastTerm uint64, leaderLastIndex uint64, autopilotConf *AutopilotConfig) bool {
	if h.SerfStatus != serf.StatusAlive {
		return false
	}

	if h.LastContact > autopilotConf.LastContactThreshold || h.LastContact < 0 {
		return false
	}

	if h.LastTerm != lastTerm {
		return false
	}

	if leaderLastIndex > autopilotConf.MaxTrailingLogs && h.LastIndex < leaderLastIndex-autopilotConf.MaxTrailingLogs {
		return false
	}

	return true
}

// IsStable returns true if the ServerHealth is in a stable, passing state
// according to the given AutopilotConfig
func (h *ServerHealth) IsStable(now time.Time, conf *AutopilotConfig) bool {
	if h == nil {
		return false
	}

	if !h.Healthy {
		return false
	}

	if now.Sub(h.StableSince) < conf.ServerStabilizationTime {
		return false
	}

	return true
}

// ServerStats holds miscellaneous Raft metrics for a server
type ServerStats struct {
	// LastContact is the time since this node's last contact with the leader.
	LastContact string

	// LastTerm is the highest leader term this server has a record of in its Raft log.
	LastTerm uint64

	// LastIndex is the last log index this server has a record of in its Raft log.
	LastIndex uint64
}

// OperatorHealthReply is a representation of the overall health of the cluster
type OperatorHealthReply struct {
	// Healthy is true if all the servers in the cluster are healthy.
	Healthy bool

	// FailureTolerance is the number of healthy servers that could be lost without
	// an outage occurring.
	FailureTolerance int

	// Servers holds the health of each server.
	Servers []ServerHealth
}
