import { FC, useState } from "react";
import {
  Alert,
  Code,
  Group,
  Modal,
  Paper,
  SimpleGrid,
  Skeleton,
  Stack,
  Text,
} from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { useAPIQuery } from "../../api/api";
import { QueryCostResult } from "../../api/responseTypes/query";
import { useAppSelector } from "../../state/hooks";
import { getEffectiveResolution } from "../../state/queryPageSlice";
import { formatPrometheusDuration } from "../../lib/formatTime";
import { formatCount } from "../../lib/formatCount";

const costCard = (label: string, value: string, hint: string) => (
  <Paper key={label} withBorder p="md" radius="md">
    <Text size="xs" c="dimmed" tt="uppercase" fw={700}>
      {label}
    </Text>
    <Text size="xl" fw={700} my={4}>
      {value}
    </Text>
    <Text size="xs" c="dimmed">
      {hint}
    </Text>
  </Paper>
);

// Estimates the cost of the expression currently in the editor without
// executing it. The evaluation parameters mirror the ones the panel would use
// to run the query, so that the estimate matches what executing it would cost.
const CostEstimate: FC<{ panelIdx: number; expr: string }> = ({
  panelIdx,
  expr,
}) => {
  const { visualizer } = useAppSelector(
    (state) => state.queryPage.panels[panelIdx],
  );
  const { activeTab, endTime, range, resolution } = visualizer;

  // Freeze the default evaluation time, so that re-renders do not keep moving
  // the query window (and thus the query key) while the estimate is loading.
  const [now] = useState(() => Date.now());

  const rangeQuery = activeTab === "graph";
  const effectiveEndTime = (endTime !== null ? endTime : now) / 1000;
  const step = getEffectiveResolution(resolution, range) / 1000;

  const { data, error, isFetching } = useAPIQuery<QueryCostResult>({
    path: rangeQuery ? "/query_range_cost" : "/query_cost",
    params: rangeQuery
      ? {
          query: expr,
          start: (effectiveEndTime - range / 1000).toString(),
          end: effectiveEndTime.toString(),
          step: step.toString(),
        }
      : {
          query: expr,
          time: effectiveEndTime.toString(),
        },
  });

  return (
    <Stack gap="md">
      <Code block>{expr}</Code>
      <Text size="sm" c="dimmed">
        {rangeQuery
          ? `Range query over the last ${formatPrometheusDuration(range)} at a ${formatPrometheusDuration(step * 1000)} resolution.`
          : "Instant query."}
      </Text>
      {isFetching ? (
        <Group grow>
          <Skeleton height={92} radius="md" />
          <Skeleton height={92} radius="md" />
        </Group>
      ) : error !== null ? (
        <Alert
          color="red"
          title="Error estimating query cost"
          icon={<IconAlertTriangle />}
        >
          {error.message}
          <Text size="sm" mt="xs">
            Cost estimation requires the <Code>query-cost</Code> feature flag (
            <Code>--enable-feature=query-cost</Code>) on the server, and is
            unavailable in agent mode.
          </Text>
        </Alert>
      ) : (
        data !== undefined && (
          <>
            <SimpleGrid cols={2}>
              {costCard(
                "Series touched",
                formatCount(data.data.estimate.seriesTouched),
                "Series read by the query, summed per selector.",
              )}
              {costCard(
                "Samples read",
                formatCount(data.data.estimate.samplesRead),
                "Storage input in sample units; histograms are weighted by size.",
              )}
            </SimpleGrid>
            {data.warnings?.map((warning) => (
              <Alert
                key={warning}
                color="yellow"
                icon={<IconAlertTriangle />}
                title="Estimation warning"
              >
                {warning}
              </Alert>
            ))}
            <Alert
              variant="light"
              color="gray"
              icon={<IconInfoCircle />}
              title="These are estimates"
            >
              The estimate uses the index and a bounded sample of stored data.
              It can be higher or lower than the actual cost: selectors can
              share series, indexed series can lack samples in the window, and
              sampled data may not represent all matching series. Estimates are
              advisory and never used to reject a query.
            </Alert>
          </>
        )
      )}
    </Stack>
  );
};

// Shows the estimated cost of an expression in a modal, on demand.
const QueryCostModal: FC<{
  panelIdx: number;
  expr: string;
  opened: boolean;
  onClose: () => void;
}> = ({ panelIdx, expr, opened, onClose }) => (
  <Modal
    size="lg"
    opened={opened}
    onClose={onClose}
    title="Estimated query cost"
  >
    {/* Only mount (and thus estimate) while the modal is open. */}
    {opened && <CostEstimate panelIdx={panelIdx} expr={expr} />}
  </Modal>
);

export default QueryCostModal;
