---
layout: "docs"
page_title: "Configuration"
sidebar_current: "docs-agent-config"
description: |-
  The agent has various configuration options that can be specified via the command-line or via configuration files. All of the configuration options are completely optional and their defaults will be specified with their descriptions.
---

# Configuration

The agent has various configuration options that can be specified via
the command-line or via configuration files. All of the configuration
options are completely optional and their defaults will be specified
with their descriptions.

When loading configuration, Serf loads the configuration from files
and directories in the order specified. Configuration specified later
will be merged into configuration specified earlier. In most cases,
"merge" means that the later version will override the earlier. But in
some cases, such as event handlers, merging just appends the handlers.
The exact merging behavior will be specified.

Serf also supports reloading of configuration when it receives the
SIGHUP signal. Not all changes are respected, but those that are
are documented below.

## Command-line Options

The options below are all specified on the command-line.

* `-bind` - The address that Serf will bind to for communication with
  other Serf nodes. By default this is "0.0.0.0:7946". Serf nodes may
  have different ports. If a join is specified without a port, we default
  to locally configured port. Serf uses both TCP and UDP and use the
  same port for both, so if you have any firewalls be sure to allow both protocols.
  If this configuration value is changed and no port is specified, the default of
  "7946" will be used. An important compatibility note, protocol version 2
  introduces support for non-consistent ports across the cluster. For more information,
  see the [compatibility page](/docs/compatibility.html).
  Note: To use an IPv6 address, specify "[::1]" or "[::1]:7946".

* `-iface` - This flag can be used to provide a binding interface. It can be
  used instead of `-bind` if the interface is known but not the address. If both
  are provided, then Serf verifies that the interface has the bind address that is
  provided. This flag also sets the multicast device used for `-discover`.

* `-advertise` - The advertise flag is used to change the address that we
  advertise to other nodes in the cluster. By default, the bind address is
  advertised. However, in some cases (specifically NAT traversal), there may
  be a routable address that cannot be bound to. This flag enables gossiping
  a different address to support this. If this address is not routable, the node
  will be in a constant flapping state, as other nodes will treat the non-routability
  as a failure.

* `-config-file` - A configuration file to load. For more information on
  the format of this file, read the "Configuration Files" section below.
  This option can be specified multiple times to load multiple configuration
  files. If it is specified multiple times, configuration files loaded later
  will merge with configuration files loaded earlier, with the later values
  overriding the earlier values.

* `-config-dir` - A directory of configuration files to load. Serf will
  load all files in this directory ending in ".json" as configuration files
  in alphabetical order. For more information on the format of the configuration
  files, see the "Configuration Files" section below.

* `-discover` - A cluster name, which is used with mDNS to
  automatically discover peers. When provided, Serf will respond to mDNS
  queries and periodically poll for new peers. This feature requires a network
  environment that supports multicasting.

* `-encrypt` - Specifies the secret key to use for encryption of Serf
  network traffic. This key must be 16-bytes that are base64 encoded. The
  easiest way to create an encryption key is to use `serf keygen`. All
  nodes within a cluster must share the same encryption key to communicate.

* `-keyring-file` - Specifies a file to load keyring data from. Serf is able to
  keep encryption keys in sync and perform key rotations. During a key rotation,
  there may be some period of time in which Serf is required to maintain more
  than one encryption key until all members have received the new key. The
  keyring file helps persist changes to the encryption keyring, allowing the
  agent to start and rejoin the cluster successfully later on, even if key
  rotations had been initiated by other members in the cluster. If left blank, the
  keyring will not be persisted to a file. More information on the format of the
  keyring file can be found below in the examples section.

  NOTE: this option is not compatible with the `-encrypt` option.

* `-event-handler` - Adds an event handler that Serf will invoke for
  events. This flag can be specified multiple times to define multiple
  event handlers. By default no event handlers are registered. See the
  [event handler page](/docs/agent/event-handlers.html) for more details on
  event handlers as well as a syntax for filtering event handlers by event.
  Event handlers can be changed by reloading the configuration.

* `-join` - Address of another agent to join upon starting up. This can be
  specified multiple times to specify multiple agents to join. Startup will
  succeed if any specified agent can be joined, but will fail if none of the
  agents specified can be joined. By default, the agent won't join any nodes
  when it starts up.

* `-replay` - If set, old user events from the past will be replayed for the
  agent/cluster that is joining based on a `-join` configuration. Otherwise,
  past events will be ignored. This configures for the initial join
  only.

* `-log-level` - The level of logging to show after the Serf agent has
  started. This defaults to "info". The available log levels are "trace",
  "debug", "info", "warn", "err". This is the log level that will be shown
  for the agent output, but note you can always connect via `serf monitor`
  to an agent at any log level. The log level can be changed during a
  config reload.

* `-node` - The name of this node in the cluster. This must be unique within
  the cluster. By default this is the hostname of the machine.

* `-profile` - Serf by default is configured to run in a LAN or Local Area
  Network. However, there are cases in which a user may want to use Serf over
  the Internet or (WAN), or even just locally. To support setting the correct
  configuration values for each environment, you can select a timing profile.
  The current choices are "lan", "wan", and "local". This defaults to "lan".
  If a "lan" or "local" profile is used over the Internet, or a "local" profile
  over the LAN, a high rate of false failures is risked, as the timing constrains
  are too tight.

* `-protocol` - The Serf protocol version to use. This defaults to the latest
  version. This should be set only when [upgrading](/docs/upgrading.html).
  You can view the protocol versions supported by Serf by running `serf -v`.

* `-retry-join` - Address of another agent to join after starting up. This can
  be specified multiple times to specify multiple agents to join. If Serf is
  unable to join with any of the specified addresses, the agent will retry
  the join every `-retry-interval` up to `-retry-max` attempts. This can be used
  instead of `-join` to continue attempting to join the cluster.

* `-retry-interval` - Provides a duration string to control how often the
  retry join is performed. By default, the join is attempted every 30 seconds
  until success. This should use the "s" suffix for second, "m" for minute,
  or "h" for hour.

* `-retry-max` - Provides a limit on how many attempts to join the cluster
  can be made by `-retry-join`. If 0, there is no limit, and the agent will
  retry forever. Defaults to 0.
  
* `-disable-compression` - Disable message compression for broadcasting events. Enabled by default. **Useful for debugging message payloads**.

* `-role` - **Deprecated** The role of this node, if any. By default this is blank or empty.
  The role can be used by events in order to differentiate members of a
  cluster that may have different functional roles. For example, if you're
  using Serf in a load balancer and web server setup, you only want to add
  web servers to the load balancers, so the role of web servers may be "web"
  and the event handlers can filter on that. This has been deprecated as of
  version 0.4. Instead "-tag role=foo" should be used. The role can be changed
  during a config reload

* `-rpc-addr` - The address that Serf will bind to for the agent's  RPC server.
  By default this is "127.0.0.1:7373", allowing only loopback connections.
  The RPC address is used by other Serf commands, such as  `serf members`,
  in order to query a running Serf agent. It is also used by other applications
  to control Serf using it's [RPC protocol](/docs/agent/rpc.html).

* `-snapshot` - The snapshot flag provides a file path that is used to store
  recovery information, so when Serf restarts it is able to automatically
  re-join the cluster, and avoid replay of events it has already seen. The path
  must be read/writable by Serf, and the directory must allow Serf to create
  other files, so that it can periodically compact the snapshot file.

* `-rejoin` - When provided with the `-snapshot`, Serf will ignore a previous
  leave and attempt to rejoin the cluster when starting. By default, Serf treats
  leave as a permanent intent, and does not attempt to join the cluster again
  when starting. This flag allows the snapshot state to be used to rejoin
  the cluster.

* `-tag` - The tag flag is used to associate a new key/value pair with the
  agent. The tags are gossiped and can be used to provide additional information
  such as roles, ports, and configuration values to other nodes. Multiple tags
  can be specified per agent. There is a byte size limit for the maximum number
  of tags, but in practice dozens of tags may be used. Tags can be changed during
  a config reload.


* `-tags-file` - The tags file is used to persist tag data. As an agent's tags
  are changed, the tags file will be updated. Tags can be reloaded during later
  agent starts. This option is incompatible with the `-tag` option and requires
  there be no tags in the agent configuration file, if given.

* `-syslog` - When provided, the logs will also be sent to the syslog facility.
  This flag can only be enabled on Linux or OSX systems, as Windows and Plan 9 do
  not provide the syslog facility.

* `-broadcast-timeout` - Sets the broadcast timeout, which is the max time allowed for
  responses to events including leave and force remove messages. Defaults to 5s. This
  should use the "s" suffix for second, "m" for minute, or "h" for hour.

## Configuration Files

In addition to the command-line options, configuration can be put into
files. This may be easier in certain situations, for example when Serf is
being configured using a configuration management system.

The configuration files are JSON formatted, making them easily readable
and editable by both humans and computers. The configuration is formatted
at a single JSON object with configuration within it.

#### Example Configuration File

```javascript
{
  "tags": {
    "role": "load-balancer",
    "datacenter": "east"
  },
  "event_handlers": [
    "handle.sh",
    "user:deploy=deploy.sh"
  ]
}
```

#### Configuration Key Reference

* `node_name` - Equivalent to the `-node` command-line flag.

* `role` - **Deprecated**. Equivalent to the `-role` command-line flag.

* `disable_coordinates` - Disables features related to [network coordinates](/docs/internals/coordinates.html).

* `tags` - This is a dictionary of tag values. It is the same as specifying
  the `tag` command-line flag once per tag.

* `tags_file` - Equivalent to the `-tags-file` command-line flag.

* `bind` - Equivalent to the `-bind` command-line flag.

* `interface` - Equivalent to the `-iface` command-line flag.

* `advertise` - Equivalent to the `-advertise` command-line flag.

* `discover` - Equivalent to the `-discover` command-line flag.

* `encrypt_key` - Equivalent to the `-encrypt` command-line flag.

* `log_level` - Equivalent to the `-log-level` command-line flag.

* `profile` - Equivalent to the `-profile` command-line flag.

* `protocol` - Equivalent to the `-protocol` command-line flag.

* `rpc_addr` - Equivalent to the `-rpc-addr` command-line flag.

* `rpc_auth` - Used to provide an RPC auth token. If this token is set, then
  all RPC clients are required to provide this token to make RPC requests.
  This is a simple security mechanism that can be used to prevent other users
  from making RPC requests to Serf without the token.

* `event_handlers` - An array of strings specifying the event handlers.
  The format of the strings is equivalent to the format specified for
  the `-event-handler` command-line flag.

* `start_join` - An array of strings specifying addresses of nodes to
  join upon startup.

* `replay_on_join` - Equivalent to the `-replay` command-line flag.

* `snapshot_path` - Equivalent to the `-snapshot` command-line flag.

* `leave_on_terminate` - If enabled, when the agent receives a TERM signal,
  it will send a Leave message to the rest of the cluster and gracefully
  leave. Defaults to false.

* `skip_leave_on_interrupt` - This is the similar to`leave_on_terminate` but
  only affects interrupt handling. By default, an interrupt causes Serf to
  gracefully leave, but setting this to true disables that. Defaults to false.
  Interrupts are usually from a Control-C from a shell. (This was previously
  `leave_on_interrupt` but has since changed).

* `reconnect_interval` - This controls how often the agent will attempt to
  connect to a failed node. By default this is every 30 seconds.

* `reconnect_timeout` - This controls for how long the agent attempts to connect
  to a failed node before reaping it from the cluster. By default this is 24 hours.

* `tombstone_timeout` - This controls for how long the agent remembers nodes that
  have gracefully left the cluster before reaping. By default this is 24 hours.

* `disable_name_resolution` - If enabled, then Serf will not attempt to automatically
  resolve name conflicts. Serf relies on the each node having a unique name, but as a
  result of misconfiguration sometimes Serf agents have conflicting names. By default,
  the agents that are conflicting will query the cluster to determine which node is
  believed to be "correct" by the majority of other nodes. The node(s) that are in the
  minority will shutdown at the end of the conflict resolution. Setting this flag prevents
  this behavior, and instead Serf will merely log a warning. This is not recommended since
  the cluster will disagree about the mapping of NodeName -> IP:Port and cannot reconcile
  this.

* `enable_syslog` - Equivalent to the `-syslog` command-line flag.

* `syslog_facility` - When used with `enable_syslog`, specifies the syslog
  facility messages are sent to. By default `LOCAL0` is used.

* `retry_join` - An array of strings specifying addresses of nodes to
  join upon startup with retries if we fail to join.

* `retry_max_attempts` - Equivalent to the `-retry-max` command-line flag.

* `retry_interval` - Equivalent to the `-retry-interval` command-line flag.

* `rejoin_after_leave` - Equivalent to the `-rejoin` command-line flag.

* `statsite_addr` - This provides the address of a statsite instance. If provided
  Serf will stream various telemetry information to that instance for aggregation.
  This can be used to capture various runtime information.

* `statsd_addr` - This provides the address of a statsd instance. If provided
  Serf will stream various telemetry information to that instance for aggregation.
  This can be used to capture various runtime information.

* `query_response_size_limit` and `query_size_limit` limit the inbound and outbound
  payload sizes for queries, respectively. These must fit in a UDP packet with some
  additional overhead, so tuning these past the default values of 1024 will depend
  on your network configuration.

* `broadcast_timeout` - Equivalent to the `-broadcast-timeout` command-line flag.

#### Example Keyring File

The keyring file is a simple JSON-formatted text file. It is important to
understand how Serf will use its contents. Following is an example of a keyring
file:

```javascript
[
  "QHOYjmYlxSCBhdfiolhtDQ==",
  "daZ2wnuw+Ql+2hCm7vQB6A==",
  "keTZydopxtiTY7HVoqeWGw=="
]
```

The order in which the keys appear is important. The key appearing first in the
list is the primary key, which is the key used to encrypt all outgoing messages.
The remaining keys in the list are considered secondary and are used for
decryption only. During message decryption, Serf uses the configured encryption
keys in the order they appear in the keyring file until all keys are exhausted.

## Ports Used

Serf requires 2 ports to work properly. Below we document the requirements for each
port.

* Gossip (Default 7946) This is used for communication between the Serf nodes. TCP
and UDP.

* RPC (Default 7373) This is used by agents to handle RPC from the CLI, as well as
by custom RPC clients written by users. TCP only.
