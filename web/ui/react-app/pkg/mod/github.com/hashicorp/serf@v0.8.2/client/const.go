package client

import (
	"net"
	"time"

	"github.com/hashicorp/serf/coordinate"
	"github.com/hashicorp/serf/serf"
)

const (
	maxIPCVersion = 1
)

const (
	handshakeCommand       = "handshake"
	eventCommand           = "event"
	forceLeaveCommand      = "force-leave"
	joinCommand            = "join"
	membersCommand         = "members"
	membersFilteredCommand = "members-filtered"
	streamCommand          = "stream"
	stopCommand            = "stop"
	monitorCommand         = "monitor"
	leaveCommand           = "leave"
	installKeyCommand      = "install-key"
	useKeyCommand          = "use-key"
	removeKeyCommand       = "remove-key"
	listKeysCommand        = "list-keys"
	tagsCommand            = "tags"
	queryCommand           = "query"
	respondCommand         = "respond"
	authCommand            = "auth"
	statsCommand           = "stats"
	getCoordinateCommand   = "get-coordinate"
)

const (
	unsupportedCommand    = "Unsupported command"
	unsupportedIPCVersion = "Unsupported IPC version"
	duplicateHandshake    = "Handshake already performed"
	handshakeRequired     = "Handshake required"
	monitorExists         = "Monitor already exists"
	invalidFilter         = "Invalid event filter"
	streamExists          = "Stream with given sequence exists"
	invalidQueryID        = "No pending queries matching ID"
	authRequired          = "Authentication required"
	invalidAuthToken      = "Invalid authentication token"
)

const (
	queryRecordAck      = "ack"
	queryRecordResponse = "response"
	queryRecordDone     = "done"
)

// Request header is sent before each request
type requestHeader struct {
	Command string
	Seq     uint64
}

// Response header is sent before each response
type responseHeader struct {
	Seq   uint64
	Error string
}

type handshakeRequest struct {
	Version int32
}

type authRequest struct {
	AuthKey string
}

type coordinateRequest struct {
	Node string
}

type coordinateResponse struct {
	Coord coordinate.Coordinate
	Ok    bool
}

type eventRequest struct {
	Name     string
	Payload  []byte
	Coalesce bool
}

type forceLeaveRequest struct {
	Node string
}

type joinRequest struct {
	Existing []string
	Replay   bool
}

type joinResponse struct {
	Num int32
}

type membersFilteredRequest struct {
	Tags   map[string]string
	Status string
	Name   string
}

type membersResponse struct {
	Members []Member
}

type keyRequest struct {
	Key string
}

type keyResponse struct {
	Messages map[string]string
	Keys     map[string]int
	NumNodes int
	NumErr   int
	NumResp  int
}

type monitorRequest struct {
	LogLevel string
}

type streamRequest struct {
	Type string
}

type stopRequest struct {
	Stop uint64
}

type tagsRequest struct {
	Tags       map[string]string
	DeleteTags []string
}

type queryRequest struct {
	FilterNodes []string
	FilterTags  map[string]string
	RequestAck  bool
	RelayFactor uint8
	Timeout     time.Duration
	Name        string
	Payload     []byte
}

type respondRequest struct {
	ID      uint64
	Payload []byte
}

type queryRecord struct {
	Type    string
	From    string
	Payload []byte
}

// NodeResponse is used to return the response of a query
type NodeResponse struct {
	From    string
	Payload []byte
}

type logRecord struct {
	Log string
}

type userEventRecord struct {
	Event    string
	LTime    serf.LamportTime
	Name     string
	Payload  []byte
	Coalesce bool
}

// Member is used to represent a single member of the
// Serf cluster
type Member struct {
	Name        string // Node name
	Addr        net.IP // Address of the Serf node
	Port        uint16 // Gossip port used by Serf
	Tags        map[string]string
	Status      string
	ProtocolMin uint8 // Minimum supported Memberlist protocol
	ProtocolMax uint8 // Maximum supported Memberlist protocol
	ProtocolCur uint8 // Currently set Memberlist protocol
	DelegateMin uint8 // Minimum supported Serf protocol
	DelegateMax uint8 // Maximum supported Serf protocol
	DelegateCur uint8 // Currently set Serf protocol
}

type memberEventRecord struct {
	Event   string
	Members []Member
}
