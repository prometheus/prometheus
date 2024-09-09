import { Stack, Card, Table, Text } from "@mantine/core";
import { useSuspenseAPIQuery } from "../api/api";
import { TSDBStatusResult } from "../api/responseTypes/tsdbStatus";
import { formatTimestamp } from "../lib/formatTime";
import { useSettings } from "../state/settingsSlice";

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

  const { useLocalTime } = useSettings();

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

  return (
    <Stack gap="lg" maw={1000} mx="auto" mt="xs">
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
        <Card shadow="xs" withBorder p="md">
          <Text fz="xl" fw={600} ml="xs" mb="sm">
            {title}
          </Text>
          <Table layout="fixed">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>{unit}</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {stats.map(({ name, value }) => {
                return (
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
                );
              })}
            </Table.Tbody>
          </Table>
        </Card>
      ))}
    </Stack>
  );
}
