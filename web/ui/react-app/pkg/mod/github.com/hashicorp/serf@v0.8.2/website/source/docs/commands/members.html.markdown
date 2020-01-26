---
layout: "docs"
page_title: "Commands: Members"
sidebar_current: "docs-commands-members"
description: |-
  The `serf members` command outputs the current list of members that a Serf agent knows about, along with their state. The state of a node can only be alive, left or failed.
---

# Serf Members

Command: `serf members`

The `serf members` command outputs the current list of members that a Serf
agent knows about, along with their state. The state of a node can only
be "alive", "left" or "failed".

Nodes in the "failed" state are still listed because Serf attempts to
reconnect with failed nodes for a certain amount of time in the case
that the failure is actually just a network partition.

## Usage

Usage: `serf members [options]`

The command-line flags are all optional. The list of available flags are:

* `-detailed` - Will show additional information per member, such as the
  protocol version that each can understand and that each is speaking.

* `-format` - Controls the output format. Supports `text` and `json`.
  The default format is `text`.

* `-name` - If provided, only members with names matching this regular
  expression will be returned.

* `-role` - If provided, output is filtered to only nodes matching
  the regular expression for role. `-role` is deprecated in favor of
  `-tag role=foo`. The regexp is anchored at the start and end,
  and must be a full match.

* `-status` - If provided, output is filtered to only nodes matching
  the regular expression for status. The regexp is anchored at the start
  and end, and must be a full match.

* `-tag key=value` - If provided, output is filtered to only nodes with the specified
  tag if its value matches the regular expression. tag can be specified
  multiple times to filter on multiple keys. The regexp is anchored at the start
  and end, and must be a full match.

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command. This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.
