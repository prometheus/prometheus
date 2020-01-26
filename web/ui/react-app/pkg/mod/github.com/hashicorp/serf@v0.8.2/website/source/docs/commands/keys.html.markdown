---
layout: "docs"
page_title: "Commands: Key"
sidebar_current: "docs-commands-key"
description: |-
  The `serf keys` command performs cluster-wide encryption key operations, such as installing new keys and removing old keys. When used properly, the `keys` command allows you to achieve non-disruptive encryption key rotation across a Serf cluster.
---

# Serf Keys

Command: `serf keys`

The `serf keys` command performs cluster-wide encryption key operations, such as
installing new keys and removing old keys. When used properly, the `keys`
command allows you to achieve non-disruptive encryption key rotation across a
Serf cluster.

By default, changes made to the encryption keys will not be written to disk, and
will be lost upon agent restart. It is possible to enable persistence by using
the `-keyring-file` option to the Serf agent. More information is available on
the <a href="/docs/agent/options.html">agent configuration options</a> page.

Serf allows multiple encryption keys to be in use simultaneously. This is
intended to provide a transition state while the cluster converges. It is the
responsibility of the operator to ensure that only the required encryption keys
are installed on the cluster. You can ensure that a key is not installed using
the `-list` and `-remove` options.

All variations of the `keys` command will return 0 if all nodes reply and there
are no errors. If any node fails to reply or reports failure, the exit code will
be 1.

## Usage

Usage: `serf keys [options]`

All operations are idempotent. The list of available flags are:

* `-install` - Install a new encryption key to the Serf keyring. This will
  broadcast the new key to the cluster.

* `-use` - Change the primary encryption key. The primary key is the only key
  used to encrypt messages, and is the first key used while decrypting messages.

* `-remove` - Remove a currently installed encryption key from the Serf keyring.
  Any messages transmitted using this key after this operation completes will
  fail verification and be rejected.

* `-list` - Ask all members in the cluster for a list of the keys they have
  installed. After gathering keys from all members, the results will be returned
  in a summary showing each key and the number of members which have that key
  installed. This is useful to operators to ensure that a given key has been
  installed on or removed from all members. It is possible that there are too
  many keys to fit into one message. In that case the reporting member truncates
  the list until the message can be sent. This is done to avoid not being able
  to list the keys in case there are too many keys.

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command. This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.
