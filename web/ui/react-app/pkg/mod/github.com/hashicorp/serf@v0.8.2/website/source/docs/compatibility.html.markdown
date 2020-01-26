---
layout: "docs"
page_title: "Serf Protocol Compatibility Promise"
sidebar_current: "docs-upgrading-compatibility"
description: |-
  We expect Serf to run in large clusters as long-running agents. Because upgrading agents in this sort of environment relies heavily on protocol compatibility, this page makes it clear on our promise to keeping different Serf versions compatible with each other.
---

# Protocol Compatibility Promise

We expect Serf to run in large clusters as long-running agents. Because
upgrading agents in this sort of environment relies heavily on protocol
compatibility, this page makes it clear on our promise to keeping different
Serf versions compatible with each other.

We promise that every subsequent release of Serf will remain backwards
compatible with _at least_ one prior version. Concretely: version 0.5 can
speak to 0.4 (and vice versa), but may not be able to speak to 0.1.

The backwards compatibility must be explicitly enabled: Serf agents by
default will speak the latest protocol, but can be configured to speak earlier
ones. If speaking an earlier protocol, _new features may not be available_.
The ability for an agent to speak an earlier protocol is only so that they
can be upgraded without cluster disruption.

This compatibility guarantee makes it possible to upgrade Serf agents one
at a time, one version at a time. For more details on the specifics of
upgrading, see the [upgrading page](/docs/upgrading.html).

## Protocol Compatibility Table

<table class="table table-bordered table-striped">
<tr>
<th>Version</th>
<th>Protocol Compatibility</th>
</tr>
<tr>
<td>0.1</td>
<td>0</td>
</tr>
<tr>
<td>0.2</td>
<td>0, 1</td>
</tr>
<tr>
<td>0.3</td>
<td>0, 1, 2&nbsp;&nbsp;&nbsp;<span class="label label-info">see warning below</span></td>
</tr>
<tr>
<td>0.4</td>
<td>1, 2, 3&nbsp;&nbsp;&nbsp;<span class="label label-info">see warning below</span></td>
</tr>
<tr>
<td>0.5</td>
<td>2, 3, 4&nbsp;&nbsp;&nbsp;<span class="label label-info">see warning below</span></td>
</tr>
<tr>
<td>0.6</td>
<td>2, 3, 4&nbsp;&nbsp;&nbsp;<span class="label label-info">see warning below</span></td>
</tr>
</table>

~> **Warning!** Version 0.3 introduces support for dynamic ports, allowing each
agent to bind to a different port. However, this feature is only supported
if all agents are running protocol version 2. Due to the nature of this
feature, it is hard to detect using the versioning scheme. If ports are kept
consistent across the cluster, then protocol version 2 is fully backwards
compatible.

~> **Warning!** Version 0.4 introduces support for dynamic tags, allowing each
agent to provide key/value tags and update them without restarting. This feature is only supported
if all agents are running protocol version 3. If an agent is running an older protocol,
then only the "role" tag is supported for backwards compatibility.

~> **Warning!** Version 0.6 introduces support for key rotation. This feature
uses the same protocol version, but requires that all agents be on 0.6. Unless this condition
is met, attempting to use key rotation will result in errors.
