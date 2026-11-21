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
import { FeaturesResult } from "../api/responseTypes/features";
import { TSDBStatusResult } from "../api/responseTypes/tsdbStatus";
import { formatTimestamp } from "../lib/formatTime";
import { useSettings } from "../state/settingsSlice";
import InfoPageStack from "../components/InfoPageStack";
import InfoPageCard from "../components/InfoPageCard";
import { useI18n } from "../i18nContext";

dayjs.extend(utc);

export default function TSDBStatusPage() {
  const { t } = useI18n();
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

  const {
    data: {
      data: { api: featureApi },
    },
  } = useSuspenseAPIQuery<FeaturesResult>({ path: '/features' });

  const adminApiEnabled = featureApi?.admin ?? false;

  const { useLocalTime, pathPrefix } = useSettings();

  const unixToTime = (unix: number): string => {
    const formatted = formatTimestamp(unix, useLocalTime);
    if (formatted === "Invalid Date") {
      if (numSeries === 0) {
        return t("tsdb.noDatapoints");
      }
      return t("tsdb.errorParsingTime", { time: unix });
    }

    return formatted;
  };

  const { chunkCount, numSeries, numLabelPairs, minTime, maxTime } = headStats;
  const stats = [
    { name: t("tsdb.numberSeries"), value: numSeries },
    { name: t("tsdb.numberChunks"), value: chunkCount },
    { name: t("tsdb.numberLabelPairs"), value: numLabelPairs },
    { name: t("tsdb.currentMinTime"), value: `${unixToTime(minTime / 1000)}` },
    { name: t("tsdb.currentMaxTime"), value: `${unixToTime(maxTime / 1000)}` },
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
      setDeleteError(t("tsdb.provideSelector"));
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
        t("tsdb.deleteSuccess", { series: matchList.join(", ") })
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

      setCleanSuccess(t("tsdb.cleanSuccess"));
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
          title: t("tsdb.statusHead"),
          stats,
          formatAsCode: false,
        },
        {
          title: t("tsdb.topLabelNamesByValues"),
          stats: labelValueCountByLabelName,
          formatAsCode: true,
        },
        {
          title: t("tsdb.topMetricNames"),
          stats: seriesCountByMetricName,
          formatAsCode: true,
        },
        {
          title: t("tsdb.topLabelNamesMemory"),
          unit: t("tsdb.bytes"),
          stats: memoryInBytesByLabelName,
          formatAsCode: true,
        },
        {
          title: t("tsdb.topLabelPairs"),
          stats: seriesCountByLabelValuePair,
          formatAsCode: true,
        },
      ].map(({ title, unit = t("tsdb.count"), stats, formatAsCode }) => (
        <InfoPageCard title={title}>
          <Table layout="fixed">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>{t("tsdb.name")}</Table.Th>
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

      <InfoPageCard title={t("tsdb.delete")} icon={IconTrash}>
        <Stack gap="md">
          <Alert
            icon={<IconAlertTriangle size={16} />}
            color="yellow"
            title={t("tsdb.warning")}
          >
            {t("tsdb.deleteWarning")}
          </Alert>

          {deleteError && (
            <Alert
              color="red"
              title={t("tsdb.error")}
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
              title={t("tsdb.success")}
              withCloseButton
              onClose={() => setDeleteSuccess(null)}
            >
              {deleteSuccess}
            </Alert>
          )}

          <Textarea
            label={t("tsdb.seriesSelectors")}
            description='PromQL series selectors, one per line. Example: up{job="prometheus"}'
            placeholder={'up{job="prometheus"}'}
            disabled={!adminApiEnabled}
            title={!adminApiEnabled ? t("tsdb.requiresAdminApi") : ""}
            value={matchers}
            onChange={(e) => setMatchers(e.currentTarget.value)}
            autosize
            minRows={2}
            maxRows={6}
          />

          <Group grow>
            <DateTimePicker
              label={t("tsdb.startTime")}
              description={t("tsdb.optional")}
              placeholder={t("tsdb.selectStartTime")}
              valueFormat="YYYY-MM-DD HH:mm:ss"
              disabled={!adminApiEnabled}
              title={!adminApiEnabled ? t("tsdb.requiresAdminApi") : ""}
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
              label={t("tsdb.endTime")}
              description={t("tsdb.optional")}
              placeholder={t("tsdb.selectEndTime")}
              valueFormat="YYYY-MM-DD HH:mm:ss"
              disabled={!adminApiEnabled}
              title={!adminApiEnabled ? t("tsdb.requiresAdminApi") : ""}
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
              disabled={!matchers.trim() || !adminApiEnabled}
              title={!adminApiEnabled ? t("tsdb.requiresAdminApi") : ""}
            >
              {t("tsdb.delete")}
            </Button>
          </Group>
        </Stack>
      </InfoPageCard>

      <InfoPageCard title={t("tsdb.clean")}>
        <Stack gap="md">
          {cleanError && (
            <Alert
              color="red"
              title={t("tsdb.error")}
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
              title={t("tsdb.success")}
              withCloseButton
              onClose={() => setCleanSuccess(null)}
            >
              {cleanSuccess}
            </Alert>
          )}

          <Text size="sm" c="dimmed">
            {t("tsdb.cleanDescription")}
          </Text>
          <Group>
            <Button
              variant="outline"
              onClick={handleCleanTombstones}
              loading={cleaning}
              disabled={!adminApiEnabled}
              title={!adminApiEnabled ? t("tsdb.requiresAdminApi") : ""}
            >
              {t("tsdb.clean")}
            </Button>
          </Group>
        </Stack>
      </InfoPageCard>
    </InfoPageStack>
  );
}
