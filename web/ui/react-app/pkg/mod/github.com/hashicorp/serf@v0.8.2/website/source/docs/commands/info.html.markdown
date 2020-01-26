---
layout: "docs"
page_title: "Commands: Info"
sidebar_current: "docs-commands-info"
description: |-
  The `serf info` command outputs the various debugging information that can be used by operators. It also provides the agent's local name, and currently set tags. It can be used as a way to gain more insight into the state of the local agent.
---

# Serf Info

Command: `serf info`

The `serf info` command outputs the various debugging information that can
be used by operators. It also provides the agent's local name, and
currently set tags. It can be used as a way to gain more insight
into the state of the local agent.

## Usage

Usage: `serf info [options]`

The command-line flags are all optional. The list of available flags are:

* `-format` - Controls the output format. Supports `text` and `json`.
  The default format is `text`.

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command. This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.

