---
layout: "docs"
page_title: "Security Model"
sidebar_current: "docs-internals-security"
description: |-
  Serf uses a symmetric key, or shared secret, cryptosystem to provide confidentiality, integrity and authentication.
---

# Security Model

Serf uses a symmetric key, or shared secret, cryptosystem to provide
[confidentiality, integrity and authentication](https://en.wikipedia.org/wiki/Information_security).

This means Serf communication is protected against eavesdropping, tampering,
or attempts to generate fake events. This makes it possible to run Serf over
untrusted networks such as EC2 and other shared hosting providers.

~> **Advanced Topic!** This page covers the technical details of
the security model of Serf. You don't need to know these details to
operate and use Serf. These details are documented here for those who wish
to learn about them without having to go spelunking through the source code.

## Security Primitives

The Serf security model is built on around a symmetric key, or shared secret system.
All members of the Serf cluster must be provided the shared secret ahead of time.
This places the burden of key distribution on the user.

To support confidentiality, all messages are encrypted using the
[AES-128 standard](https://en.wikipedia.org/wiki/Advanced_Encryption_Standard). The
AES standard is considered one of the most secure and modern encryption standards.
Additionally, it is a fast algorithm, and modern CPUs provide hardware instructions to
make encryption and decryption very lightweight.

AES is used with the [Galois Counter Mode (GCM)](https://en.wikipedia.org/wiki/Galois/Counter_Mode),
using a randomly generated nonce. The use of GCM provides message integrity,
as the ciphertext is suffixed with a 'tag' that is used to verify integrity.

## Message Format

In the previous section we described the crypto primitives that are used. In this
section we cover how messages are framed on the wire and interpreted.

### UDP Message Format

UDP messages do not require any framing since they are packet oriented. This
allows the message to be simple and saves space. The format is as follows:

    -------------------------------------------------------------------
    | Version (byte) | Nonce (12 bytes) | CipherText | Tag (16 bytes) |
    -------------------------------------------------------------------

The UDP message has an overhead of 29 bytes per message.
Tampering or bit corruption will cause the GCM tag verification to fail.

Once we receive a packet, we first verify the GCM tag, and only on verification,
decrypt the payload. The version byte is provided to allow future versions to
change the algorithm they use. It is currently always set to 0.

### TCP Message Format

TCP provides a stream abstraction and therefore we must provide our own framing.
This introduces a potential attack vector since we cannot verify the tag
until the entire message is received, and the message length must be in plaintext.
Our current strategy is to limit the maximum size of a framed message to prevent
a malicious attacker from being able to send enough data to cause a Denial of Service.

The TCP format is similar to the UDP format, but prepends the message with
a message type byte (similar to other Serf messages). It also adds a 4 byte length
field, encoded in Big Endian format. This increases its maximum overhead to 33 bytes.

When we first receive a TCP encrypted message, we check the message type. If any
party has encryption enabled, the other party must as well. Otherwise we are vulnerable
to a downgrade attack where one side can force the other into a non-encrypted mode of
operation.

Once this is verified, we determine the message length and if it is less than our limit,.
After the entire message is received, the tag is used to verify the entire message.

## Threat Model

The following are the various parts of our threat model:

* Non-members getting access to events
* Cluster state manipulation due to malicious messages
* Fake event generation due to malicious messages
* Tampering of messages causing state corruption
* Denial of Service against a node

We are specifically not concerned about replay attacks, as the gossip
protocol is designed to handle that due to the nature of its broadcast mechanism.

Additionally, we recognize that an attacker that can observe network
traffic for an extended period of time may infer the cluster members.
The gossip mechanism used by Serf relies on sending messages to random
members, so an attacker can record all destinations and determine all
members of the cluster.

When designing security into a system you design it to fit the threat model.
Our goal is not to protect top secret data but to provide a "reasonable"
level of security that would require an attacker to commit a considerable
amount of resources to defeat.

## Key Rotation

Serf supports rotating keys. Because our security model assumes that all current
Serf members are not compromised, we are able to use our own gossip mechanism to
distribute new keys.

The basic flow of changing the encryption key on a given Serf cluster is:

* Broadcast new key to cluster via gossip
* Instruct all members to update the key used to encrypt messages
* Remove old key

Due to the nature of distributed systems, it is difficult to reason about when
to change the key used for message encryption on any given member in a
cluster. Therefore, Serf allows multiple keys to be used to decrypt messages
while the cluster converges. Decrypting messages becomes more expensive while
there is more than one key active, as multiple attempts to decrypt any given
message are required. For this reason, utilizing multiple keys is only
recommended as a transition state.

## Future Roadmap

Eventually, Serf will be able to use the versioning byte to support
different encryption algorithms. These could be configured at the
start time of the agent.
