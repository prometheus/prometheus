---
layout: "docs"
page_title: "Commands"
sidebar_current: "docs-commands"
description: |-
  Serf is controlled via a very easy to use command-line interface (CLI). Serf is only a single command-line application: `serf`. This application then takes a subcommand such as agent or members. The complete list of subcommands is in the navigation to the left.
---

# Serf Commands (CLI)

Serf is controlled via a very easy to use command-line interface (CLI).
Serf is only a single command-line application: `serf`. This application
then takes a subcommand such as "agent" or "members". The complete list of
subcommands is in the navigation to the left.

The `serf` CLI is a well-behaved command line application. In erroneous
cases, a non-zero exit status will be returned. It also responds to `-h` and `--help`
as you'd most likely expect. And some commands that expect input accept
"-" as a parameter to tell Serf to read the input from stdin.

To view a list of the available commands at any time, just run `serf` with
no arguments:

```
$ serf
usage: serf [--version] [--help] <command> [<args>]

Available commands are:
    agent           Runs a Serf agent
    event           Send a custom event through the Serf cluster
    force-leave     Forces a member of the cluster to enter the "left" state
    info            Provides debugging information for operators
    join            Tell Serf agent to join cluster
    keygen          Generates a new encryption key
    keys            Manipulate the internal encryption keyring used by Serf
    leave           Gracefully leaves the Serf cluster and shuts down
    members         Lists the members of a Serf cluster
    monitor         Stream logs from a Serf agent
    query           Send a query to the Serf cluster
    reachability    Test network reachability
    rtt             Estimates network round trip time between nodes
    tags            Modify tags of a running Serf agent
    version         Prints the Serf version
```

To get help for any specific command, pass the `-h` flag to the relevant
subcommand. For example, to see help about the `members` subcommand:

```
$ serf members -h
Usage: serf members [options]

  Outputs the members of a running Serf agent.

Options:

  -rpc-addr=127.0.0.1:7373  RPC address of the Serf agent.
```
