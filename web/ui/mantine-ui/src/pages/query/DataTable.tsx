import { FC, ReactNode, useEffect, useId, useState } from "react";
import {
  Table,
  Alert,
  Skeleton,
  Box,
  LoadingOverlay,
  SegmentedControl,
  ScrollArea,
} from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import {
  InstantQueryResult,
  InstantSample,
  RangeSamples,
} from "../../api/responseTypes/query";
import SeriesName from "./SeriesName";
import { useAPIQuery } from "../../api/api";
import classes from "./DataTable.module.css";
import dayjs from "dayjs";
import timezone from "dayjs/plugin/timezone";
import { useAppSelector } from "../../state/hooks";
import { formatTimestamp } from "../../lib/formatTime";
import HistogramChart from "./HistogramChart";
import { Histogram } from "../../types/types";
import { bucketRangeString } from "./HistogramHelpers";
dayjs.extend(timezone);

const maxFormattableSeries = 1000;
const maxDisplayableSeries = 10000;

const limitSeries = <S extends InstantSample | RangeSamples>(
  series: S[]
): S[] => {
  if (series.length > maxDisplayableSeries) {
    return series.slice(0, maxDisplayableSeries);
  }
  return series;
};

export interface DataTableProps {
  expr: string;
  evalTime: number | null;
  retriggerIdx: number;
}

const DataTable: FC<DataTableProps> = ({ expr, evalTime, retriggerIdx }) => {
  const [scale, setScale] = useState<string>("exponential");

  const { data, error, isFetching, isLoading, refetch } =
    useAPIQuery<InstantQueryResult>({
      key: useId(),
      path: "/query",
      params: {
        query: expr,
        time: `${(evalTime !== null ? evalTime : Date.now()) / 1000}`,
      },
      enabled: expr !== "",
    });

  useEffect(() => {
    expr !== "" && refetch();
  }, [retriggerIdx, refetch, expr, evalTime]);

  const useLocalTime = useAppSelector((state) => state.settings.useLocalTime);

  // Show a skeleton only on the first load, not on subsequent ones.
  if (isLoading) {
    return (
      <Box>
        {Array.from(Array(5), (_, i) => (
          <Skeleton key={i} height={30} mb={15} />
        ))}
      </Box>
    );
  }

  if (error) {
    return (
      <Alert
        color="red"
        title="Error executing query"
        icon={<IconAlertTriangle size={14} />}
      >
        {error.message}
      </Alert>
    );
  }

  if (data === undefined) {
    return <Alert variant="transparent">No data queried yet</Alert>;
  }

  const { result, resultType } = data.data;

  if (result.length === 0) {
    return (
      <Alert title="Empty query result" icon={<IconInfoCircle size={14} />}>
        This query returned no data.
      </Alert>
    );
  }

  const doFormat = result.length <= maxFormattableSeries;

  return (
    <Box pos="relative" className={classes.tableWrapper}>
      <LoadingOverlay
        visible={isFetching}
        zIndex={1000}
        overlayProps={{ radius: "sm", blur: 1 }}
        loaderProps={{
          children: <Skeleton m={0} w="100%" h="100%" />,
        }}
        styles={{ loader: { width: "100%", height: "100%" } }}
      />
      <Table fz="xs">
        <Table.Tbody>
          {resultType === "vector" ? (
            limitSeries<InstantSample>(result).map((s, idx) => (
              <Table.Tr key={idx}>
                <Table.Td>
                  <SeriesName labels={s.metric} format={doFormat} />
                </Table.Td>
                <Table.Td className={classes.numberCell}>
                  {s.value && s.value[1]}
                  {s.histogram && (
                    <>
                      <HistogramChart
                        histogram={s.histogram[1]}
                        index={idx}
                        scale={scale}
                      />
                      <div className={classes.histogramSummaryWrapper}>
                        <div className={classes.histogramSummary}>
                          <span>
                            <strong>Total count:</strong> {s.histogram[1].count}
                          </span>
                          <span>
                            <strong>Sum:</strong> {s.histogram[1].sum}
                          </span>
                        </div>
                        <div className={classes.histogramSummary}>
                          <span>x-axis scale:</span>
                          <SegmentedControl
                            size={"xs"}
                            value={scale}
                            onChange={setScale}
                            data={["exponential", "linear"]}
                          />
                        </div>
                      </div>
                      {histogramTable(s.histogram[1])}
                    </>
                  )}
                </Table.Td>
              </Table.Tr>
            ))
          ) : resultType === "matrix" ? (
            limitSeries<RangeSamples>(result).map((s, idx) => (
              <Table.Tr key={idx}>
                <Table.Td>
                  <SeriesName labels={s.metric} format={doFormat} />
                </Table.Td>
                <Table.Td className={classes.numberCell}>
                  {s.values &&
                    s.values.map((v, idx) => (
                      <div key={idx}>
                        {v[1]} @{" "}
                        {
                          <span title={formatTimestamp(v[0], useLocalTime)}>
                            {v[0]}
                          </span>
                        }
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
              icon={<IconAlertTriangle size={14} />}
            >
              Invalid result value type
            </Alert>
          )}
        </Table.Tbody>
      </Table>
    </Box>
  );
};

export const histogramTable = (h: Histogram): ReactNode => (
  <Table>
    <Table.Thead>
      <Table.Tr>
        <Table.Th style={{ textAlign: "center" }} colSpan={2}>
          Bucket counts
        </Table.Th>
      </Table.Tr>
    </Table.Thead>
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
        <Table.Th>Range</Table.Th>
        <Table.Th>Count</Table.Th>
      </Table.Tr>
      <ScrollArea w={"100%"} h={250}>
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
