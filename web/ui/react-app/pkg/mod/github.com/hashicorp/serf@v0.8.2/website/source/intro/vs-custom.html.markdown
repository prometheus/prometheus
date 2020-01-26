---
layout: "intro"
page_title: "Serf vs. Custom Solutions"
sidebar_current: "vs-other-custom"
description: |-
  Many organizations find themselves building home grown solutions for service discovery and administration. It is an undisputed fact that distributed systems are hard; building one is error prone and time consuming. Most systems cut corners by introducing single points of failure such as a single Redis or RDBMS to maintain cluster state. These solutions may work in the short term, but they are rarely fault tolerant or scalable. Besides these limitations, they require time and resources to build and maintain.
---

# Serf vs. Custom Solutions

Many organizations find themselves building home grown solutions
for service discovery and administration. It is an undisputed fact that
distributed systems are hard; building one is error prone and time consuming.
Most systems cut corners by introducing single points of failure such
as a single Redis or RDBMS to maintain cluster state. These solutions may work in the short term,
but they are rarely fault tolerant or scalable. Besides these limitations,
they require time and resources to build and maintain.

Serf provides many features that are effortless to use out of the box.
However, Serf still may not provide the exact feature set needed by an organization.
Instead it can be used as building block, composed with other systems it provides generally
useful features for building distributed systems.

Serf is built on top of well-cited academic research where the pros, cons,
failure scenarios, scalability, etc. are all well defined enabling you to
build on the shoulder of giants.

