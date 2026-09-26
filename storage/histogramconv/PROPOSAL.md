## Query-time histogram conversion

* **Owners:**
  * Bartłomiej (Bartek) Płotka (@bwplotka)
  * @rbizos

* **Implementation Status:** Partially implemented, a prototype exists, see [Why](#why).

* **Related Issues and PRs:**
  * <https://github.com/prometheus/prometheus/issues/16948>
  * <https://github.com/prometheus/prometheus/pull/18048>
  * <https://github.com/prometheus/prometheus/pull/18030>

* **Other docs or links:**
  * PROM-31, classic histograms stored as native histograms: <https://github.com/prometheus/proposals/blob/main/proposals/0031-classic-histograms-stored-as-native-histograms.md>

> TL;DR: Let PromQL queries written for classic histograms read native histograms and vice versa, by converting
> between the representations while evaluating the query. One experimental feature flag enables it, and
> `--query.convert-histograms-from` lists the representations (`classic`, `nhcb`, `nhe`) to convert from, none by
> default. A `__convert_stored_as__` matcher overrides that per selector, and `__debug_stored_as__="true"` adds a
> normal `__stored_as__` label showing which representation each series is stored as. Stored data wins over converted
> data.

## Why

Native histograms, with exponential or custom buckets (NHCB), are cheaper to store and are written atomically, but
they change the query syntax: `histogram_quantile(0.9, rate(foo_bucket[5m]))` becomes
`histogram_quantile(0.9, rate(foo[5m]))`. Migrating the storage therefore means migrating every dashboard, alert and
recording rule at the same time, which is the main blocker for switching to NHCB (`convert_classic_histograms_to_nhcb`)
in practice. PROM-31 left a compatibility layer for classic histogram queries as future work, and #16948 proposed it.

A prototype, started in #18048 and extended on the `histogram-promql` branch, implements such a layer as three
feature flags. This document proposes how to turn it into one coherent feature.

### Pitfalls of the current solution

Prometheus only converts between histogram representations when scraping: `convert_classic_histograms_to_nhcb`
stores classic histograms as NHCB. PromQL cannot read one representation with the syntax of the other, so:

* Switching the representation a histogram is stored as breaks every query of its `_bucket`, `_count` and `_sum`
  series, or of its native histogram, until the query is rewritten.
* Queries covering the switch need both syntaxes, e.g.
  `histogram_quantile(0.9, sum(rate(foo[5m]))) or histogram_quantile(0.9, sum by (le) (rate(foo_bucket[5m])))`, for
  as long as the queried range covers data stored before the switch.
* Scraping both representations with `always_scrape_classic_histograms` until all queries are migrated doubles the
  ingested and stored series, and does not help for histograms that only exist as native histograms, e.g. those of
  instrumentation that only exposes native histograms.
* Exponential native histograms cannot be queried with classic syntax at all, e.g. by dashboards or tools that only
  understand `le` buckets.

## Goals

* MUST: Ability to query NHCB using classic syntax.
* COULD: Ability to query classic using NH syntax.
* COULD: Ability to query NH exponential with classic syntax (derived buckets).
* MUST: Collisions and error cases are deterministically and cleanly reflected.
* MUST: Ability to control, override and debug conversions from PromQL matchers.
* SHOULD: Work in distributed PromQL systems.

### Audience

* Prometheus operators moving histograms from classic to native storage.
* Authors of dashboards, alerts and recording rules that should keep working during and after that move.
* Maintainers of projects embedding the PromQL engine.

## Non-Goals

* Changing what is stored or remote written. All conversions happen at query time.
* Conversions outside of PromQL. Federation and the remote read endpoint keep returning stored data.
* Converted series and control labels in the metadata APIs (`/api/v1/series`, `/api/v1/labels`,
  `/api/v1/label/<name>/values`, `/api/v1/metadata`), e.g. to autocomplete `foo_bucket` when only `foo` is stored.
  They are useful, but descoped from the initial implementation, see the [action plan](#action-plan).
* Converting to exponential histograms, which would require estimating how the observations are distributed within
  classic buckets.
* A reloadable configuration file option. It can be added later.
* Stabilising the feature.

## How

### Naming

The feature is called query-time histogram conversion. "Conversion" is the term Prometheus already uses for these
transformations, e.g. `histogram.ConvertNHCBToClassic` and `util/convertnhcb`. "Translation" is taken by OTLP, where
`otlp.translation_strategy` and the `otlptranslator` library translate metric and label names. "Compatibility layer"
is what PROM-31 and #16948 call the feature, and it is used in the documentation to describe its purpose.

As the scrape option `convert_classic_histograms_to_nhcb` converts too, but at scrape time, and stores the result,
everything user facing says that these conversions happen at query time:

* the feature flag has the `promql-` prefix of the other PromQL engine features, e.g. `promql-experimental-functions`,
* the flag is in the `--query.*` namespace of the PromQL engine flags, next to `--query.lookback-delta`,
* the documentation contrasts it with `convert_classic_histograms_to_nhcb`: it never changes what is stored or remote
  written.

### Values

The flag and the label use the same values, the representations data can be stored as:

| Value | Stored as | Converted for |
|---|---|---|
| `classic` | Float samples, e.g. the `foo_bucket`, `foo_count` and `foo_sum` series of a classic histogram. | `foo`, as NHCB with the same buckets. Lossless. |
| `nhcb` | Native histograms with custom buckets. | `foo_bucket`, `foo_count` and `foo_sum`, with the same buckets. Lossless. |
| `nhe` | Native histograms with exponential buckets. | `foo_bucket`, `foo_count` and `foo_sum`, with [derived buckets](#derived-buckets-of-exponential-histograms). |

The target follows from the selector, so it does not need to be named: a selector for classic series can only get
classic series, and a selector for native histograms only NHCB. `nhe` rather than `nh`, as NHCB are native histograms
too: the native histogram specification defines them as native histograms with custom buckets.

### Configuration

```
--enable-feature=promql-histogram-conversion
--query.convert-histograms-from=nhcb,nhe,classic
```

The feature flag enables the conversions and the `__convert_stored_as__` and `__debug_stored_as__` matchers. It
replaces the three prototype flags, which were never released.

`--query.convert-histograms-from` sets the default for selectors without a `__convert_stored_as__` matcher, see
below. Like `--enable-feature`, it takes a comma separated list and can be repeated, and any combination is valid. It
is empty by default: enabling the feature converts nothing until a selector asks for it, and operators opt into the
conversions their migration needs, e.g. `nhcb` to keep classic histogram queries working after enabling
`convert_classic_histograms_to_nhcb`. Setting it without the feature flag, or with an unknown value, is an error.

### Dispatching selectors

Each selector with a metric name equality matcher is handled by exactly one side:

* A name with a `_bucket`, `_count` or `_sum` suffix returns the stored classic series, plus the classic series
  converted from the native histograms stored under the base name, from `nhcb` and `nhe`. The other matchers, except
  `le` matchers, select the native histograms, and `le` matchers are applied to the converted series.
* Any other name returns the stored series, plus the NHCB assembled from the classic series of that name, from
  `classic`. As NHCB have no `le` label, nothing is converted if a `le` matcher of the selector does not match the
  empty value.
* Selectors without a metric name equality matcher, e.g. `{__name__=~"foo.*"}`, are not converted.

Both sides read stored data only, so nothing is converted twice, and all conversions can be enabled together. The
prototype's combined querier does this already
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/storage/nh_classic_compat_querier.go#L61-L67>).

Staleness markers are converted in both directions. Converted `_bucket` series format `le` like scraped classic
histograms have been since Prometheus v3.0, e.g. `le="1.0"`
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/model/textparse/openmetricsparse.go#L775-L783>),
so a converted bucket has the same labels as the stored one.

### Per selector control: `__convert_stored_as__` and `__debug_stored_as__`

Two control labels give per selector control. They are not passed on to the storage, and selected series never have
them, although `absent()` copies equality matchers on them into its output, as for any label:

* A `__convert_stored_as__` matcher selects the representations a selector reads, overriding
  `--query.convert-histograms-from`. It is matched against `classic`, `nhcb` and `nhe`.
* A `__debug_stored_as__` matcher that matches `true` adds a `__stored_as__` label to the returned series, holding the
  representation each series is stored as.

Control labels have a precedent in the scrape configuration: relabeling can set the
`__convert_classic_histograms_to_nhcb__` target label to override the scrape-time conversion per target
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/scrape/scrape.go#L1362>).

| Selector | Result |
|---|---|
| `foo_bucket` | Stored series, plus the conversions of `--query.convert-histograms-from`. |
| `foo_bucket{__convert_stored_as__="classic"}` | Stored series only. |
| `foo_bucket{__convert_stored_as__=~"classic\|nhcb"}` | Stored series, plus the series converted from NHCB. |
| `foo_bucket{__convert_stored_as__="nhcb"}` | Only the series converted from NHCB. |
| `foo{__convert_stored_as__=~"nhcb\|nhe"}` | Stored native histograms only. |
| `foo{__convert_stored_as__="nhe"}` | Stored exponential histograms only. |
| `foo{__convert_stored_as__="classic"}` | Only the NHCB converted from classic series. |
| `foo_bucket{__debug_stored_as__="true"}` | Same as `foo_bucket`, with `__stored_as__` on every series. |
| `foo_bucket{__convert_stored_as__=~".*", __debug_stored_as__="true"}` | Stored series, plus the series converted from every representation, with `__stored_as__`. |

The rules:

1. Without a `__convert_stored_as__` matcher, a selector returns the stored series, plus the conversions from the
   representations listed by `--query.convert-histograms-from`.
2. With one, a selector returns the samples of each representation the matcher matches, stored or converted, even if
   the flag does not list it. Float samples are `classic`, native histograms with custom buckets `nhcb` and those with
   exponential buckets `nhe`, and converted samples have the representation they were converted from. Several
   matchers on the label must all match, as for other labels, and a matcher that matches none of the representations
   selects nothing. Where a stored series changes to a representation the selector does not read, it gets a
   staleness marker, like a converted series whose source changes.
3. With `__debug_stored_as__="true"`, the value of `__stored_as__` is the representation of the samples of the series,
   so a stored series whose samples change representation, e.g. from `nhcb` to `nhe`, is split in two. Otherwise,
   converted series look exactly like stored ones, and are merged with them, see
   [below](#histograms-stored-in-both-representations).
4. `__stored_as__` is a normal label, and PromQL needs no changes for it. Functions keep it, as they only drop
   `__name__`, `__type__` and `__unit__`
   (<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/schema/labels.go#L24-L26>),
   and aggregations and vector matching treat it like any other label: `sum by (le)` drops it, and
   `sum by (le, __stored_as__)` keeps it. Matchers on it go to the storage, where no series has it, so representations
   are selected with `__convert_stored_as__`.
5. As for any selector, at least one matcher besides the control matchers must not match the empty value.

The names say what the labels do. Stored data is selected by its own representation, e.g.
`foo_bucket{__convert_stored_as__="classic"}`, and as returned series only carry a label in debug queries, stored and
converted series compare without label handling:
`foo_count{__convert_stored_as__="nhcb"} - foo_count{__convert_stored_as__="classic"}` returns the difference per
series. The PromQL engine passes all matchers to `Select` unchanged
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/promql/engine.go#L1123>),
so the conversion layer sees them.

Keeping the debug switch separate means that no representation matcher, e.g. `__convert_stored_as__!="nhe"`, adds a
label by accident. To see the representations in the result of an aggregation, add `__stored_as__` to its `by`
clause, e.g. `histogram_quantile(0.9, sum by (le, __stored_as__) (rate(foo_bucket{__debug_stored_as__="true"}[5m])))`,
and a binary operation with debug on one side only matches with `ignoring(__stored_as__)`, as for any label that
only one side has.

Control labels do not degrade gracefully, though. Where the feature is disabled, and in the metadata APIs, no series
has them, so selectors with `__convert_stored_as__` or `__debug_stored_as__` matchers return nothing. That is
acceptable for an experimental feature. A virtual label that only converted series have would keep its meaning
there, see the [alternatives](#alternatives).

### Histograms stored in both representations

A naive layer that returns converted series in addition to the stored ones, like the prototype, fails queries or
double counts where a histogram is stored in both representations in the same range, see the
[examples](#examples). This happens in range queries across a migration, and when both representations are scraped
at the same time.

Instead, where a selector reads a histogram both stored and converted, stored data wins, and conversions only fill
its gaps:

* A converted sample is dropped where the stored data the selector reads has a non-stale sample of the same histogram
  at the same timestamp. For classic series, the same histogram means the same labels except `__name__` and `le`,
  e.g. `foo_bucket{job="a"}` only gets buckets converted from `foo{job="a"}` at timestamps where no
  `foo_bucket{job="a"}` series has a sample, also if the selector has a `le` matcher. This also covers buckets
  converted from exponential histograms, whose `le` values differ from the stored ones, so merging by labels alone
  would mix both bucket layouts.
* Converted series are then merged into the stored series with the same labels.
* Where one side of a merged series ends with a staleness marker and the other one continues it, the marker is
  dropped, i.e. if the other side has a sample at the same timestamp, or if the sample before the marker is from the
  other side, which has samples after it. When a histogram switches representation, the series of the old one get
  staleness markers at the timestamp of the first sample of the new one, in the same scrape
  (<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/scrape/scrape.go#L1753-L1768>),
  or, after a configuration reload, at about that time, when the next scrape of the old configuration would have been
  (<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/scrape/scrape.go#L1675-L1682>).
  They would otherwise hide the series continuing them for up to a scrape interval.
* Where the representations differ in kind, e.g. NHCB converted from classic series followed by stored exponential
  histograms, functions across the switch return PromQL's usual warning about mixing exponential and custom bucket
  histograms, rather than failing the query.
* In debug queries, stored and converted series have different `__stored_as__` values and are not merged, but stored
  data still wins, so `foo_bucket{__debug_stored_as__="true"}` shows which representation `foo_bucket` takes each
  sample from. Selectors that do not read the stored representation, e.g.
  `foo_bucket{__convert_stored_as__="nhcb"}`, return all converted samples, e.g. to compare them with
  `foo_bucket{__convert_stored_as__="classic"}`.

Converted and merged series are sorted by labels, which also fixes the prototype's unsorted output.

#### Examples

These were checked against a naive layer, i.e. the prototype, with all conversions enabled.

A migration from classic histograms to NHCB, e.g. by enabling `convert_classic_histograms_to_nhcb` at 5m. The classic
series go stale at 5m, the NHCB starts at 5m, and the count grows by 2 per minute throughout:

```
foo_count{job="a"}  0 2 4 6 8 stale          from 0m to 5m, as a classic histogram
foo{job="a"}        10 12 14 16 18 (count)   from 5m to 9m, as an NHCB
```

* `foo_count` at 7m returns 14, as the stored series is stale by then.
* `rate(foo_count[5m])` at 7m fails with `vector cannot contain metrics with the same labelset`: its window has
  stored samples from before 5m and converted ones from after, i.e. two series with the same labels. `rate(foo[5m])`
  fails the same way in the other direction, and so does every range query with steps covering the switch, e.g. a
  dashboard showing the last week.
* With stored data winning, both are one series, and `rate()` returns 2 per minute across the switch.

Classic and exponential histograms scraped side by side, e.g. with `always_scrape_classic_histograms`, converting
from `nhe` and `classic`:

```
bar_bucket{job="a", le="1.0"}   1
bar_bucket{job="a", le="2.0"}   3
bar_bucket{job="a", le="+Inf"}  4
bar_count{job="a"}              4
bar{job="a"}                    {count:4, (0.5,1]:1, (1,2]:2, (2,4]:1}
```

* `bar_count` and `bar` fail, as each exists both stored and converted.
* `sum(bar_count)` returns 8 instead of 4, and `sum by (le) (bar_bucket)` returns 2, 6, 4 and 8 for the `le` values
  1.0, 2.0, 4.0 and +Inf: aggregations hide the collision and silently double count.
* With stored data winning, nothing is converted at the timestamps of stored samples, so all of them return the
  stored data.

The same happens with NHCB and classic histograms scraped side by side (`convert_classic_histograms_to_nhcb` with
`always_scrape_classic_histograms`), where every converted bucket has the same labels as a stored one.

### Where conversions apply

Conversions become an option of the PromQL engine, next to toggles like `EnableDelayedNameRemoval`
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/promql/engine.go#L345-L353>).
When enabled, the engine wraps the querier it gets for each query
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/promql/engine.go#L819>),
which sits above the fanout to local and remote read storage, and nothing else in the engine changes. So:

* They apply to the query API, rules and console templates, and to both local and remote read data.
* The control matchers are stripped before selecting from the storage, so remote read endpoints never see them.
* The remote read endpoint, federation and the metadata APIs return stored data only, and do not interpret
  `__convert_stored_as__` and `__debug_stored_as__` matchers.
* Projects embedding the engine can enable them with the same option.

The prototype wraps the local storage below the fanout instead
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/cmd/prometheus/main.go#L970-L984>).
Data from remote read storage is not converted there, and the conversions leak into the remote read endpoint, but
only for sampled reads
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/storage/remote/read_handler.go#L136>),
not for streamed chunks
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/storage/remote/read_handler.go#L208>).

In distributed PromQL systems:

* Systems that select from other Prometheus servers or storages, e.g. through remote read, convert where the PromQL
  engine runs, and the other side needs no support.
* Systems that evaluate parts of a query in other engines pass the control matchers on, as they are part of the query
  text. Those engines need the feature, with the same `--query.convert-histograms-from`, for consistent results.
* Systems that shard a selector by series must keep all representations of a histogram in the same shard, e.g. by
  sharding on the labels without `__name__` and `le`: the conversion reads the other representation, and stored data
  can only win where the conversion sees it.

### Derived buckets of exponential histograms

An exponential histogram has no fixed bucket boundaries, and classic `_bucket` series can only be aggregated by `le`
if they share them. So all exponential histograms selected by one selector are converted with the same derived
boundaries: the union of their bucket boundaries, reduced to the lowest schema amongst them
(<https://github.com/prometheus/prometheus/blob/aef3a9c1fb268dd79d71432c658a3c608a469915/storage/nhcb_querier.go#L32-L63>).
The cumulative count at each boundary is exact, as the boundaries of a lower schema are a subset of those of a higher
one, and only zero thresholds that are not a boundary of the lowest schema are approximated. Nothing is extrapolated
or interpolated, but:

* the `le` values depend on the selected series and time range,
* one low resolution histogram lowers the resolution of all of them,
* every boundary is one series, so histograms with many buckets result in many `_bucket` series.

### Performance

* Every converted selector does one more select. Selectors with a `le` matcher do another one to find the timestamps
  of the stored classic histograms, if anything was converted.
* Conversions, and selectors with control matchers, buffer the samples they read, the classic side all native
  histograms of the selector, as the derived buckets depend on all of them. That memory is not accounted in
  `--query.max-samples` yet, and should be.
* Letting stored data win needs the timestamps of the stored samples of every histogram that is converted too, so
  those stored series are read twice. That only costs where a query covers a histogram stored in both
  representations, e.g. across a migration.
* Select hints could later let the storage skip data that is not converted, as PROM-31 suggested.

## Alternatives

1. One flag per conversion, as in the prototype. Combinations need more flags and exclusion rules.
2. Directional values, e.g. `nhcb-to-classic`, in the flag or in a control label like `__hist_translate__`. More
   explicit, but the selector already determines the target, and they cannot name stored data.
3. A virtual label that only converted series have, e.g. `__converted_from__="nhcb"`, with standard label semantics:
   `=""` selects stored series, and results have the label whenever a selector matches on it. It keeps its meaning
   where the feature is disabled, as no stored series has the label, but stored data is selected by the empty value,
   and a selector opting into a conversion gets the label on its results, so converted series never merge with
   stored ones, e.g. across a migration.
4. One control label with a `debug` value, e.g. `__stored_as__=~"debug|nhcb"`, which the engine keeps in the
   aggregations and vector matching of debug queries. Debugging needs no other change to a query, even one that
   aggregates by `le`, but matchers like `!="nhe"` or `=~".*"` also match `debug`, and no other label behaves like
   that in PromQL.
5. Metric name quoting as the switch, suggested in #16948: `foo_bucket` converts, `{"foo_bucket"}` doesn't. No new
   syntax, but quoting has no meaning today, UTF-8 names must be quoted and could never be converted, and the parser
   does not keep the quoting for the storage.
6. A query API parameter, like `lookback_delta`. Applies to the whole query and is not part of the query text, so it
   cannot be used in rules. Could complement the matchers later.
7. PromQL functions, e.g. `histogram_buckets()` from #18030. Requires rewriting queries, which is what this feature
   avoids.
8. Scraping both representations (`always_scrape_classic_histograms`) until all queries are migrated. Doubles the
   storage, and does not help once classic histograms are dropped, or for exponential histograms.

## Action Plan

* [ ] Agree on this document.
* [ ] Move the prototype into its own package, without behaviour changes.
* [ ] Replace the three flags with `promql-histogram-conversion` and `--query.convert-histograms-from`, wired as an
      engine option.
* [ ] Add the `__convert_stored_as__` and `__debug_stored_as__` control labels.
* [ ] Let stored data win, merging converted with stored series.
* [ ] Account buffered samples in the query limits.
* [ ] Show converted series, and interpret the control labels, in the metadata APIs.

## Appendix: PromQL today

How PromQL treats the control labels and a `__stored_as__` label today, checked with a throwaway PromQL test on the
prototype branch, without any conversion layer. Control matchers select nothing, and `__stored_as__` behaves like any
label: functions keep it, `by` and `on` drop it unless listed, and vector matching needs it on both sides unless
ignored:

```
load 1m
	foo_bucket{job="a", le="1"}	1+1x5
	bar_bucket{job="a", le="1", __stored_as__="nhcb"}	1+1x5
	bar_bucket{job="a", le="+Inf", __stored_as__="nhcb"}	2+2x5
	bar_count{job="a", __stored_as__="nhcb"}	2+2x5
	baz_count{job="a"}	2+2x5

eval instant at 1m foo_bucket{__convert_stored_as__="classic"}

eval instant at 1m foo_bucket{__debug_stored_as__="true"}

eval instant at 5m rate(bar_bucket{le="1"}[5m])
	{job="a", le="1", __stored_as__="nhcb"} 0.016666666666666666

eval instant at 5m histogram_quantile(0.5, bar_bucket)
	{job="a", __stored_as__="nhcb"} 1

eval instant at 5m sum by (le) (bar_bucket)
	{le="1"} 6
	{le="+Inf"} 12

eval instant at 5m sum by (le, __stored_as__) (bar_bucket)
	{le="1", __stored_as__="nhcb"} 6
	{le="+Inf", __stored_as__="nhcb"} 12

eval instant at 5m bar_count / on(job) baz_count
	{job="a"} 1

eval instant at 5m bar_count / baz_count

eval instant at 5m bar_count / ignoring(__stored_as__) baz_count
	{job="a"} 1
```
