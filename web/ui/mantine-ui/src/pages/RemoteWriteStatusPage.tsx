import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Badge, Group, Table, Text } from "@mantine/core";
import { IconInfoCircle, IconUpload } from "@tabler/icons-react";
import { useAPIQuery } from "../api/api";
import {
  SelfMetricsResult,
  ProtoMetricFamily,
  ProtoMetric,
} from "../api/responseTypes/selfMetrics";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";

// Extract the numeric value from a ProtoMetric (gauge or counter).
const metricValue = (m: ProtoMetric): number => {
  if (m.gauge) {
    return m.gauge.value;
  }
  if (m.counter) {
    return m.counter.value;
  }
  return 0;
};

// Return the label value for a given label name, or "" if absent.
const labelValue = (m: ProtoMetric, name: string): string =>
  m.label?.find((l) => l.name === name)?.value ?? "";

// A composite key that identifies a single remote write queue.
const queueKey = (remoteName: string, url: string): string =>
  `${remoteName}|${url}`;

interface QueueData {
  name: string;
  endpoint: string;

  highestTimestampSec: number;
  highestSentTimestampSec: number;

  pendingSamples: number;
  pendingExemplars: number;
  pendingHistograms: number;

  currentShards: number;
  desiredShards: number;
  minShards: number;
  maxShards: number;
  shardCapacity: number;

  samplesSent: number;
  exemplarsSent: number;
  histogramsSent: number;
  metadataSent: number;
  bytesSent: number;

  samplesFailed: number;
  exemplarsFailed: number;
  histogramsFailed: number;
  metadataFailed: number;

  samplesRetried: number;
  exemplarsRetried: number;
  histogramsRetried: number;
  metadataRetried: number;
}

// Map from metric suffix (after "prometheus_remote_storage_") to the
// QueueData field it populates.
const metricFieldMap: Record<string, keyof QueueData> = {
  queue_highest_timestamp_seconds: "highestTimestampSec",
  queue_highest_sent_timestamp_seconds: "highestSentTimestampSec",
  samples_pending: "pendingSamples",
  exemplars_pending: "pendingExemplars",
  histograms_pending: "pendingHistograms",
  shards: "currentShards",
  shards_desired: "desiredShards",
  shards_min: "minShards",
  shards_max: "maxShards",
  shard_capacity: "shardCapacity",
  samples_total: "samplesSent",
  exemplars_total: "exemplarsSent",
  histograms_total: "histogramsSent",
  metadata_total: "metadataSent",
  bytes_total: "bytesSent",
  samples_failed_total: "samplesFailed",
  exemplars_failed_total: "exemplarsFailed",
  histograms_failed_total: "histogramsFailed",
  metadata_failed_total: "metadataFailed",
  samples_retried_total: "samplesRetried",
  exemplars_retried_total: "exemplarsRetried",
  histograms_retried_total: "histogramsRetried",
  metadata_retried_total: "metadataRetried",
};

const METRIC_PREFIX = "prometheus_remote_storage_";

// Counter fields used for health checks: if any of these have a nonzero
// rate, the queue is considered degraded.
const HEALTH_COUNTER_FIELDS: (keyof QueueData)[] = [
  "samplesFailed",
  "exemplarsFailed",
  "histogramsFailed",
  "samplesRetried",
  "exemplarsRetried",
  "histogramsRetried",
];

// Snapshot of counter values and the time they were observed, used for
// computing rates between two polls.
interface CounterSnapshot {
  ts: number; // Date.now() when observed
  counters: Map<string, QueueData>; // keyed by queueKey
}

// Computed error/retry rate per queue, derived from two successive snapshots.
type HealthRates = Map<string, number | "calculating">;

const buildQueues = (families: ProtoMetricFamily[]): QueueData[] => {
  const queues = new Map<string, QueueData>();

  for (const family of families) {
    if (!family.name.startsWith(METRIC_PREFIX)) {
      continue;
    }
    const suffix = family.name.slice(METRIC_PREFIX.length);
    const field = metricFieldMap[suffix];
    if (!field) {
      continue;
    }

    for (const m of family.metric) {
      const name = labelValue(m, "remote_name");
      const url = labelValue(m, "url");
      const key = queueKey(name, url);
      if (!queues.has(key)) {
        queues.set(key, {
          name,
          endpoint: url,
          highestTimestampSec: 0,
          highestSentTimestampSec: 0,
          pendingSamples: 0,
          pendingExemplars: 0,
          pendingHistograms: 0,
          currentShards: 0,
          desiredShards: 0,
          minShards: 0,
          maxShards: 0,
          shardCapacity: 0,
          samplesSent: 0,
          exemplarsSent: 0,
          histogramsSent: 0,
          metadataSent: 0,
          bytesSent: 0,
          samplesFailed: 0,
          exemplarsFailed: 0,
          histogramsFailed: 0,
          metadataFailed: 0,
          samplesRetried: 0,
          exemplarsRetried: 0,
          histogramsRetried: 0,
          metadataRetried: 0,
        });
      }
      // All mapped fields are numeric; the cast is safe.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (queues.get(key) as any)[field] = metricValue(m);
    }
  }

  return Array.from(queues.values());
};

const formatBytes = (bytes: number): string => {
  if (bytes === 0) {
    return "0 B";
  }
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
};

const formatDelay = (seconds: number): string => {
  if (seconds < 0 || !isFinite(seconds)) {
    return "N/A";
  }
  if (seconds < 60) {
    return `${seconds.toFixed(0)}s`;
  }
  if (seconds < 3600) {
    return `${(seconds / 60).toFixed(1)}m`;
  }
  return `${(seconds / 3600).toFixed(1)}h`;
};

// Compute per-queue error/retry rates from two successive counter snapshots.
// Returns "calculating" for queues that only appear in the current snapshot
// (i.e. we don't have a baseline yet).
const computeHealthRates = (
  baseline: CounterSnapshot,
  current: CounterSnapshot,
): HealthRates => {
  const rates: HealthRates = new Map();
  const dt = (current.ts - baseline.ts) / 1000; // seconds
  if (dt <= 0) {
    for (const key of current.counters.keys()) {
      rates.set(key, "calculating");
    }
    return rates;
  }

  for (const [key, cur] of current.counters) {
    const base = baseline.counters.get(key);
    if (!base) {
      rates.set(key, "calculating");
      continue;
    }

    let totalRate = 0;
    for (const field of HEALTH_COUNTER_FIELDS) {
      const curVal = cur[field] as number;
      const baseVal = base[field] as number;
      // Counter going backwards means a restart — treat delta as zero
      // (the baseline will be reset by the caller).
      const delta = curVal >= baseVal ? curVal - baseVal : 0;
      totalRate += delta / dt;
    }
    rates.set(key, totalRate);
  }

  return rates;
};

// Detect whether any counter went backwards (Prometheus restart).
const detectCounterReset = (
  baseline: CounterSnapshot,
  current: CounterSnapshot,
): boolean => {
  for (const [key, cur] of current.counters) {
    const base = baseline.counters.get(key);
    if (!base) {
      continue;
    }
    for (const field of HEALTH_COUNTER_FIELDS) {
      if ((cur[field] as number) < (base[field] as number)) {
        return true;
      }
    }
  }
  return false;
};

interface QueueCardProps {
  queue: QueueData;
  healthRate: number | "calculating" | undefined;
}

const QueueCard = ({ queue, healthRate }: QueueCardProps) => {
  const delay = queue.highestTimestampSec - queue.highestSentTimestampSec;
  const totalPending =
    queue.pendingSamples + queue.pendingExemplars + queue.pendingHistograms;

  const isCalculating =
    healthRate === undefined || healthRate === "calculating";
  const isHealthy = isCalculating ? undefined : healthRate === 0 && delay < 600;

  return (
    <InfoPageCard title={queue.name} icon={IconUpload}>
      <Group mb="sm" ml="xs" gap="xs">
        {isCalculating ? (
          <Badge color="gray" variant="filled" size="sm">
            Calculating...
          </Badge>
        ) : (
          <Badge
            color={isHealthy ? "green" : "red"}
            variant="filled"
            size="sm"
          >
            {isHealthy ? "Healthy" : "Degraded"}
          </Badge>
        )}
        <Text size="sm" c="dimmed">
          {queue.endpoint}
        </Text>
      </Group>

      <Table layout="fixed">
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Metric</Table.Th>
            <Table.Th>Value</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          <Table.Tr>
            <Table.Td>Delay</Table.Td>
            <Table.Td>
              <Badge
                color={delay < 60 ? "green" : delay < 600 ? "yellow" : "red"}
                variant="light"
                size="sm"
                styles={{ label: { textTransform: "none" } }}
              >
                {formatDelay(delay)}
              </Badge>
            </Table.Td>
          </Table.Tr>

          <Table.Tr>
            <Table.Td>Shards (current / desired / min / max)</Table.Td>
            <Table.Td>
              <code>
                {queue.currentShards} / {queue.desiredShards} /{" "}
                {queue.minShards} / {queue.maxShards}
              </code>
            </Table.Td>
          </Table.Tr>
          <Table.Tr>
            <Table.Td>Shard capacity</Table.Td>
            <Table.Td>
              <code>{queue.shardCapacity}</code>
            </Table.Td>
          </Table.Tr>

          <Table.Tr>
            <Table.Td>Pending (samples / exemplars / histograms)</Table.Td>
            <Table.Td>
              <code>
                {queue.pendingSamples} / {queue.pendingExemplars} /{" "}
                {queue.pendingHistograms}
              </code>
              {totalPending > 0 && (
                <Badge
                  color="yellow"
                  variant="light"
                  size="sm"
                  ml="xs"
                  styles={{ label: { textTransform: "none" } }}
                >
                  {totalPending} total
                </Badge>
              )}
            </Table.Td>
          </Table.Tr>

          <Table.Tr>
            <Table.Td>Samples sent / failed / retried</Table.Td>
            <Table.Td>
              <code>
                {queue.samplesSent} / {queue.samplesFailed} /{" "}
                {queue.samplesRetried}
              </code>
            </Table.Td>
          </Table.Tr>
          <Table.Tr>
            <Table.Td>Exemplars sent / failed / retried</Table.Td>
            <Table.Td>
              <code>
                {queue.exemplarsSent} / {queue.exemplarsFailed} /{" "}
                {queue.exemplarsRetried}
              </code>
            </Table.Td>
          </Table.Tr>
          <Table.Tr>
            <Table.Td>Histograms sent / failed / retried</Table.Td>
            <Table.Td>
              <code>
                {queue.histogramsSent} / {queue.histogramsFailed} /{" "}
                {queue.histogramsRetried}
              </code>
            </Table.Td>
          </Table.Tr>
          <Table.Tr>
            <Table.Td>Metadata sent / failed / retried</Table.Td>
            <Table.Td>
              <code>
                {queue.metadataSent} / {queue.metadataFailed} /{" "}
                {queue.metadataRetried}
              </code>
            </Table.Td>
          </Table.Tr>
          <Table.Tr>
            <Table.Td>Bytes sent</Table.Td>
            <Table.Td>
              <code>{formatBytes(queue.bytesSent)}</code>
            </Table.Td>
          </Table.Tr>
        </Table.Tbody>
      </Table>
    </InfoPageCard>
  );
};

const POLL_INTERVAL = 10_000; // 10 seconds

export default function RemoteWriteStatusPage() {
  const baselineRef = useRef<CounterSnapshot | null>(null);
  const [healthRates, setHealthRates] = useState<HealthRates>(new Map());
  const [tabVisible, setTabVisible] = useState(!document.hidden);

  // Pause polling when the tab is hidden.
  useEffect(() => {
    const handler = () => setTabVisible(!document.hidden);
    document.addEventListener("visibilitychange", handler);
    return () => document.removeEventListener("visibilitychange", handler);
  }, []);

  // Poll self_metrics for remote_storage counters + gauges.
  const { data } = useAPIQuery<SelfMetricsResult>({
    path: `/status/self_metrics`,
    params: {
      metric_name_pattern: "prometheus_remote_storage_.*",
    },
    refetchInterval: tabVisible ? POLL_INTERVAL : false,
  });

  const families = data?.data;
  const queues = families ? buildQueues(families) : [];

  // Build a snapshot map from the current poll result.
  const snapshotFromQueues = useCallback(
    (qs: QueueData[]): CounterSnapshot => {
      const counters = new Map<string, QueueData>();
      for (const q of qs) {
        counters.set(queueKey(q.name, q.endpoint), q);
      }
      return { ts: Date.now(), counters };
    },
    [],
  );

  // Update baseline and health rates whenever new data arrives.
  useEffect(() => {
    if (queues.length === 0) {
      return;
    }

    const current = snapshotFromQueues(queues);

    if (!baselineRef.current) {
      // First sample: set baseline, mark all queues as "calculating".
      baselineRef.current = current;
      const rates: HealthRates = new Map();
      for (const key of current.counters.keys()) {
        rates.set(key, "calculating");
      }
      setHealthRates(rates);
      return;
    }

    // Counter reset detection: if any counter went backwards, reset baseline.
    if (detectCounterReset(baselineRef.current, current)) {
      baselineRef.current = current;
      const rates: HealthRates = new Map();
      for (const key of current.counters.keys()) {
        rates.set(key, "calculating");
      }
      setHealthRates(rates);
      return;
    }

    // Compute rates from baseline to current.
    setHealthRates(computeHealthRates(baselineRef.current, current));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  if (!families) {
    return (
      <InfoPageStack>
        <Alert icon={<IconInfoCircle />} title="Loading..." color="gray">
          Fetching remote write metrics...
        </Alert>
      </InfoPageStack>
    );
  }

  if (queues.length === 0) {
    return (
      <InfoPageStack>
        <Alert
          icon={<IconInfoCircle />}
          title="No remote write endpoints configured"
          color="gray"
        >
          Configure remote write endpoints in your Prometheus configuration to
          see their status here.
        </Alert>
      </InfoPageStack>
    );
  }

  return (
    <InfoPageStack>
      {queues.map((queue) => (
        <QueueCard
          key={`${queue.name}-${queue.endpoint}`}
          queue={queue}
          healthRate={healthRates.get(queueKey(queue.name, queue.endpoint))}
        />
      ))}
    </InfoPageStack>
  );
}
