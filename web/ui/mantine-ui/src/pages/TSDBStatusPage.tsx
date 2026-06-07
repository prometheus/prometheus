import { useState } from "react";
import {
  Alert,
  Button,
  Group,
  Stack,
  Table,
  Text,
  Textarea,
} from "@mantine/core";
import { DateTimePicker } from "@mantine/dates";
import { IconAlertTriangle, IconCheck, IconTrash } from "@tabler/icons-react";
import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import { useSuspenseAPIQuery, API_PATH } from "../api/api";
import { TSDBStatusResult } from "../api/responseTypes/tsdbStatus";
import { formatTimestamp } from "../lib/formatTime";
import { useSettings } from "../state/settingsSlice";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";

dayjs.extend(utc);

export default function TSDBStatusPage() {
  const {
    data: {
      data: {
        headStats,
        labelValueCountByLabelName,
        seriesCountByMetricName,
        memoryInBytesByLabelName,
        seriesCountByLabelValuePair,
      },
    },
  } = useSuspenseAPIQuery<TSDBStatusResult>({ path: `/status/tsdb` });

  const { useLocalTime, pathPrefix } = useSettings();

  const unixToTime = (unix: number): string => {
    const formatted = formatTimestamp(unix, useLocalTime);
    if (formatted === "Invalid Date") {
      if (numSeries === 0) {
        return "No datapoints yet";
      }
      return `Error parsing time (${unix})`;
    }

    return formatted;
  };

  const { chunkCount, numSeries, numLabelPairs, minTime, maxTime } = headStats;
  const stats = [
    { name: "Number of Series", value: numSeries },
    { name: "Number of Chunks", value: chunkCount },
    { name: "Number of Label Pairs", value: numLabelPairs },
    { name: "Current Min Time", value: `${unixToTime(minTime / 1000)}` },
    { name: "Current Max Time", value: `${unixToTime(maxTime / 1000)}` },
  ];

  // Delete series state
  const [matchers, setMatchers] = useState("");
  const [startTime, setStartTime] = useState<number | null>(null);
  const [endTime, setEndTime] = useState<number | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [deleteSuccess, setDeleteSuccess] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  // Clean tombstones state
  const [cleanError, setCleanError] = useState<string | null>(null);
  const [cleanSuccess, setCleanSuccess] = useState<string | null>(null);
  const [cleaning, setCleaning] = useState(false);

  const handleDelete = async () => {
    setDeleteError(null);
    setDeleteSuccess(null);

    const matchList = matchers
      .split("\n")
      .map((m) => m.trim())
      .filter((m) => m !== "");

    if (matchList.length === 0) {
      setDeleteError("Provide at least one match[] selector.");
      return;
    }

    const params = new URLSearchParams();
    for (const m of matchList) {
      params.append("match[]", m);
    }
    if (startTime !== null) {
      params.append("start", (startTime / 1000).toString());
    }
    if (endTime !== null) {
      params.append("end", (endTime / 1000).toString());
    }

    setDeleting(true);
    try {
      const res = await fetch(
        `${pathPrefix}/${API_PATH}/admin/tsdb/delete_series?${params.toString()}`,
        {
          method: "POST",
          credentials: "same-origin",
        }
      );

      if (!res.ok) {
        if (res.headers.get("content-type")?.startsWith("application/json")) {
          const body = await res.json();
          throw new Error(body.error || res.statusText);
        }
        throw new Error(res.statusText);
      }

      setDeleteSuccess(
        `Successfully deleted series matching: ${matchList.join(", ")}`
      );
      setMatchers("");
      setStartTime(null);
      setEndTime(null);
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setDeleting(false);
    }
  };

  const handleCleanTombstones = async () => {
    setCleanError(null);
    setCleanSuccess(null);
    setCleaning(true);

    try {
      const res = await fetch(
        `${pathPrefix}/${API_PATH}/admin/tsdb/clean_tombstones`,
        {
          method: "POST",
          credentials: "same-origin",
        }
      );

      if (!res.ok) {
        if (res.headers.get("content-type")?.startsWith("application/json")) {
          const body = await res.json();
          throw new Error(body.error || res.statusText);
        }
        throw new Error(res.statusText);
      }

      setCleanSuccess("Tombstones cleaned successfully.");
    } catch (err) {
      setCleanError(err instanceof Error ? err.message : "Unknown error");
    } finally {
      setCleaning(false);
    }
  };

  return (
    <InfoPageStack>
      {[
        {
          title: "TSDB Head Status",
          stats,
          formatAsCode: false,
        },
        {
          title: "Top 10 label names with value count",
          stats: labelValueCountByLabelName,
          formatAsCode: true,
        },
        {
          title: "Top 10 series count by metric names",
          stats: seriesCountByMetricName,
          formatAsCode: true,
        },
        {
          title: "Top 10 label names with high memory usage",
          unit: "Bytes",
          stats: memoryInBytesByLabelName,
          formatAsCode: true,
        },
        {
          title: "Top 10 series count by label value pairs",
          stats: seriesCountByLabelValuePair,
          formatAsCode: true,
        },
      ].map(({ title, unit = "Count", stats, formatAsCode }) => (
        <InfoPageCard title={title}>
          <Table layout="fixed">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>{unit}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {stats.map(({ name, value }) => (
                <Table.Tr key={name}>
                  <Table.Td
                    style={{
                      wordBreak: "break-all",
                    }}
                  >
                    {formatAsCode ? <code>{name}</code> : name}
                  </Table.Td>
                  <Table.Td>{value}</Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </InfoPageCard>
      ))}

      <InfoPageCard title="Delete Series" icon={IconTrash}>
        <Stack gap="md">
          <Alert
            icon={<IconAlertTriangle size={16} />}
            color="yellow"
            title="Warning"
          >
            This operation marks matching series for deletion. Deleted data
            cannot be recovered. Use "Clean Tombstones" afterwards to reclaim
            disk space.
          </Alert>

          {deleteError && (
            <Alert
              color="red"
              title="Error"
              withCloseButton
              onClose={() => setDeleteError(null)}
            >
              {deleteError}
            </Alert>
          )}

          {deleteSuccess && (
            <Alert
              icon={<IconCheck size={16} />}
              color="green"
              title="Success"
              withCloseButton
              onClose={() => setDeleteSuccess(null)}
            >
              {deleteSuccess}
            </Alert>
          )}

          <Textarea
            label="Series selector(s)"
            description='PromQL series selectors, one per line. Example: up{job="prometheus"}'
            placeholder={'up{job="prometheus"}'}
            value={matchers}
            onChange={(e) => setMatchers(e.currentTarget.value)}
            autosize
            minRows={2}
            maxRows={6}
          />

          <Group grow>
            <DateTimePicker
              label="Start time"
              description="Optional"
              placeholder="Select start time"
              valueFormat="YYYY-MM-DD HH:mm:ss"
              withSeconds
              value={
                startTime !== null
                  ? useLocalTime
                    ? dayjs(startTime).format()
                    : dayjs(startTime)
                        .subtract(dayjs().utcOffset(), "minutes")
                        .format()
                  : undefined
              }
              onChange={(value) =>
                setStartTime(
                  value
                    ? useLocalTime
                      ? new Date(value).getTime()
                      : dayjs.utc(value).valueOf()
                    : null
                )
              }
              clearable
            />
            <DateTimePicker
              label="End time"
              description="Optional"
              placeholder="Select end time"
              valueFormat="YYYY-MM-DD HH:mm:ss"
              withSeconds
              value={
                endTime !== null
                  ? useLocalTime
                    ? dayjs(endTime).format()
                    : dayjs(endTime)
                        .subtract(dayjs().utcOffset(), "minutes")
                        .format()
                  : undefined
              }
              onChange={(value) =>
                setEndTime(
                  value
                    ? useLocalTime
                      ? new Date(value).getTime()
                      : dayjs.utc(value).valueOf()
                    : null
                )
              }
              clearable
            />
          </Group>

          <Group>
            <Button
              color="red"
              leftSection={<IconTrash size={16} />}
              onClick={handleDelete}
              loading={deleting}
              disabled={!matchers.trim()}
            >
              Delete series
            </Button>
          </Group>
        </Stack>
      </InfoPageCard>

      <InfoPageCard title="Clean Tombstones">
        <Stack gap="md">
          {cleanError && (
            <Alert
              color="red"
              title="Error"
              withCloseButton
              onClose={() => setCleanError(null)}
            >
              {cleanError}
            </Alert>
          )}

          {cleanSuccess && (
            <Alert
              icon={<IconCheck size={16} />}
              color="green"
              title="Success"
              withCloseButton
              onClose={() => setCleanSuccess(null)}
            >
              {cleanSuccess}
            </Alert>
          )}

          <Text size="sm" c="dimmed">
            After deleting series, tombstones mark the data for deletion but do
            not free disk space immediately. Use this to remove tombstones and
            reclaim storage.
          </Text>
          <Group>
            <Button
              variant="outline"
              onClick={handleCleanTombstones}
              loading={cleaning}
            >
              Clean tombstones
            </Button>
          </Group>
        </Stack>
      </InfoPageCard>
    </InfoPageStack>
  );
}
