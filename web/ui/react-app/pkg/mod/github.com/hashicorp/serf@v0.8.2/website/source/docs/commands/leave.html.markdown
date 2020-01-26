---
layout: "docs"
page_title: "Commands: Leave"
sidebar_current: "docs-commands-leave"
description: |-
  The `serf leave` command triggers a graceful leave and shutdown of the agent. This is used to ensure other nodes see the agent as left instead of failed. Nodes that leave will not attempt to re-join the cluster on restarting with a snapshot.
---

# Serf Leave

Command: `serf leave`

The `serf leave` command triggers a graceful leave and shutdown of the agent.
This is used to ensure other nodes see the agent as "left" instead of "failed".
Nodes that leave will not attempt to re-join the cluster on restarting with a
snapshot.

## Usage

Usage: `serf leave`

The command-line flags are all optional. The list of available flags are:

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command.  This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.
