# Native Stateset vs Float-Gauge: Memory Measurement Findings

**Date:** 2026-04-29  
**Branch:** native-stateset-representation  
**Hardware:** Intel Core Ultra 7 265H  
**Go version:** see go.mod

## Setup

The comparison models `kube_pod_status_phase`, a real-world OpenMetrics stateset with 5 states:
`Failed`, `Pending`, `Running`, `Succeeded`, `Unknown`.

**Legacy (float gauges):** 5 separate float gauge series per pod, one per state.
Each series carries the labels `{__name__, namespace, pod, phase}`.
Only the "Running" series has value `1`; the other four have value `0`.

**Native stateset:** 1 stateset series per pod.
Labels: `{__name__, namespace, pod}`.
State is encoded as a bitset (`Values=0b00100`, "Running" active).

Scale: **2,000 pods**, **60 samples per series** (1 hour at 1-minute scrape intervals).

## How Memory Was Measured

Each scenario runs in its own process so the kernel's `VmHWM` (high-water mark)
starts clean. Two measurements are taken:

1. **HeapInuse** — Go heap bytes in use, read from `runtime.ReadMemStats` after
   two `runtime.GC()` calls. Reflects live Go objects: `MemSeries` structs,
   chunk data, labels, postings index.

2. **VmHWM / VmRSS** — read from `/proc/self/status`. Includes everything the
   kernel counts as resident: Go heap, stack, goroutine stacks, mmap'd WAL
   segments, mmap'd head chunk files.

3. **External peak VmRSS** — polled from `/proc/{pid}/status` every 50 ms by
   the runner script while the test process is running. Equivalent to what
   `systemd-run --wait` reports as `MemoryPeak` for the transient service.

The test is `tsdb.TestStatesetMemory`; the runner script is
`scripts/stateset_memory.sh`.

## Results

| Representation | Series | Samples iterated | HeapInuse (after GC) | VmHWM | External peak VmRSS |
|---|---|---|---|---|---|
| `native_stateset` | 2,000 | 120,000 | **16.3 MiB** | **77.9 MiB** | **70 MiB** |
| `float_gauges` | 10,000 | 600,000 | 154.7 MiB | 234.9 MiB | 258 MiB |
| **ratio** | 5× | 5× | **9.5×** | **3.0×** | **3.7×** |

### Interpretation

**Why 9.5× heap instead of the expected 5×**

The test head uses `chunkRange = 10,000 ms` (10 s).
Samples arrive at 60,000 ms intervals (1 m).
Because each new sample's timestamp exceeds the current chunk's `nextAt`,
every sample starts a fresh head chunk.
After 60 samples, each series has accumulated 60 `memChunk` + `XORChunk`
struct pairs in the Go heap in addition to the `MemSeries`, `labels.Labels`,
and postings index entries.

Float gauges have 5× more series, so they accumulate 5× more of each
structure. The combined effect (5× series × ~2× chunk overhead relative to
data size) pushes the heap ratio past 5×.

**Why VmHWM ratio is only 3×**

A significant fraction of RSS is the mmap'd WAL (shared between both tests)
and Go runtime overhead (stacks, finalizer queues, etc.) that does not scale
with series count. The mmap'd head chunk files also add RSS without adding
heap. These fixed costs dilute the ratio when comparing total RSS.

**Query sample count**

The float-gauge query iterates 600,000 samples (5 values per pod per
timestamp); the stateset query iterates 120,000 (1 bitset per pod per
timestamp). The information content is identical — for 2,000 pods × 60
timestamps, each query knows exactly which state is active for each pod.
Native stateset delivers this in 5× fewer iterator steps.

## Reproduce

```bash
# Build once
go test -c -o /tmp/tsdb.test ./tsdb/

# Native stateset scenario
/tmp/tsdb.test -test.run='^TestStatesetMemory/native_stateset$' -test.v

# Float-gauge scenario
/tmp/tsdb.test -test.run='^TestStatesetMemory/float_gauges$' -test.v

# Or use the script (runs both with external RSS monitoring)
bash scripts/stateset_memory.sh
```

When `systemd-run` is available the equivalent wrapper is:

```bash
systemd-run -t --user --wait \
  --slice=prometheus-memory.slice \
  --property=MemoryAccounting=yes \
  --same-dir \
  /tmp/tsdb.test -test.run='^TestStatesetMemory/native_stateset$' -test.v
```

`MemoryPeak` in the unit's accounting output corresponds to the `VmHWM`
column above.

## Conclusion

For a 5-state metric family at 2,000 instances native stateset reduces:

- **Go heap in use by 9.5×** (16 MiB vs 155 MiB)
- **Peak process RSS by 3×** (78 MiB vs 235 MiB)
- **Query iterator steps by 5×** (120k vs 600k samples)

The heap saving grows with scrape interval relative to `chunkRange` and with
the number of states per metric family.
