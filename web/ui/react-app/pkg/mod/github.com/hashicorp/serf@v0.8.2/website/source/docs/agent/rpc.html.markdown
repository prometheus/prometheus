---
layout: "docs"
page_title: "RPC"
sidebar_current: "docs-agent-rpc"
description: |-
  The Serf agent provides a complete RPC mechanism that can be used to control the agent programmatically. This RPC mechanism is the same one used by the CLI, but can be used by other applications to easily leverage the power of Serf without directly embedding. Additionally, it can be used as a fast IPC mechanism to allow applications to receive events immediately instead of using the fork/exec model of event handlers.
---

# RPC Protocol

The Serf agent provides a complete RPC mechanism that can
be used to control the agent programmatically. This RPC
mechanism is the same one used by the CLI, but can be
used by other applications to easily leverage the power
of Serf without directly embedding. Additionally, it can
be used as a fast IPC mechanism to allow applications to
receive events immediately instead of using the fork/exec
model of event handlers.

A reference implementation in Go can be found [here](https://github.com/hashicorp/serf/blob/master/client/rpc_client.go).

## Implementation Details

The RPC protocol is implemented using [MsgPack](http://msgpack.org/)
over TCP. This choice is driven by the fact that all operating
systems support TCP, and MsgPack provides a fast serialization format
that is broadly available across languages.

All RPC requests have a request header, and some requests have
a request body. The request header looks like:

```
    {"Command": "handshake", "Seq": 0}
```

All responses have a response header, and some may contain
a response body. The response header looks like:

```
    {"Seq": 0, "Error": ""}
```

The `Command` is used to specify what command the server should
run, and the `Seq` is used to track the request. Responses are
tagged with the same `Seq` as the request. This allows for some
concurrency on the server side, as requests are not purely FIFO.
Thus, the `Seq` value should not be re-used between commands.
All responses may be accompanied by an error.

Possible commands include:

* handshake - Used to initialize the connection, set the version
* auth - Used to authenticate a client
* event - Fires a new user event
* force-leave - Removes a failed node from the cluster
* join - Requests Serf join another node
* members - Returns the list of members
* members-filtered - Returns a subset of members
* tags - Modifies tags on a running Serf agent
* stream - Starts streaming events over the connection
* monitor - Starts streaming logs over the connection
* stop - Stops streaming logs or events
* leave - Serf agent performs a graceful leave and shutdown
* query - Initiates a new query
* respond - Responds to an incoming query
* install-key - Installs a new encryption key
* use-key - Changes the primary key used for encrypting messages
* remove-key - Removes an existing encryption key
* list-keys - Provides a list of encryption keys in use in the cluster
* stats - Provides a debugging information about the running serf agent
* get-coordinate - Returns the network coordinate for a node

Below each command is documented along with any request or
response body that is applicable.

### handshake

The handshake MUST be the first command that is sent, as it informs
the server which version the client is using.

The request header must be followed with a handshake body, like:

```
    {"Version": 1}
```

The body specifies the IPC version being used, however only version
1 is currently supported. This is to ensure backwards compatibility
in the future.

There is no special response body, but the client should wait for the
response and check for an error.

### auth

If the agent is configured to use an auth key, then the client must
issue an `auth` command after the handshake is complete.

The auth request body looks like:

```
    {"AuthKey": "my-secret-auth-token"}
```

The `AuthKey` must be provided and is the authorization key.
There is no special response body.

### event

The event command is used to fire a new user event. It takes the
following request body:

```
	{"Name": "foo", "Payload": "test payload", "Coalesce": true}
```

The `Name` is a string, but `Payload` is just opaque bytes. Coalesce
is used to control if Serf should enable [event coalescing](/docs/commands/event.html).

There is no special response body.

### force-leave

This command is used to remove failed nodes from a cluster. It takes
the following body:

```
    {"Node": "failed-node-name"}
```

There is no special response body.

### join

This command is used to join an existing cluster using a known node.
It takes the following body:

```
    {"Existing": ["192.168.0.1:6000", "192.168.0.2:6000"], "Replay": false}
```

The `Existing` nodes are each contacted, and `Replay` controls if we will replay
old user events or if they will simply be ignored. The response body in addition
to the header is returned. The body looks like:

```
    {"Num": 2}
```

The body returns the number of nodes successfully joined.

### members

The members command is used to return all the known members and associated
information. There is no request body, but the response looks like:

```
    {"Members": [
        {
        "Name": "TestNode"
        "Addr": [127, 0, 0, 1],
        "Port": 5000,
        "Tags": {
            "role": "test"
        },
        "Status": "alive",
        "ProtocolMin": 0,
        "ProtocolMax": 3,
        "ProtocolCur": 2,
        "DelegateMin": 0,
        "DelegateMax": 1,
        "DelegateCur": 1,
        },
        ...]
    }
```

### members-filtered

The members-filtered command is used to return a subset of the known members
based on their metadata. It takes the following body:

```
    {"Tags": {"key": "val"}, "Status": "alive", "Name": "node1"}
```

`Tags` are used to filter nodes based on tag values. `Status` is used to filter
nodes based on operational status. `Name` is used to filter based on node names.
Both `Name` and `Status`, as well as all `Tags` values, can contain regular
expression patterns.

Note that regular expression patterns will automatically be placed between start
(`^`) and end (`$`) anchors.

The response will be in the same format as the `members` command.

### tags

The tags command is used to alter the tags on a Serf agent while it is running.
A `member-update` event will be triggered immediately to notify the other agents
in the cluster of the change. The tags command can add new tags, modify existing
tags, or delete tags. The request body looks like:

```
    {"Tags": {"tag1": "val1"}, "DeleteTags": ["tag2"]}
```

### stream

The stream command is used to subscribe to a stream of all events
matching a given type filter. Events will continue to be sent until
the stream is stopped. The request body looks like:

```
    {"Type": "member-join,user:deploy"}`
```

The format of type is the same as the [event handler](/docs/agent/event-handlers.html),
except no script is specified. The one exception is that `"*"` can be specified to
subscribe to all events.

The server will respond with a standard response header indicating if the stream
was successful. However, now as events occur they will be sent and tagged with
the same `Seq` as the stream command that matches.

Assume we issued the previous stream command with Seq `50`,
we may start getting messages like:

```
    {"Seq": 50, "Error": ""}
    {
        "Event": "user",
        "LTime": 123,
        "Name": "deploy",
        "Payload": "9c45b87",
        "Coalesce": true,
    }

    {"Seq": 50, "Error": ""}
    {
        "Event": "member-join",
        "Members": [
            {
                "Name": "TestNode"
                "Addr": [127, 0, 0, 1],
                "Port": 5000,
                "Tags": {
                    "role": "test"
                },
                "Status": "alive",
                "ProtocolMin": 0,
                "ProtocolMax": 3,
                "ProtocolCur": 2,
                "DelegateMin": 0,
                "DelegateMax": 1,
                "DelegateCur": 1,
            },
            ...
        ]
    }

    {"Seq": 50, "Error": ""}
    {
        "Event": "query",
        "ID": 1023,
        "LTime": 125,
        "Name": "load",
        "Payload": "15m",
    }
```

It is important to realize that these messages are sent asynchronously,
and not in response to any command. That means if a client is streaming
commands, there may be events streamed while a client is waiting for a
response to a command. This is why the `Seq` must be used to pair requests
with their corresponding responses.

There is no limit to the number of concurrent streams a client can request,
however a message is not deduplicated, so if multiple streams match a given
event, it will be sent multiple times with the corresponding `Seq` number.

To stop streaming, the `stop` command is used.

### monitor

The monitor command is similar to the stream command, but instead of
events it subscribes the channel to log messages from the Agent.

The request is like:

```
    {"LogLevel": "DEBUG"}
```

This subscribes the client to all messages of at least DEBUG level.

The server will respond with a standard response header indicating if the monitor
was successful. However, now as logs occur they will be sent and tagged with
the same `Seq` as the monitor command that matches.

Assume we issued the previous monitor command with Seq `50`,
we may start getting messages like:

```
    {"Seq": 50, "Error": ""}
    {"Log": "2013/12/03 13:06:53 [INFO] agent: Received event: member-join"}
```

It is important to realize that these messages are sent asynchronously,
and not in response to any command. That means if a client is streaming
commands, there may be logs streamed while a client is waiting for a
response to a command. This is why the `Seq` must be used to pair requests
with their corresponding responses.

The client can only be subscribed to at most a single monitor instance.
To stop streaming, the `stop` command is used.

### stop

The stop command is used to stop either a stream or monitor.
The request looks like:

```
    {"Stop": 50}
```

This unsubscribes the client from the monitor and/or stream registered
with `Seq` value of 50.

There is no special response body.

### leave

The leave command is used trigger a graceful leave and shutdown.
There is no request body, or special response body.

### query

The query command is used to issue a new query. It takes the following request body:

```
    {
        "FilterNodes": ["foo", "bar"],
        "FilterTags": {"role": ".*web.*"},
	    "RequestAck": true,
        "Timeout": 0,
        "Name": "load",
        "Payload": "15m",
    }
```

The `Name` is a string, but `Payload` is just opaque bytes. The remaining fields are
optional. `FilterNodes` is used to restrict the nodes that should respond to only
those named. `FilterTags` is used to filter tags using a regular expression on each
tag. `RequestAck` is used to ask that nodes send an "ack" once the message is received,
otherwise only responses are delivered. `Timeout` can be provided (in nanoseconds) to
optionally override the default.

The server will respond with a standard response header indicating if the query
was successful. However, the channel is now subscribed to receive any acks or
responses. This is similar to `stream`, except scoped only to this query. The same
`Seq` is used as the query command that matches.

We will start to get the following:

```
    {"Seq": 50, "Error": ""}
    {
        "Type": "ack",
        "From": "foo",
    }

    {"Seq": 50, "Error": ""}
    {
        "Type": "response",
        "From": "foo",
        "Payload": "1.02",
    }

    {"Seq": 50, "Error": ""}
    {
        "Type": "done",
    }
```

Each query record has a `Type` to indicate what is being represented. This is
one of `ack`, `response` or `done`. Once `done` is received the client should
not expect any further messages corresponding to that query.

### respond

The respond command is with `stream` to subscribe to queries and then respond.
It takes the following request body:

```
	{"ID": 1023, "Payload": "my response"}
```

The `ID` is an opaque value that is assigned by the IPC layer. This number is
unique per client connection and cannot be used across connections. `Payload` is
just opaque bytes.

There is no special response body.

### install-key

The install-key command is used to install a new encryption key onto the
cluster's keyring.
The request looks like:

```
    {"Key": "lkuIAePQcb/XGvuLPqwNtw=="}
```

The `Key` must be 16 bytes of base64-encoded data. This value can be generated
easily using the [keygen command](/docs/commands/keygen.html).

Once invoked, this method will begin broadcasting the new key to all members in
the cluster via the gossip protocol. Once the query has completed, a response
like the following will be returned:

```
    {
        "Messages": {
            "node1": "message from node1",
            "node2": "message from node2"
        },
        "NumErr": 0,
        "NumNodes": 2,
        "NumResp": 2
    }
```

The `Messages` field contains a per-node mapping of messages. Messages may be
informational or error messages, which can be determined by examining the other
fields in the response. The `NumErr` field indicates the total number of
errors encountered by members during the query. `NumNodes` indicates the total
number of members in the cluster, and `NumResp` indicates the number of
responses received during the query.

### use-key

The use-key command is used to change the primary key, which is used to encrypt
messages.
The request looks like:

```
    {"Key": "lkuIAePQcb/XGvuLPqwNtw=="}
```

The key requested must already exist in the keyring of all agents for this
call to succeed. Once invoked, this method will broadcast the desired key to all
members. The members will attempt to change their current primary key pointer
and respond with the result.

The response returned by this method is in the same format as the `install-key`
call.

### remove-key

The remove-key command is used to remove a key from the cluster's keyring.
The request looks like:

```
    {"Key": "lkuIAePQcb/XGvuLPqwNtw=="}
```

The key requested must already exist in the keyring of each agent for this
command to succeed. Once invoked, this method will broadcast the key requested
for deletion to all members in the cluster and ask them to remove it from their
internal keyring. Each node will reply with their individual results.

The response returned by this method is in the same format as the `install-key`
call.

**NOTE**: If the key requested for deletion is currently the primary key on any
node, that node will report failure and refuse to remove the key.

### list-keys

The list-keys command is used to return a list of all encryption keys currently
in use on the cluster.
There is no request body, but the response looks like:

```
    {
        "Messages": {
            "node1": "message from node1",
            "node2": "message from node2"
        },
        "Keys": {
            "lkuIAePQcb/XGvuLPqwNtw==": 2,
            "FhADzydYiGiVz3vW7wpunQ==": 1
        },
        "NumErr": 0,
        "NumNodes": 2,
        "NumResp": 2
    }
```

This response body is almost the same as the other key operations, but notice
that it contains a `Keys` field. The `Keys` field lists all keys known to the
cluster, and how many members know about it. Typically if key broadcasting is
successful, this number should be equivalent to the `NumNodes` field. If not all
members are aware of a key, you should either rebroadcast that key using the
`install-key` RPC command, or remove it using the `remove-key` RPC command. More
on encryption keys can be found on the
[agent encryption](/docs/agent/encryption.html) page.

### stats

The stats command is used to obtain operator debugging information about the
running serf agent. There is no request body, but the response looks like:

```
    {
        "agent": {
            "name": "node1"
        },
        "runtime": {
            "os": "linux",
            "arch": "amd64",
            "version": "go1.2",
            "max_procs": "1",
            "goroutines": "22",
            "cpu_count": "4"
        },
        "serf": {
            "failed": "0",
            "left": "0",
            "event_time": "1",
            "query_time": "1",
            "event_queue": "0",
            "members": "5",
            "member_time": "5",
            "intent_queue": "0",
            "query_queue": "0"
        },
        "tags": {}
    }
```

### get-coordinate

The get-coordinate command is used to obtain the network coordinate of a given
node.

Serf builds up a set of network coordinates for all the nodes in the cluster.
Agents cache these, and once the coordinates for two nodes are known, it's
possible to estimate the network round trip time between them using a simple
calculation.

The request looks like:

```
    {"Node": "n1"}
```

Once invoked, this method will look up the coordinate in the agent's cache and
return it, yielding a response like this:

```
    {
        "Coord": {
            "Adjustment": 0,
            "Error": 1.5,
            "Height": 0,
            "Vec": [0,0,0,0,0,0,0,0]
        },
        "Ok": true
}
```

The returned coordinate is valid only if `Ok` is true. Otherwise, there wasn't
a coordinate available for the given node. This might mean that coordinates
are not enabled, or that the node has not yet contacted the agent.

See the [Network Coordinates](/docs/internals/coordinates.html)
internals guide for more information on how these coordinates are computed, and
for details on how to perform calculations with them.
