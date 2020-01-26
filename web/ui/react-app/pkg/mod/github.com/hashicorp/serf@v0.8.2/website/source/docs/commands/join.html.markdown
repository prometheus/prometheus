---
layout: "docs"
page_title: "Commands: Join"
sidebar_current: "docs-commands-join"
description: |-
  The `serf join` command tells a Serf agent to join an existing cluster. A new Serf agent must join with at least one existing member of a cluster in order to join an existing cluster. After joining that one member, the gossip layer takes over, propagating the updated membership state across the cluster.
---

# Serf Join

Command: `serf join`

The `serf join` command tells a Serf agent to join an existing cluster.
A new Serf agent must join with at least one existing member of a cluster
in order to join an existing cluster. After joining that one member,
the gossip layer takes over, propagating the updated membership state across
the cluster.

If you don't join an existing cluster, then that agent is part of its own
isolated cluster. Other nodes can join it.

Agents can join other agents multiple times without issue. If a node that
is already part of a cluster joins another node, then the clusters of the
two nodes join to become a single cluster.

## Usage

Usage: `serf join [options] address ...`

You may call join with multiple addresses if you want to try to join
multiple clusters. Serf will attempt to join all clusters, and the join
command will fail only if Serf was unable to join with any.

The command-line flags are all optional. The list of available flags are:

* `-replay` - If set, old user events from the past will be replayed for the
  agent/cluster that is joining. Otherwise, past events will be ignored.

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command. This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.

## Replaying User Events

When joining a cluster, the past events that were sent to the cluster are
usually ignored. However, if you specify the `-replay` flag, then past events
will be replayed on the joining cluster.

Note that only a fixed amount of past events are stored. At the time of writing
this documentation, that fixed amount is 1024 events. Therefore, if you replay
events, you'll only receive the past 1024 events (if there are that many).
