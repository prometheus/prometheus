import { Table } from "@mantine/core";
import { useSuspenseAPIQuery } from "../api/api";
import { IconRun, IconWall } from "@tabler/icons-react";
import { formatTimestamp } from "../lib/formatTime";
import { useSettings } from "../state/settingsSlice";
import InfoPageCard from "../components/InfoPageCard";
import InfoPageStack from "../components/InfoPageStack";

export default function StatusPage() {
  const { data: buildinfo } = useSuspenseAPIQuery<Record<string, string>>({
    path: `/status/buildinfo`,
  });
  const { data: runtimeinfo } = useSuspenseAPIQuery<Record<string, string>>({
    path: `/status/runtimeinfo`,
  });

  const { useLocalTime } = useSettings();

  const statusConfig: Record<
    string,
    {
      title?: string;
      formatValue?: (v: string | boolean) => string;
    }
  > = {
    startTime: {
      title: "Start time",
      formatValue: (v: string | boolean) =>
        formatTimestamp(new Date(v as string).valueOf() / 1000, useLocalTime),
    },
    CWD: { title: "Working directory" },
    hostname: { title: "Hostname" },
    serverTime: {
      title: "Server Time",
      formatValue: (v: string | boolean) =>
        formatTimestamp(new Date(v as string).valueOf() / 1000, useLocalTime),
    },
    reloadConfigSuccess: {
      title: "Configuration reload",
      formatValue: (v: string | boolean) => (v ? "Successful" : "Unsuccessful"),
    },
    lastConfigTime: {
      title: "Last successful configuration reload",
      formatValue: (v: string | boolean) =>
        formatTimestamp(new Date(v as string).valueOf() / 1000, useLocalTime),
    },
    corruptionCount: { title: "WAL corruptions" },
    goroutineCount: { title: "Goroutines" },
    storageRetention: { title: "Storage retention" },
  };

  return (
    <InfoPageStack>
      <InfoPageCard title="Build information" icon={IconWall}>
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
      <InfoPageCard title="Runtime information" icon={IconRun}>
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
