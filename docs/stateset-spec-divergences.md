# Native StateSet — OpenMetrics Spec Divergences

## Spec locations

| Version | Canonical URL | Notes |
|---|---|---|
| OpenMetrics 1.0 | https://prometheus.io/docs/specs/om/open_metrics_spec/ | Rendered from `github.com/prometheus/docs`; IETF draft `draft-ietf-opsawg-openmetrics-00`. StateSet data model: `#stateset`; text format: `#stateset-1`. |
| OpenMetrics 2.0 RC0 | https://prometheus.io/docs/specs/om/open_metrics_spec_2_0/ | `2.0.0-rc0`, dated March 2026. Working document; referenced from `github.com/prometheus/OpenMetrics` issue #276 "[meta] OpenMetrics 2.0". |
| OM issue #310 | https://github.com/prometheus/OpenMetrics/issues/310 | "OM 2.0: Should stateset be represented in a Complex/Composite format?" — records working group decision to keep one-sample-per-state for RC0. |

## Relevant spec rules

### OpenMetrics 1.0

Data model (`#stateset`):

> A StateSet Metric's LabelSet MUST NOT have a label name which is the same as the
> name of its MetricFamily.

> If encoded as a StateSet, ENUMs MUST have exactly one Boolean which is true
> within a MetricPoint.

> MetricFamilies of type StateSets MUST have an empty Unit string.

Text format (`#stateset-1`):

> StateSets MUST have one sample per State in the MetricPoint. Each State's sample
> MUST have a label with the MetricFamily name as the label name and the State name
> as the label value. The State sample's value MUST be 1 if the State is true and
> MUST be 0 if the State is false.

### OpenMetrics 2.0 RC0 changes

- The data-model MUST NOT ("label name must not equal MetricFamily name") is **removed**.
  Issue #310 notes that real-world exporters (kube-state-metrics and others) routinely
  violate it; RC0 drops it rather than requiring widespread breakage.
- The positive text-format rule (state label name = MetricFamily name) is **retained**.
- Adds: StateSets MUST NOT be interleaved across dimension groups.
- Drops protobuf format specification entirely.
- No compact/composite wire encoding introduced (issue #310 deferred to a future
  "StateSet2" type or post-RC0 revision).

## Divergences

### 1. State label name may differ from metric family name (native format)

**Affected spec:** OM 1.0 (text-format MUST); OM 2.0 RC0 (same positive rule retained).

**Rule:** Each per-state sample MUST carry a label whose name equals the MetricFamily name.

**Divergence:** The native Prometheus wire extension `{{lname: state=0|1 …}}` lets the
scrape target specify an arbitrary label name in the `lname` field, independent of the
metric family name. Example:

```
# TYPE kube_pod_status_phase stateset
kube_pod_status_phase{pod="p1"} {{phase: Failed=0 Running=1}}
```

Here `phase ≠ kube_pod_status_phase`. This is intentional: it mirrors real-world usage
where the enum label is conventionally shorter than the metric name (kube-state-metrics
uses `phase`, `condition`, `reason`, etc.).

The OM1 aggregation path (`StateSetParser` in `model/textparse/statesetparse.go`) does
enforce the spec: it looks up `p.lset.Get(metricName)` and silently ignores series that
lack a label named after the metric family. The native path passes `LabelName` through
verbatim from the `{{…}}` body.

**Observable signal:** Scrape counter
`prometheus_target_scrapes_stateset_noncanonical_label_name_total` increments once per
native stateset sample where `ss.LabelName != metricFamilyName`.
See `scrape/metrics.go` and `scrape/scrape_append_v2.go`.

### 2. Unit MUST be empty — not validated

**Affected spec:** OM 1.0 only (OM 2.0 status unclear; not explicitly retained in RC0 text reviewed).

**Rule:** MetricFamilies of type StateSets MUST have an empty Unit string.

**Divergence:** Neither the scrape pipeline nor the ingestion path validates this.
A target can expose `# UNIT foo states` alongside `# TYPE foo stateset` without error.

### 3. ENUM one-hot constraint — not enforced

**Affected spec:** OM 1.0 only.

**Rule:** "If encoded as a StateSet, ENUMs MUST have exactly one Boolean which is true."

**Divergence:** Multiple simultaneously active states, or zero active states, are accepted
without error or warning at scrape time, ingestion time, or query time. This is
intentional for the extension: native statesets are not restricted to enum semantics.

### 4. Interleaving — silently tolerated, not rejected

**Affected spec:** OM 2.0 RC0 ("StateSets MUST NOT be interleaved").

**Divergence:** `StateSetParser` flushes the current accumulation on a dimension-group
hash change and starts a new group. Interleaved series are therefore silently split into
separate stateset samples rather than producing a parse error. OM 1.0 does not have an
explicit interleaving rule.

### 5. Compact wire format is a Prometheus-specific extension

**Affected spec:** Both OM 1.0 and 2.0 RC0.

**Rule:** Neither spec defines or reserves a `{{lname: …}}` composite value syntax.

**Divergence:** The `{{lname: state=0|1 …}}` format is a Prometheus-specific extension
to both the Prometheus text format and OpenMetrics text format. It is not part of either
spec and is not covered by the Prometheus HTTP API OpenAPI specification. It is gated
behind `--enable-feature=native-statesets`.
