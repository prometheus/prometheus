# Native Stateset Text Format — Design Proposal

**Status:** draft  
**Date:** 2026-04-29  
**Branch:** native-stateset-representation

---

## Background

OpenMetrics 1.0 represents a stateset as a _family_ of float gauge series, one per
state.  Every series carries the state name in an extra label whose name equals
the metric family name:

```
# TYPE kube_pod_status_phase stateset
kube_pod_status_phase{namespace="prod",pod="web-1",kube_pod_status_phase="Failed"}    0
kube_pod_status_phase{namespace="prod",pod="web-1",kube_pod_status_phase="Pending"}   0
kube_pod_status_phase{namespace="prod",pod="web-1",kube_pod_status_phase="Running"}   1
kube_pod_status_phase{namespace="prod",pod="web-1",kube_pod_status_phase="Succeeded"} 0
kube_pod_status_phase{namespace="prod",pod="web-1",kube_pod_status_phase="Unknown"}   0
```

For a cluster with N pods, this is **5 × N** scrape lines, **5 × N** TSDB
series, and **5 × N** chunk allocations.  The implementation on this branch
stores the same information in one TSDB series per pod, encoded as a bitset
(`Values uint64`).  The native in-memory representation needs a matching
native _wire_ representation so that a target can emit one line per pod
instead of five.

### Reference: native histogram text format

Native histograms use the compact `{{key:value …}}` notation introduced in
`TestExpression()` (`model/histogram/float_histogram.go:209`):

```
# TYPE rpc_latency_seconds histogram
rpc_latency_seconds{handler="/api"} {{schema:0 count:12 sum:100 z_bucket:2 buckets:[2 3 0 1 4]}}
```

The outer `{{` / `}}` delimiters are unambiguous because `{…}` is reserved for
label sets and `{{` cannot begin a label set.  This document proposes an
analogous encoding for statesets.

---

## Proposed syntax

```
<metric_name>['{' <labels> '}'] '{{' 'lname' ':' <label_name> [<state> '=' <0|1>]… '}}' [<timestamp>]
```

### Tokens

| token | description |
|---|---|
| `{{` | opens the stateset value (reuse `tBraceOpen tBraceOpen` or a dedicated `tOpenStateset` token) |
| `lname` | keyword introducing the label name field |
| `:` | separator between keyword and value |
| `<label_name>` | unquoted identifier; the label that carries state in the OM1 wire format |
| `<state>` | unquoted identifier; a state name — must satisfy `[a-zA-Z_:][a-zA-Z0-9_:]*` (same as MetricName) |
| `=` | separator between state name and its active flag |
| `0` or `1` | active flag — `1` means the state is active, `0` means inactive |
| `}}` | closes the stateset value |

State names in the `{{…}}` body appear in **any order**; the parser sorts them
into the lexicographically-ordered `Names` slice required by the `StateSet`
data type.

### Examples

**Basic 5-state pod phase, one pod:**

```
# TYPE kube_pod_status_phase stateset
kube_pod_status_phase{namespace="prod",pod="web-1"} {{lname:kube_pod_status_phase Failed=0 Pending=0 Running=1 Succeeded=0 Unknown=0}} 1704067200000
```

**Multiple pods:**

```
kube_pod_status_phase{namespace="prod",pod="web-1"} {{lname:kube_pod_status_phase Failed=0 Pending=0 Running=1 Succeeded=0 Unknown=0}}
kube_pod_status_phase{namespace="prod",pod="web-2"} {{lname:kube_pod_status_phase Failed=1 Pending=0 Running=0 Succeeded=0 Unknown=0}}
kube_pod_status_phase{namespace="prod",pod="web-3"} {{lname:kube_pod_status_phase Failed=0 Pending=0 Running=0 Succeeded=0 Unknown=1}}
```

This replaces 15 OM1 float lines with 3.

**Feature-flag stateset (multiple states active at once):**

```
# TYPE feature_flags stateset
feature_flags{service="api"} {{lname:flag auth=1 caching=1 debug=0 logging=1 tracing=0}}
```

**All states inactive:**

```
node_role{node="node-1"} {{lname:role control_plane=0 etcd=0 worker=0}}
```

**Empty stateset** (no known states — unusual but representable):

```
ephemeral_feature{} {{lname:state}}
```

---

## Mapping to the `StateSet` struct

The `StateSet` type (in `model/stateset/stateset.go`) has three fields:

| field | type | source in wire format |
|---|---|---|
| `LabelName` | `string` | value of the `lname:` key |
| `Names` | `[]string` | keys of all `name=0\|1` pairs, sorted lexicographically |
| `Values` | `uint64` | bitset assembled from the `=1` pairs: bit _i_ set ↔ `Names[i]` is active |

The parser never relies on the wire order of state names; it always sorts
them before constructing the bitset.  This is the same invariant the
promqltest `ss(…)` parser maintains.

---

## `# TYPE` requirement

A native stateset sample **must** be preceded by a `# TYPE … stateset` line,
exactly as OM1 requires.  Parsers use the type hint to distinguish the
`{{lname:… state=0|1 …}}` value from any hypothetical future syntax that also
uses `{{…}}`.

If a `{{lname:…}}` value is encountered without a `stateset` type hint the
parser should return an error.

---

## Relationship to the OM1 float-series encoding

The two encodings carry identical information.  A scrape target **must not**
emit both in the same scrape for the same metric family; the Prometheus server
should return an error if it receives duplicate representations.

When a server accepts a native stateset scrape, the `StateSetParser`
aggregation pass that currently converts OM1 float series into native samples
is **not** applied (the aggregation is not needed because the sample is
already native).

The `# HELP` line, unit annotations, and exemplars are **not** supported on
native stateset lines (they belong on classic float gauge lines in OM1).

---

## Parser changes

### 1. Lexer (`model/textparse/promlex.l`)

Add two new token values:

```go
tOpenStateset  // {{  when the parser state indicates a value position
tCloseStateset // }}
```

The lexer already produces `tBraceOpen` for `{`.  The simplest extension is
to emit `tOpenStateset` when two consecutive `{` appear in the value position
(i.e., after the whitespace following the label set or metric name).

No new tokens are needed for `lname`, `=`, or state names; they reuse
`tText` / `tMName` / `tLName` patterns already present.

### 2. Production parser (`model/textparse/promparse.go`)

In `parseMetricSuffix`, add a branch after the whitespace that follows the
label set:

```
if next token is tOpenStateset:
    parse lname:<name> and zero or more state=0|1 pairs until tCloseStateset
    return EntryStateset with constructed StateSet
```

The state-machine shape mirrors the existing histogram / exemplar branches.

### 3. OpenMetrics parser (`model/textparse/openmetricsparse.go`)

Same change in the equivalent value-parsing function.  The OpenMetrics parser
already handles exemplars and native histograms as non-float values following
the label set; native statesets follow the same pattern.

### 4. `Parser` interface (`model/textparse/interface.go`)

No interface change is required.  `Stateset()` already exists and is called
when `Next()` returns `EntryStateset`.  The new native-format parser branch
just uses a different code path to populate the same return values.

---

## Comparison with the histogram `{{…}}` format

The patterns are deliberately parallel:

| | native histogram | native stateset |
|---|---|---|
| **delimiter** | `{{` … `}}` | `{{` … `}}` |
| **required key** | none (empty `{{}}` is a valid zero histogram) | `lname:<name>` |
| **data payload** | `key:value` pairs (schema, count, sum, buckets) | `state=0\|1` pairs |
| **array values** | `buckets:[v1 v2 …]` | n/a (each state is a separate key) |
| **ordering** | spans and buckets must be in bucket order | state names may appear in any order; parser sorts |
| **omitted keys** | zero values may be omitted (`{{}}` = zero histogram) | inactive states may NOT be omitted — all known states must appear |
| **type annotation** | `# TYPE … histogram` | `# TYPE … stateset` |

**Why state names are not in an array:** Histograms use arrays for buckets
because bucket counts are opaque numbers; context is provided by the schema.
For statesets, each number is either `0` or `1` and the semantics are
immediate: `Running=1` is more readable than `states:[Failed Pending Running
Succeeded Unknown] values:[0 0 1 0 0]`.  The key-value form also makes
partial statesets (e.g., only one state active) scannable at a glance.

---

## Alternatives considered

### A. Separate `states:` and `values:` arrays

```
{{lname:phase states:[Failed Pending Running Succeeded Unknown] values:[0 0 1 0 0]}}
```

**Pro:** closer to histogram's array syntax; explicit separation of schema
from data.  
**Con:** verbose; requires scanning two lists to understand the sample;
`values` is redundant information once `states` is known if only one state can
be active.

### B. Compact bitset

```
{{lname:phase names:[Failed Pending Running Succeeded Unknown] mask:0x04}}
```

**Pro:** extremely compact; `mask` maps directly to `StateSet.Values`.  
**Con:** unreadable without mental bit-arithmetic; loses the self-documenting
property; `0x04` doesn't convey that "Running is active" at first glance.

### C. OM1 float extension with inline hint

Embed a machine-readable hint in a comment line so that OM1-style series can
be grouped server-side without the multi-line aggregation:

```
# NATIVE_STATESET kube_pod_status_phase
kube_pod_status_phase{pod="web-1",kube_pod_status_phase="Running"} 1
…
```

**Pro:** backwards compatible with OM1 parsers.  
**Con:** still N lines per instance; does not reduce series cardinality at the
scrape target; requires the aggregation heuristic the native encoding is meant
to eliminate.

### D. Label-style single-brace syntax

```
kube_pod_status_phase{pod="web-1"} {Failed="0",Pending="0",Running="1"}
```

**Pro:** reuses the label-pair parser.  
**Con:** ambiguous — this looks like a metric with an empty metric name and
unusual labels; `{…}` in the value position is not valid today and adding it
would create a permanent grammar ambiguity around label sets.

---

## Open questions

1. **Quoting**: The proposal requires state names to be valid `MetricName`
   identifiers (`[a-zA-Z_:][a-zA-Z0-9_:]*`).  Should quoted state names (e.g.
   `"my-state"="1"`) be supported to accommodate OM2 names that include
   hyphens or Unicode?

2. **`lname` omission default**: If `lname:` is omitted, should the parser
   default `LabelName` to the metric family name (which is the OM1 convention)?
   Or should omitting `lname:` be a parse error?  The current proposal requires
   `lname:` to be explicit.

3. **`lname` vs `label`**: The keyword `lname` is short but unfamiliar.  The
   alternative `label:<name>` is slightly more self-documenting.  The
   promqltest `ss(labelName, …)` convention uses the term "label name" which
   favours `lname`.

4. **Multiple active states**: OpenMetrics allows multiple states to be active
   simultaneously (e.g., feature flags).  The proposed format handles this
   naturally (`auth=1 caching=1 debug=0`); the `Values` uint64 bitset supports
   up to 64 simultaneously active states.

5. **Scrape negotiation**: Targets that emit native statesets must signal
   capability so that Prometheus 2.x servers (which cannot parse the new syntax)
   do not receive incompatible scrapes.  The existing `application/openmetrics-text`
   content-type version field or a new scrape capability header could be used,
   mirroring the mechanism used for native histograms.

6. **Remote write v2**: The remote write v2 path already encodes native
   statesets in `TimeSeries.XXX_unrecognized` (fields 7 and 8).  The text
   format is only relevant for scrape; no changes to remote write are implied
   by this proposal.

---

## Implementation sketch

```
model/textparse/promlex.l              add tOpenStateset / tCloseStateset
model/textparse/promparse.go           parseMetricSuffix: {{lname:… state=…}} branch
model/textparse/openmetricsparse.go    parseSeriesEndOfLine: same branch
model/textparse/promparse_test.go      round-trip tests for native stateset lines
model/textparse/openmetricsparse_test.go  same
```

No changes to `model/textparse/interface.go`, `statesetparse.go`, or any
downstream TSDB / scrape packages are required: those layers already consume
`EntryStateset` entries produced by `StateSetParser` and they will equally
accept entries produced directly by the production parsers.
