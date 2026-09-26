import { FC } from "react";
import { Box, Text, Tooltip, Table } from "@mantine/core";
import { QueryCostComparison, QueryStats } from "../../api/responseTypes/query";
import { formatCount } from "../../lib/formatCount";

const statsTable = (stats: Record<string, number>) => {
  return (
    <Table withRowBorders={false}>
      <Table.Tbody>
        {Object.entries(stats).map(([k, v]) => (
          <Table.Tr key={k}>
            <Table.Td pl={0} py={3} c="dimmed">
              {k}
            </Table.Td>
            <Table.Td pr={0} py={3} ta="right">
              {v}
            </Table.Td>
          </Table.Tr>
        ))}
      </Table.Tbody>
    </Table>
  );
};

// Renders estimated storage input next to the cost measured during
// execution. Peak samples are only measured for the actual cost, so the
// estimated column is left empty for that row.
const costTable = (cost: QueryCostComparison) => {
  const rows: [string, number | null, number][] = [
    ["series touched", cost.estimated.seriesTouched, cost.actual.seriesTouched],
    [
      "samples read",
      cost.estimated.samplesRead,
      cost.actual.samplesRead,
    ],
    ["peak samples", null, cost.actual.peakSamples ?? 0],
  ];

  return (
    <Table withRowBorders={false}>
      <Table.Thead>
        <Table.Tr>
          <Table.Th pl={0} py={3} fw="normal" c="dimmed" />
          <Table.Th py={3} fw="normal" c="dimmed" ta="right">
            estimated
          </Table.Th>
          <Table.Th pr={0} py={3} fw="normal" c="dimmed" ta="right">
            actual
          </Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {rows.map(([name, estimated, actual]) => (
          <Table.Tr key={name}>
            <Table.Td pl={0} py={3} c="dimmed">
              {name}
            </Table.Td>
            <Table.Td py={3} ta="right">
              {estimated === null ? "–" : formatCount(estimated)}
            </Table.Td>
            <Table.Td pr={0} py={3} ta="right">
              {formatCount(actual)}
            </Table.Td>
          </Table.Tr>
        ))}
      </Table.Tbody>
    </Table>
  );
};

const QueryStatsDisplay: FC<{
  numResults: number;
  responseTime: number;
  stats: QueryStats;
  cost?: QueryCostComparison;
}> = ({ numResults, responseTime, stats, cost }) => {
  return (
    <Tooltip
      label={
        <Box p="xs">
          <Text mb="xs">Timing stats (s):</Text>
          {statsTable(stats.timings)}
          <Text mt="sm" mb="xs">
            Sample stats:
          </Text>
          {statsTable(stats.samples)}
          {cost && (
            <>
              <Text mt="sm" mb="xs">
                Query cost:
              </Text>
              {costTable(cost)}
            </>
          )}
        </Box>
      }
    >
      <Text size="xs" c="gray">
        Load time: {responseTime}ms &ensp; Result series: {numResults}
      </Text>
    </Tooltip>
  );
};

export default QueryStatsDisplay;
