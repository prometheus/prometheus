// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tsdbv2

/*
Package tsdbv2 is multi tenant alternative implementation of [storage.Storage]
that is focused on efficient processing and storage of timeseries data using a
roaribg bitmap index on top of a persistent LSM based key value storage pebble.

This is a drop in replacement for tsdb. Care has been taken to ensure  we replicate
behavior of tsdb as much as possible by copying tests from tsdb package and make
sure they pass on tsdbv2.

# Key properties

	1. Multi tenant
	2. Disk based index
	3. Sharded

# Multi tenancy

Special label __tenant__ is used to pass the ID of a tenant to the storage. The ID
value has no special meaning inside the store it is treated as a blob of []byte that
defines data locality. Any valid prometheus label value is a valid tenant ID.

By using special label, we ensure no any breaking changes on existing prometheus deployments
will happen during migration. By default a special [encoding.RootTenant] is used as a fallback.
Data ingested without a __tenant__ label is automatically assigned a [encoding.RootTenant] id.

Querying the store with __tenant__ label via the [*labels.Matcher] ensures only data for the tenant will be accessed and
processed. When no [*labels.Matcher] with __tenant__ is  provided [encoding.RootTenant] is used in its place.

Behavior of assigning [encoding.RootTenant] as fallback is configuraable with WithRootTenant option
when initializing the store, if disabled, append and query operations will require a __tenant__
to be present, failure to do so will result in a proper missing tenant id error message.


# Disk based indexing

We retain very little information in memory. All data is persisted using pebble key value store.
Caching happens at the key value store. Care is taken to avoid copying data to minimize memory impact.

Details abput the index implementation is documented else where, see [*DB.Appender] method documentation
for details on the data layout, and [*DB.Querier] on how indexed data is accessed.

# Sharding

Data is organized in shards, each shard consists of 1 million samples. During query
shards are processed concurrently based on available CPU cores.

Shard processing is iterator based, entire shards  are skipped the moement we determine
there is no interesting data in it, combining this with columnar layout, queries are
ensured to be efficient by avoiding unnecessary reads using terator seeks to  move
cursors to the relevant column data.
*/
