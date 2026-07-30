import { FC, ReactNode, useEffect, useState } from "react";
import {
  Table,
  Alert,
  Box,
  SegmentedControl,
  ScrollArea,
  Group,
  Stack,
  Text,
  Anchor,
  ActionIcon,
} from "@mantine/core";
import {
  IconAlertTriangle,
  IconInfoCircle,
  IconMinus,
  IconPlus,
} from "@tabler/icons-react";
import {
  ContextRef,
  InstantQueryResult,
  InstantSample,
  RangeSamples,
  SeriesContext,
} from "../../api/responseTypes/query";
import SeriesName from "./SeriesName";
import classes from "./DataTable.module.css";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { formatTimestamp } from "../../lib/formatTime";
import HistogramChart from "./HistogramChart";
import { Histogram } from "../../types/types";
import { bucketRangeString } from "./HistogramHelpers";
import { useSettings } from "../../state/settingsSlice";
dayjs.extend(timezone);

const maxFormattableSeries = 1000;
const maxDisplayableSeries = 10000;

const limitSeries = <S extends InstantSample | RangeSamples>(
  series: S[],
  limit: boolean
): S[] => {
  if (limit && series.length > maxDisplayableSeries) {
    return series.slice(0, maxDisplayableSeries);
  }
  return series;
};

type ResolvedContext = { id: string; ctx: SeriesContext };

// resolveContexts maps a series' context reference to the (deduplicated)
// entries it points at in the shared contexts table. Returns [] when the series
// has no resolvable context.
const resolveContexts = (
  ref: ContextRef | undefined,
  table: Record<string, SeriesContext> | undefined
): ResolvedContext[] => {
  if (ref === undefined || table === undefined) {
    return [];
  }
  const ids =
    typeof ref === "string"
      ? [ref]
      : Array.from(
          new Set(
            ref.map((r) => r.id).filter((id): id is string => id !== null)
          )
        );
  return ids
    .map((id) => ({ id, ctx: table[id] }))
    .filter((e): e is ResolvedContext => e.ctx !== undefined);
};

const contextAttrRows = (attrs: Record<string, string> | undefined) =>
  attrs === undefined
    ? null
    : Object.keys(attrs)
        .sort()
        .map((k) => (
          <div key={k} style={{ fontFamily: "var(--mantine-font-family-monospace)" }}>
            <Text span fz="xs" fw={600}>
              {k}
            </Text>
            <Text span fz="xs" c="gray.7">
              {" = "}
              {attrs[k]}
            </Text>
          </div>
        ));

// SeriesContextView renders resolved native-metadata context, one attribute per
// row, grouped into identifying and descriptive sections.
const SeriesContextView: FC<{ entries: ResolvedContext[] }> = ({ entries }) => (
  <Box
    mt={6}
    pl={8}
    style={{ borderLeft: "2px solid var(--mantine-color-gray-3)" }}
  >
    {entries.map(({ id, ctx }) => (
      <Box key={id} mb={entries.length > 1 ? 8 : 0}>
        {entries.length > 1 && (
          <Text fz="xs" c="dimmed" fw={700}>
            context {id}
          </Text>
        )}
        {ctx.resource?.identifying !== undefined && (
          <>
            <Text fz="10px" tt="uppercase" c="dimmed" fw={700} mt={4}>
              identifying
            </Text>
            {contextAttrRows(ctx.resource.identifying)}
          </>
        )}
        {ctx.resource?.descriptive !== undefined && (
          <>
            <Text fz="10px" tt="uppercase" c="dimmed" fw={700} mt={4}>
              descriptive
            </Text>
            {contextAttrRows(ctx.resource.descriptive)}
          </>
        )}
      </Box>
    ))}
  </Box>
);

export interface DataTableProps {
  data: InstantQueryResult;
  limitResults: boolean;
  setLimitResults: (limit: boolean) => void;
}

const DataTable: FC<DataTableProps> = ({
  data,
  limitResults,
  setLimitResults,
}) => {
  const [scale, setScale] = useState<string>("exponential");
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
  const { useLocalTime } = useSettings();

  const { result, resultType } = data;
  const contexts = data.contexts;
  const doFormat = result.length <= maxFormattableSeries;

  // Reset expansion when a new result arrives (row indices are no longer stable).
  useEffect(() => {
    setExpandedRows(new Set());
  }, [data]);

  const toggleRow = (idx: number) =>
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) {
        next.delete(idx);
      } else {
        next.add(idx);
      }
      return next;
    });

  // seriesNameCell renders the metric name and labels, prefixed by an expand
  // toggle when the series carries native-metadata context. Expanding shows the
  // context below the name, one attribute per row.
  const seriesNameCell = (
    metric: InstantSample["metric"],
    ctxRef: ContextRef | undefined,
    idx: number
  ): ReactNode => {
    const entries = resolveContexts(ctxRef, contexts);
    const expanded = expandedRows.has(idx);
    return (
      <Group gap={6} wrap="nowrap" align="flex-start">
        <Box w={22} style={{ flexShrink: 0 }}>
          {entries.length > 0 && (
            <ActionIcon
              variant="subtle"
              color="gray"
              size="sm"
              onClick={() => toggleRow(idx)}
              aria-label={expanded ? "Hide context" : "Show context"}
              title={expanded ? "Hide context" : "Show context"}
            >
              {expanded ? <IconMinus size={14} /> : <IconPlus size={14} />}
            </ActionIcon>
          )}
        </Box>
        <Box style={{ minWidth: 0 }}>
          <SeriesName labels={metric} format={doFormat} />
          {expanded && entries.length > 0 && (
            <SeriesContextView entries={entries} />
          )}
        </Box>
      </Group>
    );
  };

  return (
    <Stack gap="lg" mt={0}>
      {limitResults &&
        ["vector", "matrix"].includes(resultType) &&
        result.length > maxDisplayableSeries && (
          <Alert
            color="orange"
            icon={<IconAlertTriangle />}
            title="Showing limited results"
          >
            Fetched {data.result.length} metrics, only displaying first{" "}
            {maxDisplayableSeries} for performance reasons.
            <Anchor ml="md" fz="1em" onClick={() => setLimitResults(false)}>
              Show all results
            </Anchor>
          </Alert>
        )}

      {!doFormat && (
        <Alert title="Formatting turned off" icon={<IconInfoCircle />}>
          Showing more than {maxFormattableSeries} series, turning off label
          formatting to improve rendering performance.
        </Alert>
      )}

      <Box pos="relative" className={classes.tableWrapper}>
        <Table fz="xs">
          <Table.Tbody>
            {resultType === "vector" ? (
              limitSeries<InstantSample>(result, limitResults).map((s, idx) => (
                <Table.Tr key={idx}>
                  <Table.Td>{seriesNameCell(s.metric, s.context, idx)}</Table.Td>
                  <Table.Td className={classes.numberCell}>
                    {s.value && s.value[1]}
                    {s.histogram && (
                      <Stack>
                        <HistogramChart
                          histogram={s.histogram[1]}
                          index={idx}
                          scale={scale}
                        />
                        <Group justify="space-between" align="center" p={10}>
                          <Group align="center" gap="1rem">
                            <span>
                              <strong>Count:</strong> {s.histogram[1].count}
                            </span>
                            <span>
                              <strong>Sum:</strong> {s.histogram[1].sum}
                            </span>
                          </Group>
                          <Group align="center" gap="1rem">
                            <span>x-axis scale:</span>
                            <SegmentedControl
                              size={"xs"}
                              value={scale}
                              onChange={setScale}
                              data={["exponential", "linear"]}
                            />
                          </Group>
                        </Group>
                        {histogramTable(s.histogram[1])}
                      </Stack>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))
            ) : resultType === "matrix" ? (
              limitSeries<RangeSamples>(result, limitResults).map((s, idx) => (
                <Table.Tr key={idx}>
                  <Table.Td>{seriesNameCell(s.metric, s.context, idx)}</Table.Td>
                  <Table.Td className={classes.numberCell}>
                    {s.values &&
                      s.values.map((v, idx) => (
                        <div key={idx}>
                          {v[1]}{" "}
                          <Text
                            span
                            c="gray.7"
                            size="1em"
                            title={formatTimestamp(v[0], useLocalTime)}
                          >
                            @ {v[0]}
                          </Text>
                        </div>
                      ))}
                  </Table.Td>
                </Table.Tr>
              ))
            ) : resultType === "scalar" ? (
              <Table.Tr>
                <Table.Td>Scalar value</Table.Td>
                <Table.Td className={classes.numberCell}>{result[1]}</Table.Td>
              </Table.Tr>
            ) : resultType === "string" ? (
              <Table.Tr>
                <Table.Td>String value</Table.Td>
                <Table.Td>{result[1]}</Table.Td>
              </Table.Tr>
            ) : (
              <Alert
                color="red"
                title="Invalid query response"
                icon={<IconAlertTriangle />}
              >
                Invalid result value type
              </Alert>
            )}
          </Table.Tbody>
        </Table>
      </Box>
    </Stack>
  );
};

const histogramTable = (h: Histogram): ReactNode => (
  <Table withTableBorder fz="xs">
    <Table.Tbody
      style={{
        display: "flex",
        flexDirection: "column",
        justifyContent: "space-between",
      }}
    >
      <Table.Tr
        style={{
          display: "flex",
          flexDirection: "row",
          justifyContent: "space-between",
        }}
      >
        <Table.Th>Bucket range</Table.Th>
        <Table.Th>Count</Table.Th>
      </Table.Tr>
      <ScrollArea w={"100%"} h={265}>
        {h.buckets?.map((b, i) => (
          <Table.Tr key={i}>
            <Table.Td style={{ textAlign: "left" }}>
              {bucketRangeString(b)}
            </Table.Td>
            <Table.Td>{b[3]}</Table.Td>
          </Table.Tr>
        ))}
      </ScrollArea>
    </Table.Tbody>
  </Table>
);

export default DataTable;
