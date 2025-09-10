import { FC, ReactNode, useState } from "react";
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
} from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import {
  InstantQueryResult,
  InstantSample,
  RangeSamples,
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
  const { useLocalTime } = useSettings();

  const { result, resultType } = data;
  const doFormat = result.length <= maxFormattableSeries;

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
                  <Table.Td>
                    <SeriesName labels={s.metric} format={doFormat} />
                  </Table.Td>
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
                  <Table.Td>
                    <SeriesName labels={s.metric} format={doFormat} />
                  </Table.Td>
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
