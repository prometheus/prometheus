import { FC } from "react";
import { Box, Text, Tooltip, Table } from "@mantine/core";
import { QueryStats } from "../../api/responseTypes/query";

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

const QueryStatsDisplay: FC<{
  numResults: number;
  responseTime: number;
  stats: QueryStats;
}> = ({ numResults, responseTime, stats }) => {
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
