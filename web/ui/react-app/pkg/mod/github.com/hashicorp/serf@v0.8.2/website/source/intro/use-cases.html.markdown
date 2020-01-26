---
layout: "intro"
page_title: "Use Cases"
sidebar_current: "use-cases"
description: |-
  At this point you should know what Serf is and the high-level features Serf provides. This page lists a handful of concrete use cases of Serf. Note that this list is not exhaustive by any means. Serf is a general purpose tool and has infinitely many more use cases. But this list should give you a good idea of how Serf might be useful to you.
---

# Use Cases

At this point you should know [what Serf is](/intro/index.html) and
the high-level features Serf provides. This page lists a handful of
concrete use cases of Serf. Note that this list is not exhaustive by
any means. Serf is a general purpose tool and has infinitely many more
use cases. But this list should give you a good idea of how Serf
might be useful to you.

It is important to remember that all the use cases available below
require _no centralized state_, are masterless, and are completely
fault tolerant.

#### Web Servers and Load Balancers

Using Serf, it is trivial to create a Serf cluster
consisting of web servers and load balancers. The load balancers can
listen for membership changes and when a web server comes online or goes
offline, they can update their node list.

#### Clustering Memcached or Redis

Servers such as Memcached or Redis can be easily clustered by creating
a Serf cluster for these nodes. When membership changes, you can update
something like [twemproxy](https://github.com/twitter/twemproxy) or your
own application's list of available servers.

#### Triggering Deploys

Serf can send custom events to a Serf cluster. If you cluster your web
applications into a single cluster, you can use Serf's event system to
trigger things such as deploys. Just call `serf event deploy` and have
event handlers installed on all the nodes and the entire cluster will
receive this message within seconds and begin deploying.

#### Updating DNS Records

Keeping your internal DNS servers updated can be a finicky process.
By using Serf, the DNS server can know within seconds when nodes join,
leave, or fail, and can update records appropriately. No more stale DNS
records, or waiting for a Chef or Puppet run to clear out the records
within X minutes. With Serf, the records can be updated nearly instantly.

#### Simple Observability

Serf provider queries which can be used as a simple request/response
mechanism. It can be used very simply to provide cluster and application
observability. Calling `serf query load` can trigger all the nodes to
call `uptime` and send their load averages to the query initiator, making
it easy to check on the cluster health.

#### A Building Block for Service Discovery

One of the most difficult parts of service discovery is simply knowing
what nodes are online, at what addresses, and for what purpose. An effective
service discovery layer can easily be built on top of Serf's seamless
membership system. Serf handles all the problems of keeping an up-to-date
node list along with some information about those nodes. The service
discovery layer then only needs to answer basic questions above that.
