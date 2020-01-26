---
layout: "docs"
page_title: "Commands: Force Leave"
sidebar_current: "docs-commands-forceleave"
description: |-
  The `force-leave` command forces a member of a Serf cluster to enter the left state. Note that if the member is still actually alive, it will eventually rejoin the cluster. The true purpose of this method is to force remove "failed" nodes.
---

# Serf Force Leave

Command: `serf force-leave`

The `force-leave` command forces a member of a Serf cluster to enter the
"left" state. Note that if the member is still actually alive, it will
eventually rejoin the cluster. The true purpose of this method is to force
remove "failed" nodes.

Serf periodically tries to reconnect to "failed" nodes in case it is a
network partition. After some configured amount of time (by default 24 hours),
Serf will reap "failed" nodes and stop trying to reconnect. The `force-leave`
command can be used to transition the "failed" nodes to "left" nodes more
quickly.

## Usage

Usage: `serf force-leave [options] node`

The following command-line options are available for this command.
Every option is optional:

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command. This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.


