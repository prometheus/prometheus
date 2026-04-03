import { Alert, Badge, Group, Table, Text } from "@mantine/core";
import { IconInfoCircle, IconUpload } from "@tabler/icons-react";
import { useSuspenseAPIQuery } from "../api/api";
import {
  RemoteWriteStatus,
  QueueStatus,
} from "../api/responseTypes/remoteWrite";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
};

const formatDelay = (seconds: number): string => {
  if (seconds < 0 || !isFinite(seconds)) return "N/A";
  if (seconds < 60) return `${seconds.toFixed(0)}s`;
  if (seconds < 3600) return `${(seconds / 60).toFixed(1)}m`;
  return `${(seconds / 3600).toFixed(1)}h`;
};

const QueueCard = ({ queue }: { queue: QueueStatus }) => {
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
    data: {
      data: { queues },
    },
  } = useSuspenseAPIQuery<RemoteWriteStatus>({
    path: `/status/remote_write`,
  });

  if (!queues || queues.length === 0) {
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
