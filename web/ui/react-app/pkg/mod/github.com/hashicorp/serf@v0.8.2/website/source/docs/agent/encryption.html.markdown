---
layout: "docs"
page_title: "Encryption"
sidebar_current: "docs-agent-encryption"
description: |-
  The Serf agent supports encrypting all of its network traffic. The exact method of this encryption is described on the encryption internals page.
---

# Encryption

The Serf agent supports encrypting all of its network traffic. The exact
method of this encryption is described on the
[encryption internals page](/docs/internals/security.html).

## Enabling Encryption

Enabling encryption only requires that you set an encryption key when
starting the Serf agent. The key can be set using the `-encrypt` flag
on `serf agent` or by setting the `encrypt_key` in a configuration file.
It is advisable to put the key in a configuration file to avoid other users
from being able to discover it by inspecting running processes.
The key must be 16-bytes that are base64 encoded. The easiest method to
obtain a cryptographically suitable key is by using `serf keygen`.

```
$ serf keygen
cg8StVXbQJ0gPvMd9o7yrg==
```

With that key, you can enable encryption on the agent. You can verify
encryption is enabled because the output will include "Encrypted: true".

```
$ serf agent -encrypt=cg8StVXbQJ0gPvMd9o7yrg==
==> Starting Serf agent...
==> Serf agent running!
    Node name: 'mitchellh.local'
    Bind addr: '0.0.0.0:7946'
     RPC addr: '127.0.0.1:7373'
    Encrypted: true
...
```

All nodes within a Serf cluster must share the same encryption key in
order to send and receive cluster information.

## Changing encryption keys

Serf supports changing keys used to encrypt network traffic and takes on the
burden of distributing the new key to the rest of the members in the cluster. To
support this, Serf includes a key management command with 4 actions:

```
serf keys -install <key>
serf keys -use <key>
serf keys -remove <key>
serf keys -list
```

Using the above commands, it is possible to effectively change the encryption
keys used to protect Serf without any interruption to the cluster. By executing
these commands, you are performing cluster-wide operations to distribute and
configure new encryption keys.

All of the above commands are idempotent - meaning, you may run them multiple
times with no negative consequences. This can prove useful in case of partial
success situations.

## Multiple Clusters

If you're running multiple Serf clusters, for example a Serf cluster to
maintain cache nodes and a Serf cluster to maintain web servers, then you
can use different encryption keys for each cluster in order to separately
encrypt the traffic.

This has the added benefit that the Serf agents from one cluster cannot
even accidentally join the other cluster, since the two clusters will not
be able to communicate without the same encryption key.
