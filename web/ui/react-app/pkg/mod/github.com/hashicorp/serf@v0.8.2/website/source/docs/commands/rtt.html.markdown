---
layout: "docs"
page_title: "Commands: RTT"
sidebar_current: "docs-commands-rtt"
description: |-
  The rtt command estimates the network round trip time between two nodes.
---

# Serf RTT

Command: `serf rtt`

The `rtt` command estimates the network round trip time between two nodes using
Serf's network coordinate model of the cluster. See the [Network Coordinates](/docs/internals/coordinates.html)
internals guide for more information on how these coordinates are computed.

While contacting nodes as part of its normal gossip protocol, Serf builds up a
set of network coordinates for all the nodes in the cluster. Agents cache these,
and once the coordinates for two nodes are known, it's possible to estimate the
network round trip time between them using a simple calculation.

## Usage

Usage: `serf rtt [options] node1 [node2]`

At least one node name is required. If the second node name isn't given, it
is set to the agent's node name. Note that these are node names as known to
Serf as `serf members` would show, not IP addresses.

The list of available flags are:

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command. This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.

## Output

If coordinates are available, the command will print the estimated round trip
time between the given nodes:

```
$ serf rtt n1 n2
Estimated n1 <-> n2 rtt: 0.610 ms

$ serf rtt n2 # Running from n1
Estimated n1 <-> n2 rtt: 0.610 ms
```
