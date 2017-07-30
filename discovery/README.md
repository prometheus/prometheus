### Service Discovery

This directory contains the service discovery (SD) component of Prometheus.



## Design of a Prometheus SD

There are many requests to add new SDs to Prometheus, this section looks at
what makes a good SD and covers some of the common implementation issues.

### Does this make sense as an SD?

The first question to be asked is does it make sense to add this particular
SD? An SD mechanism should be reasonably well established, and at a minimum in
use across multiple organisations. It should allow discovering of machines
and/or services running somewhere. When exactly an SD is popular enough to
justify being added to Prometheus natively is an open question.

It should not be a brand new SD mechanism, or a variant of an established
mechanism. We want to integrate Prometheus with the SD that's already there in
your infrastructure, not invent yet more ways to do service discovery. We also
do not add mechanisms to work around users lacking service discovery and/or
configuration management infrastructure.

SDs that merely discover other applications running the same software (e.g.
talk to one Kafka or Cassandra server to find the others) are not service
discovery. In that case the SD you should be looking at is whatever decides
that a machine is going to be a Kafka server, likely a machine database or
configuration management system.

If something is particularly custom or unusual, `file_sd` is the generic
mechanism provided for users to hook in. Generally with Prometheus we offer a
single generic mechanism for things with infinite variations, rather than
trying to support everything natively (see also, alertmanager webhook, remote
read, remote write, node exporter textfile collector). For example anything
that would involve talking to a relational database should use `file_sd`
instead.

For configuration management systems like Chef, while they do have a
database/API that'd in principle make sense to talk to for service discovery,
the idiomatic approach is to use Chef's templating facilities to write out a
file for use with `file_sd`.


### Mapping from SD to Prometheus

The general principle with SD is to extract all the potentially useful
information we can out of the SD, and let the user choose what they need of it
using relabelling. This information is generally termed metadata.

Metadata is exposed as a set of key/value pairs (labels) per target. The keys
are prefixed with `__meta_sdname_`, and there should also be an `__address__`
label with the host:port of the target (preferably an IP address to avoid DNS
lookups). No other labelnames should be exposed.

It is very common for initial pull requests for new SDs to include hardcoded
assumptions that make sense for the the author's setup. SD should be generic,
any customisation should be handled via relabelling. There should be basically
no business logic, filtering, or transformations of the data from the SD beyond
that which is needed to fit it into the metadata data model. 

Arrays (e.g. a list of tags) should be converted to a single label with the
array values joined with a comma. Also prefix and suffix the value with a
comma. So for example the array `[a, b, c]` would become `,a,b,c,`. As
relabelling regexes are fully anchored, this makes it easier to write correct
regexes against (`.*,a,.*` works no matter where `a` appears in the list). The
canonical example of this is `__meta_consul_tags`.

Maps, hashes and other forms of key/value pairs should be all prefixed and
exposed as labels. For example for EC2 tags, there would be
`__meta_ec2_tag_Description=mydescription` for the Description tag. Labelnames
may only contain `[_a-zA-Z0-9]`, sanitize by replacing with underscores as needed.

For targets with multiple potential ports, you can a) expose them as a list, b)
if they're named expose them as a map or c) expose them each as their own
target. Kubernetes SD takes the target per port approach. a) and b) can be
combined.

For machine-like SDs (OpenStack, EC2, Kubernetes to some extent) there may
be multiple network interfaces for a target. Thus far reporting the details
of only the first network interface has sufficed.


### Other implementation considerations

SDs are intended to dump all possible targets. For example the optional use of
EC2 service discovery would be to take the entire region's worth of EC2
instances it provides and do everything needed in one `scrape_config`. For
large deployments where you are only interested in a small proportion of the
returned targets, this may cause performance issues. If this occurs it is
acceptable to also offer filtering via whatever mechanisms the SD exposes. For
EC2 that would be the `Filter` option on `DescribeInstances`. Keep in mind that
this is a performance optimisation, it should be possible to do the same
filtering using relabelling alone. As with SD generally, we do not invent new
ways to filter targets (that is what relabelling is for), merely offer up
whatever functionality the SD itself offers.

It is a general rule with Prometheus that all configuration comes from the
configuration file. While the libraries you use to talk to the SD may also
offer other mechanisms for providing configuration/authentication under the
covers (EC2's use of environment variables being a prime example), using your SD
mechanism should not require this. Put another way, your SD implementation
should not read environment variables or files to obtain configuration.

Some SD mechanisms have rate limits that make them challenging to use. As an
example we have unfortunately had to reject Amazon ECS service discovery due to
the rate limits being so low that it would not be usable for anything beyond
small setups.

If a system offers multiple distinct types of SD, select which is in use with a
configuration option rather than returning them all from one mega SD that
requires relabelling to select just the one you want. So far we have only seen
this with Kubernetes. When a single SD with a selector vs.  multiple distinct
SDs makes sense is an open question.

If there is a failure while processing talking to the SD, abort rather than
returning partial data. It is better to work from stale targets than partial
or incorrect metadata.

The information obtained from service discovery is not considered sensitive
security wise. Do not return secrets in metadata, anyone with access to
the Prometheus server will be able to see them.
