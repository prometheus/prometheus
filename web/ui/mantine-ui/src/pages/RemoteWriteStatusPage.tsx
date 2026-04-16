import { Alert, Badge, Group, Table, Text } from "@mantine/core";
import { IconInfoCircle, IconUpload } from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../api/api";
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
const queueKey = (m: ProtoMetric): string =>
  `${labelValue(m, "remote_name")}|${labelValue(m, "url")}`;

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
      const key = queueKey(m);
      if (!queues.has(key)) {
        queues.set(key, {
          name: labelValue(m, "remote_name"),
          endpoint: labelValue(m, "url"),
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

const QueueCard = ({ queue }: { queue: QueueData }) => {
  const delay = queue.highestTimestampSec - queue.highestSentTimestampSec;
  const totalFailed =
    queue.samplesFailed + queue.exemplarsFailed + queue.histogramsFailed;
  const totalPending =
    queue.pendingSamples + queue.pendingExemplars + queue.pendingHistograms;
  const isHealthy = totalFailed === 0 && delay < 600;

  return (
    <InfoPageCard title={queue.name} icon={IconUpload}>
      <Group mb="sm" ml="xs" gap="xs">
        <Badge color={isHealthy ? "green" : "red"} variant="filled" size="sm">
          {isHealthy ? "Healthy" : "Degraded"}
        </Badge>
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
                <Badge color="yellow" variant="light" size="sm" ml="xs">
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

export default function RemoteWriteStatusPage() {
  const {
    data: { data: families },
  } = useSuspenseAPIQuery<SelfMetricsResult>({
    path: `/status/self_metrics`,
    params: {
      metric_name_pattern: "prometheus_remote_storage_.*",
    },
  });

  const queues = buildQueues(families);

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
        <QueueCard key={`${queue.name}-${queue.endpoint}`} queue={queue} />
      ))}
    </InfoPageStack>
  );
}
