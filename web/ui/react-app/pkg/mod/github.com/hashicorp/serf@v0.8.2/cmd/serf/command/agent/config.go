package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/serf/serf"
	"github.com/mitchellh/mapstructure"
)

// This is the default port that we use for Serf communication
const DefaultBindPort int = 7946

// DefaultConfig contains the defaults for configurations.
func DefaultConfig() *Config {
	return &Config{
		DisableCoordinates:     false,
		Tags:                   make(map[string]string),
		BindAddr:               "0.0.0.0",
		AdvertiseAddr:          "",
		LogLevel:               "INFO",
		RPCAddr:                "127.0.0.1:7373",
		Protocol:               serf.ProtocolVersionMax,
		ReplayOnJoin:           false,
		Profile:                "lan",
		RetryInterval:          30 * time.Second,
		SyslogFacility:         "LOCAL0",
		QueryResponseSizeLimit: 1024,
		QuerySizeLimit:         1024,
		BroadcastTimeout:       5 * time.Second,
	}
}

type dirEnts []os.FileInfo

// Config is the configuration that can be set for an Agent. Some of these
// configurations are exposed as command-line flags to `serf agent`, whereas
// many of the more advanced configurations can only be set by creating
// a configuration file.
type Config struct {
	// All the configurations in this section are identical to their
	// Serf counterparts. See the documentation for Serf.Config for
	// more info.
	NodeName           string `mapstructure:"node_name"`
	Role               string `mapstructure:"role"`
	DisableCoordinates bool   `mapstructure:"disable_coordinates"`

	// Tags are used to attach key/value metadata to a node. They have
	// replaced 'Role' as a more flexible meta data mechanism. For compatibility,
	// the 'role' key is special, and is used for backwards compatibility.
	Tags map[string]string `mapstructure:"tags"`

	// TagsFile is the path to a file where Serf can store its tags. Tag
	// persistence is desirable since tags may be set or deleted while the
	// agent is running. Tags can be reloaded from this file on later starts.
	TagsFile string `mapstructure:"tags_file"`

	// BindAddr is the address that the Serf agent's communication ports
	// will bind to. Serf will use this address to bind to for both TCP
	// and UDP connections. If no port is present in the address, the default
	// port will be used.
	BindAddr string `mapstructure:"bind"`

	// AdvertiseAddr is the address that the Serf agent will advertise to
	// other members of the cluster. Can be used for basic NAT traversal
	// where both the internal ip:port and external ip:port are known.
	AdvertiseAddr string `mapstructure:"advertise"`

	// EncryptKey is the secret key to use for encrypting communication
	// traffic for Serf. The secret key must be exactly 16-bytes, base64
	// encoded. The easiest way to do this on Unix machines is this command:
	// "head -c16 /dev/urandom | base64". If this is not specified, the
	// traffic will not be encrypted.
	EncryptKey string `mapstructure:"encrypt_key"`

	// KeyringFile is the path to a file containing a serialized keyring.
	// The keyring is used to facilitate encryption. If left blank, the
	// keyring will not be persisted to a file.
	KeyringFile string `mapstructure:"keyring_file"`

	// LogLevel is the level of the logs to output.
	// This can be updated during a reload.
	LogLevel string `mapstructure:"log_level"`

	// RPCAddr is the address and port to listen on for the agent's RPC
	// interface.
	RPCAddr string `mapstructure:"rpc_addr"`

	// RPCAuthKey is a key that can be set to optionally require that
	// RPC's provide an authentication key. This is meant to be
	// a very simple authentication control
	RPCAuthKey string `mapstructure:"rpc_auth"`

	// Protocol is the Serf protocol version to use.
	Protocol int `mapstructure:"protocol"`

	// ReplayOnJoin tells Serf to replay past user events
	// when joining based on a `StartJoin`.
	ReplayOnJoin bool `mapstructure:"replay_on_join"`

	// QueryResponseSizeLimit and QuerySizeLimit limit the inbound and
	// outbound payload sizes for queries, respectively. These must fit
	// in a UDP packet with some additional overhead, so tuning these
	// past the default values of 1024 will depend on your network
	// configuration.
	QueryResponseSizeLimit int `mapstructure:"query_response_size_limit"`
	QuerySizeLimit         int `mapstructure:"query_size_limit"`

	// StartJoin is a list of addresses to attempt to join when the
	// agent starts. If Serf is unable to communicate with any of these
	// addresses, then the agent will error and exit.
	StartJoin []string `mapstructure:"start_join"`

	// EventHandlers is a list of event handlers that will be invoked.
	// These can be updated during a reload.
	EventHandlers []string `mapstructure:"event_handlers"`

	// Profile is used to select a timing profile for Serf. The supported choices
	// are "wan", "lan", and "local". The default is "lan"
	Profile string `mapstructure:"profile"`

	// SnapshotPath is used to allow Serf to snapshot important transactional
	// state to make a more graceful recovery possible. This enables auto
	// re-joining a cluster on failure and avoids old message replay.
	SnapshotPath string `mapstructure:"snapshot_path"`

	// LeaveOnTerm controls if Serf does a graceful leave when receiving
	// the TERM signal. Defaults false. This can be changed on reload.
	LeaveOnTerm bool `mapstructure:"leave_on_terminate"`

	// SkipLeaveOnInt controls if Serf skips a graceful leave when receiving
	// the INT signal. Defaults false. This can be changed on reload.
	SkipLeaveOnInt bool `mapstructure:"skip_leave_on_interrupt"`

	// Discover is used to setup an mDNS Discovery name. When this is set, the
	// agent will setup an mDNS responder and periodically run an mDNS query
	// to look for peers. For peers on a network that supports multicast, this
	// allows Serf agents to join each other with zero configuration.
	Discover string `mapstructure:"discover"`

	// Interface is used to provide a binding interface to use. It can be
	// used instead of providing a bind address, as Serf will discover the
	// address of the provided interface. It is also used to set the multicast
	// device used with `-discover`.
	Interface string `mapstructure:"interface"`

	// ReconnectIntervalRaw is the string reconnect interval time. This interval
	// controls how often we attempt to connect to a failed node.
	ReconnectIntervalRaw string        `mapstructure:"reconnect_interval"`
	ReconnectInterval    time.Duration `mapstructure:"-"`

	// ReconnectTimeoutRaw is the string reconnect timeout. This timeout controls
	// for how long we attempt to connect to a failed node before removing
	// it from the cluster.
	ReconnectTimeoutRaw string        `mapstructure:"reconnect_timeout"`
	ReconnectTimeout    time.Duration `mapstructure:"-"`

	// TombstoneTimeoutRaw is the string tombstone timeout. This timeout controls
	// for how long we remember a left node before removing it from the cluster.
	TombstoneTimeoutRaw string        `mapstructure:"tombstone_timeout"`
	TombstoneTimeout    time.Duration `mapstructure:"-"`

	// By default Serf will attempt to resolve name conflicts. This is done by
	// determining which node the majority believe to be the proper node, and
	// by having the minority node shutdown. If you want to disable this behavior,
	// then this flag can be set to true.
	DisableNameResolution bool `mapstructure:"disable_name_resolution"`

	// EnableSyslog is used to also tee all the logs over to syslog. Only supported
	// on linux and OSX. Other platforms will generate an error.
	EnableSyslog bool `mapstructure:"enable_syslog"`

	// SyslogFacility is used to control which syslog facility messages are
	// sent to. Defaults to LOCAL0.
	SyslogFacility string `mapstructure:"syslog_facility"`

	// RetryJoin is a list of addresses to attempt to join when the
	// agent starts. Serf will continue to retry the join until it
	// succeeds or RetryMaxAttempts is reached.
	RetryJoin []string `mapstructure:"retry_join"`

	// RetryMaxAttempts is used to limit the maximum attempts made
	// by RetryJoin to reach other nodes. If this is 0, then no limit
	// is imposed, and Serf will continue to try forever. Defaults to 0.
	RetryMaxAttempts int `mapstructure:"retry_max_attempts"`

	// RetryIntervalRaw is the string retry interval. This interval
	// controls how often we retry the join for RetryJoin. This defaults
	// to 30 seconds.
	RetryIntervalRaw string        `mapstructure:"retry_interval"`
	RetryInterval    time.Duration `mapstructure:"-"`

	// RejoinAfterLeave controls our interaction with the snapshot file.
	// When set to false (default), a leave causes a Serf to not rejoin
	// the cluster until an explicit join is received. If this is set to
	// true, we ignore the leave, and rejoin the cluster on start. This
	// only has an affect if the snapshot file is enabled.
	RejoinAfterLeave bool `mapstructure:"rejoin_after_leave"`

	// EnableCompression specifies whether message compression is enabled
	// by `github.com/hashicorp/memberlist` when broadcasting events.
	EnableCompression bool `mapstructure:"enable_compression"`

	// StatsiteAddr is the address of a statsite instance. If provided,
	// metrics will be streamed to that instance.
	StatsiteAddr string `mapstructure:"statsite_addr"`

	// StatsdAddr is the address of a statsd instance. If provided,
	// metrics will be sent to that instance.
	StatsdAddr string `mapstructure:"statsd_addr"`

	// BroadcastTimeoutRaw is the string retry interval. This interval
	// controls the timeout for broadcast events. This defaults to
	// 5 seconds.
	BroadcastTimeoutRaw string        `mapstructure:"broadcast_timeout"`
	BroadcastTimeout    time.Duration `mapstructure:"-"`
}

// BindAddrParts returns the parts of the BindAddr that should be
// used to configure Serf.
func (c *Config) AddrParts(address string) (string, int, error) {
	checkAddr := address

START:
	_, _, err := net.SplitHostPort(checkAddr)
	if ae, ok := err.(*net.AddrError); ok && ae.Err == "missing port in address" {
		checkAddr = fmt.Sprintf("%s:%d", checkAddr, DefaultBindPort)
		goto START
	}
	if err != nil {
		return "", 0, err
	}

	// Get the address
	addr, err := net.ResolveTCPAddr("tcp", checkAddr)
	if err != nil {
		return "", 0, err
	}

	return addr.IP.String(), addr.Port, nil
}

// EncryptBytes returns the encryption key configured.
func (c *Config) EncryptBytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(c.EncryptKey)
}

// EventScripts returns the list of EventScripts associated with this
// configuration and specified by the "event_handlers" configuration.
func (c *Config) EventScripts() []EventScript {
	result := make([]EventScript, 0, len(c.EventHandlers))
	for _, v := range c.EventHandlers {
		part := ParseEventScript(v)
		result = append(result, part...)
	}
	return result
}

// Networkinterface is used to get the associated network
// interface from the configured value
func (c *Config) NetworkInterface() (*net.Interface, error) {
	if c.Interface == "" {
		return nil, nil
	}
	return net.InterfaceByName(c.Interface)
}

// DecodeConfig reads the configuration from the given reader in JSON
// format and decodes it into a proper Config structure.
func DecodeConfig(r io.Reader) (*Config, error) {
	var raw interface{}
	dec := json.NewDecoder(r)
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}

	// Decode
	var md mapstructure.Metadata
	var result Config
	msdec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Metadata:    &md,
		Result:      &result,
		ErrorUnused: true,
	})
	if err != nil {
		return nil, err
	}

	if err := msdec.Decode(raw); err != nil {
		return nil, err
	}

	// Decode the time values
	if result.ReconnectIntervalRaw != "" {
		dur, err := time.ParseDuration(result.ReconnectIntervalRaw)
		if err != nil {
			return nil, err
		}
		result.ReconnectInterval = dur
	}

	if result.ReconnectTimeoutRaw != "" {
		dur, err := time.ParseDuration(result.ReconnectTimeoutRaw)
		if err != nil {
			return nil, err
		}
		result.ReconnectTimeout = dur
	}

	if result.TombstoneTimeoutRaw != "" {
		dur, err := time.ParseDuration(result.TombstoneTimeoutRaw)
		if err != nil {
			return nil, err
		}
		result.TombstoneTimeout = dur
	}

	if result.RetryIntervalRaw != "" {
		dur, err := time.ParseDuration(result.RetryIntervalRaw)
		if err != nil {
			return nil, err
		}
		result.RetryInterval = dur
	}

	if result.BroadcastTimeoutRaw != "" {
		dur, err := time.ParseDuration(result.BroadcastTimeoutRaw)
		if err != nil {
			return nil, err
		}
		result.BroadcastTimeout = dur
	}

	return &result, nil
}

// containsKey is used to check if a slice of string keys contains
// another key
func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

// MergeConfig merges two configurations together to make a single new
// configuration.
func MergeConfig(a, b *Config) *Config {
	var result Config = *a

	// Copy the strings if they're set
	if b.NodeName != "" {
		result.NodeName = b.NodeName
	}
	if b.Role != "" {
		result.Role = b.Role
	}
	if b.DisableCoordinates == true {
		result.DisableCoordinates = true
	}
	if b.Tags != nil {
		if result.Tags == nil {
			result.Tags = make(map[string]string)
		}
		for name, value := range b.Tags {
			result.Tags[name] = value
		}
	}
	if b.BindAddr != "" {
		result.BindAddr = b.BindAddr
	}
	if b.AdvertiseAddr != "" {
		result.AdvertiseAddr = b.AdvertiseAddr
	}
	if b.EncryptKey != "" {
		result.EncryptKey = b.EncryptKey
	}
	if b.LogLevel != "" {
		result.LogLevel = b.LogLevel
	}
	if b.Protocol > 0 {
		result.Protocol = b.Protocol
	}
	if b.RPCAddr != "" {
		result.RPCAddr = b.RPCAddr
	}
	if b.RPCAuthKey != "" {
		result.RPCAuthKey = b.RPCAuthKey
	}
	if b.ReplayOnJoin != false {
		result.ReplayOnJoin = b.ReplayOnJoin
	}
	if b.Profile != "" {
		result.Profile = b.Profile
	}
	if b.SnapshotPath != "" {
		result.SnapshotPath = b.SnapshotPath
	}
	if b.LeaveOnTerm == true {
		result.LeaveOnTerm = true
	}
	if b.SkipLeaveOnInt == true {
		result.SkipLeaveOnInt = true
	}
	if b.Discover != "" {
		result.Discover = b.Discover
	}
	if b.Interface != "" {
		result.Interface = b.Interface
	}
	if b.ReconnectInterval != 0 {
		result.ReconnectInterval = b.ReconnectInterval
	}
	if b.ReconnectTimeout != 0 {
		result.ReconnectTimeout = b.ReconnectTimeout
	}
	if b.TombstoneTimeout != 0 {
		result.TombstoneTimeout = b.TombstoneTimeout
	}
	if b.DisableNameResolution {
		result.DisableNameResolution = true
	}
	if b.TagsFile != "" {
		result.TagsFile = b.TagsFile
	}
	if b.KeyringFile != "" {
		result.KeyringFile = b.KeyringFile
	}
	if b.EnableSyslog {
		result.EnableSyslog = true
	}
	if b.RetryMaxAttempts != 0 {
		result.RetryMaxAttempts = b.RetryMaxAttempts
	}
	if b.RetryInterval != 0 {
		result.RetryInterval = b.RetryInterval
	}
	if b.RejoinAfterLeave {
		result.RejoinAfterLeave = true
	}
	if b.SyslogFacility != "" {
		result.SyslogFacility = b.SyslogFacility
	}
	if b.StatsiteAddr != "" {
		result.StatsiteAddr = b.StatsiteAddr
	}
	if b.StatsdAddr != "" {
		result.StatsdAddr = b.StatsdAddr
	}
	if b.QueryResponseSizeLimit != 0 {
		result.QueryResponseSizeLimit = b.QueryResponseSizeLimit
	}
	if b.QuerySizeLimit != 0 {
		result.QuerySizeLimit = b.QuerySizeLimit
	}
	if b.BroadcastTimeout != 0 {
		result.BroadcastTimeout = b.BroadcastTimeout
	}
	result.EnableCompression = b.EnableCompression

	// Copy the event handlers
	result.EventHandlers = make([]string, 0, len(a.EventHandlers)+len(b.EventHandlers))
	result.EventHandlers = append(result.EventHandlers, a.EventHandlers...)
	result.EventHandlers = append(result.EventHandlers, b.EventHandlers...)

	// Copy the start join addresses
	result.StartJoin = make([]string, 0, len(a.StartJoin)+len(b.StartJoin))
	result.StartJoin = append(result.StartJoin, a.StartJoin...)
	result.StartJoin = append(result.StartJoin, b.StartJoin...)

	// Copy the retry join addresses
	result.RetryJoin = make([]string, 0, len(a.RetryJoin)+len(b.RetryJoin))
	result.RetryJoin = append(result.RetryJoin, a.RetryJoin...)
	result.RetryJoin = append(result.RetryJoin, b.RetryJoin...)

	return &result
}

// ReadConfigPaths reads the paths in the given order to load configurations.
// The paths can be to files or directories. If the path is a directory,
// we read one directory deep and read any files ending in ".json" as
// configuration files.
func ReadConfigPaths(paths []string) (*Config, error) {
	result := new(Config)
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("Error reading '%s': %s", path, err)
		}

		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("Error reading '%s': %s", path, err)
		}

		if !fi.IsDir() {
			config, err := DecodeConfig(f)
			f.Close()

			if err != nil {
				return nil, fmt.Errorf("Error decoding '%s': %s", path, err)
			}

			result = MergeConfig(result, config)
			continue
		}

		contents, err := f.Readdir(-1)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("Error reading '%s': %s", path, err)
		}

		// Sort the contents, ensures lexical order
		sort.Sort(dirEnts(contents))

		for _, fi := range contents {
			// Don't recursively read contents
			if fi.IsDir() {
				continue
			}

			// If it isn't a JSON file, ignore it
			if !strings.HasSuffix(fi.Name(), ".json") {
				continue
			}

			subpath := filepath.Join(path, fi.Name())
			f, err := os.Open(subpath)
			if err != nil {
				return nil, fmt.Errorf("Error reading '%s': %s", subpath, err)
			}

			config, err := DecodeConfig(f)
			f.Close()

			if err != nil {
				return nil, fmt.Errorf("Error decoding '%s': %s", subpath, err)
			}

			result = MergeConfig(result, config)
		}
	}

	return result, nil
}

// Implement the sort interface for dirEnts
func (d dirEnts) Len() int {
	return len(d)
}

func (d dirEnts) Less(i, j int) bool {
	return d[i].Name() < d[j].Name()
}

func (d dirEnts) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
