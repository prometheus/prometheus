---
layout: "docs"
page_title: "Commands: Tags"
sidebar_current: "docs-commands-tags"
description: |-
  The `serf tags` command modifies a member's tags while the Serf agent is running. The changed tags will be immediately propagated to other members in the cluster.
---

# Serf Tags

Command: `serf tags`

The `serf tags` command modifies a member's tags while the Serf agent is running.
The changed tags will be immediately propagated to other members in the
cluster.

By default, any tag changes made at runtime are not written to disk.
Tag persistence can be enabled using the `-tags-file` option to the Serf
agent. More information is available on the
<a href="/docs/agent/options.html">agent configuration options</a> page.

Tag changes can also be handled using
<a href="/intro/getting-started/event-handlers.html">event handlers</a> and the
`member-update` event.

## Usage

Usage: `serf tags [options]`

At least one of `-set` or `-delete` must be passed. The list of available
flags are:

* `-set` - Will either create a new tag on a member, or update it if it
  already exists with a new value. Must be passed as `-set tag=value`. Can
  be passed multiple times to set multiple tags.

* `-delete` - Delete an existing tag from a member. Can be passed multiple
  times to delete multiple tags.

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command. This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.
