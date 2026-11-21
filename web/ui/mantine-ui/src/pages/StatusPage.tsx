import { Table } from "@mantine/core";
import { useSuspenseAPIQuery } from "../api/api";
import { IconRun, IconWall } from "@tabler/icons-react";
import { formatTimestamp } from "../lib/formatTime";
import { useSettings } from "../state/settingsSlice";
import InfoPageCard from "../components/InfoPageCard";
import InfoPageStack from "../components/InfoPageStack";
import { useI18n } from "../i18nContext";

export default function StatusPage() {
  const { data: buildinfo } = useSuspenseAPIQuery<Record<string, string>>({
    path: `/status/buildinfo`,
  });
  const { data: runtimeinfo } = useSuspenseAPIQuery<Record<string, string>>({
    path: `/status/runtimeinfo`,
  });

  const { useLocalTime } = useSettings();
  const { t } = useI18n();

  const statusConfig: Record<
    string,
    {
      title?: string;
      formatValue?: (v: string | boolean) => string;
    }
  > = {
    startTime: {
      title: t("status.startTime"),
      formatValue: (v: string | boolean) =>
        formatTimestamp(new Date(v as string).valueOf() / 1000, useLocalTime),
    },
    CWD: { title: t("status.workingDirectory") },
    hostname: { title: t("status.hostname") },
    serverTime: {
      title: t("status.serverTime"),
      formatValue: (v: string | boolean) =>
        formatTimestamp(new Date(v as string).valueOf() / 1000, useLocalTime),
    },
    reloadConfigSuccess: {
      title: t("status.configurationReload"),
      formatValue: (v: string | boolean) =>
        v ? t("status.successful") : t("status.unsuccessful"),
    },
    lastConfigTime: {
      title: t("status.lastConfigurationReload"),
      formatValue: (v: string | boolean) =>
        formatTimestamp(new Date(v as string).valueOf() / 1000, useLocalTime),
    },
    corruptionCount: { title: t("status.walCorruptions") },
    goroutineCount: { title: t("status.goroutines") },
    storageRetention: { title: t("status.storageRetention") },
  };

  return (
    <InfoPageStack>
      <InfoPageCard title={t("status.buildInformation")} icon={IconWall}>
        <Table layout="fixed">
          <Table.Tbody>
            {Object.entries(buildinfo.data).map(([k, v]) => (
              <Table.Tr key={k}>
                <Table.Th style={{ textTransform: "capitalize" }}>{k}</Table.Th>
                <Table.Td>{v}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </InfoPageCard>
      <InfoPageCard title={t("status.runtimeInformation")} icon={IconRun}>
        <Table layout="fixed">
          <Table.Tbody>
            {Object.entries(runtimeinfo.data).map(([k, v]) => {
              const { title = k, formatValue = (val: string) => val } =
                statusConfig[k] || {};
              return (
                <Table.Tr key={k}>
                  <Table.Th style={{ textTransform: "capitalize" }}>
                    {title}
                  </Table.Th>
                  <Table.Td>{formatValue(v)}</Table.Td>
                </Table.Tr>
              );
            })}
          </Table.Tbody>
        </Table>
      </InfoPageCard>
    </InfoPageStack>
  );
}
