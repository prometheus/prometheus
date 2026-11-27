# Prombench: interpreting results and the ignorable baseline

Prombench runs two Prometheus servers side by side on dedicated GKE nodes,
one built from a pull request (the "PR" side) and one built from a reference
version (the "release" side), puts the same load on both, and graphs the
difference. It is implemented in the
[prometheus/test-infra](https://github.com/prometheus/test-infra) repository
and is triggered from pull requests in this repository with a `/prombench`
comment; release shepherds also run it for release candidates (see
[RELEASE.md](../RELEASE.md)).

This document records what the investigation in
[#18046](https://github.com/prometheus/prometheus/issues/18046) found about
how reliable those numbers are, so that reviewers do not chase regressions
that are not there — and do not wave through regressions that are.

## Verdict

Prombench is reliable enough to catch large regressions and to compare
sustained, profile-corroborated changes. It is **not** precise enough to
treat a few percent of difference on the headline panels as a real effect.
In controlled runs where both sides ran the *identical* Prometheus version,
the two sides still differed by roughly:

| Metric (dashboard average) | Same-version difference observed |
| --- | --- |
| CPU use | up to about ±6%, and about 8% over a multi-day run |
| Allocations/s, allocated objects | about ±3%, with occasional larger transient spikes |
| HTTP / query latency | about ±3–4% |

Treat differences inside these bands as noise unless they are sustained
over the whole run **and** corroborated by profiles (see below). There is
no evidence that waiting a fixed amount of time removes the noise: runs
that looked settled after a day drifted back to a ~6% CPU difference later,
so use the average over the full run rather than a favourable window.

Some panels are noisier than the headline ones and should never be used
alone to accept or block a change:

* `go_sync_mutex_wait_total_seconds_total` differed by up to ~90% between
  two instances of the same version. Mutex-wait, context-switch and other
  scheduler-adjacent panels show contention that appears randomly on one
  side after a restart; suspected sources include the TSDB isolation code
  (`tsdb/isolation.go`), which is unrelated to most changes under test.
* Single-moment profile or panel spikes: queries and appends land at
  different times on the two sides, so only profile data merged over a
  long window is comparable.

## Why the two sides differ

The same-version difference is not one bug; it is the sum of several
asymmetries and sources of variance found during the investigation:

* The PR side runs a source-built binary from an `emptyDir`
  inside a busybox image while the release side runs the official image, so
  the binaries can also differ by Go patch version, and a `/prombench main`
  run can silently compare against different commits than intended.
  Building both sides from source was proposed in
  [test-infra#940](https://github.com/prometheus/test-infra/pull/940) to
  remove those differences; until that or an equivalent is in place, check
  which commits and Go versions were actually compared before trusting a
  small delta.
* The benchmark nodes use `n1-highmem-8` without a pinned CPU platform, so
  the two VMs can land on different CPU generations (Sandy Bridge through
  Skylake). Check `node_cpu_info` (model name, family, stepping) for both
  nodes before attributing a CPU delta to code; the `cpu.info`
  node-exporter collector was enabled for exactly this check in
  [test-infra#1047](https://github.com/prometheus/test-infra/pull/1047).
* Whole-node load is asymmetric: unrelated pods on the PR node also
  burned more CPU than their counterparts on the release node, and
  `containerd` on the PR node burned a median of about 6.5x more CPU
  across all sampled runs (in one 72-hour run, a constant ~0.27 cores
  more). A constant extra load on one node produces exactly the small,
  load-independent, always-same-direction slowdown often seen on the PR
  side. The mechanism is still unknown and needs node-level profiling.
* Node-exporter placement could swap the two nodes' host metrics: its
  pod affinity used a topology key shared by both benchmark nodes, so in
  about half of historical runs node-level panels (CPU, context switches,
  memory) for the two sides were exchanged. Fixed by keying affinity on
  `kubernetes.io/hostname` in
  [test-infra#1046](https://github.com/prometheus/test-infra/pull/1046);
  distrust node-level panels from runs started before that fix.
* Scrape and rule evaluation phasing may differ per start: one hypothesis
  from the investigation is that targets and rules that happen to be
  evaluated close together contend for the same TSDB locks, so one
  instance can develop higher lock contention than the other with
  identical code and load. This is not confirmed.

None of these are fixed by this document; they are recorded here so the
remaining variance is interpreted correctly. Reliability work continues in
the test-infra repository.

## How to read a run

Before drawing any conclusion from a prombench run:

1. Confirm the load was equal: samples/s, active series and query load
   must match on both sides. If they do not, the run says nothing about
   performance. (The dashboard's load-generator panel uses a 61s step to
   avoid aliasing between the load generator and metric sampling.)
2. Confirm the comparison was fair: both sides built from the intended
   commits, and `node_cpu_info` shows the same CPU model on both nodes.
   If the CPU models differ, a CPU delta of a few percent is expected and
   ignorable.
3. Compare averages over the full run, not a selected window. Even a
   day-long run can drift, so small effects need multi-day runs; short
   runs are only useful for catching gross regressions.
4. Apply the baseline table above: inside the band, record the run as
   "within prombench noise" and move on; do not open follow-up issues for
   it. Outside the band, or when a within-band delta is suspicious because
   it matches the change under test, verify with profiles merged over
   time (for example Parca diffs) and check that the extra CPU is in code
   the change actually touches. Profile differences in unrelated code
   (HTTP API, remote write, GC, mutex waits) are the known noise signature.
5. If the result is ambiguous and the decision matters, re-run, ideally
   with the reference benchmarked against itself to measure the noise
   floor of that particular run. Expect that control run to be non-zero;
   compare the pull request's delta against it, not against zero.

For release acceptance, the existing rule stands: run the release
candidate for 3 days against the previous stable release. A candidate
whose headline deltas stay within the baseline above for the full run, with
equal load and matching CPU models, has shown no regression as far as
prombench can tell. A sustained delta beyond the baseline requires a
profile-backed explanation before the release is promoted or blocked on
performance grounds.

## What would make this better

* Pin or verify equivalent CPU platforms for the two benchmark nodes and
  surface a warning on the dashboard when they differ.
* Show the same-version noise band on the dashboard (for example by
  rendering acceptance numbers neutral when within the baseline) so the
  ignorable range is visible where the numbers are read.
* Exercise the reference twice in a run to measure that run's own variance,
  at the cost of more benchmark resources.
* Get node access during a run to profile why `containerd` burns several
  times more CPU on the PR node.

Until then, this baseline is the reference for deciding whether a
prombench difference needs action.
