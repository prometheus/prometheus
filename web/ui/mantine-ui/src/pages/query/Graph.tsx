import { FC, useEffect, useId, useState } from "react";
import { Alert, Skeleton, Box, LoadingOverlay } from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { InstantQueryResult } from "../../api/responseTypes/query";
import { useAPIQuery } from "../../api/api";
import classes from "./Graph.module.css";
import {
  GraphDisplayMode,
  GraphResolution,
  getEffectiveResolution,
} from "../../state/queryPageSlice";
import "uplot/dist/uPlot.min.css";
import "./uplot.css";
import { useElementSize } from "@mantine/hooks";
import UPlotChart, { UPlotChartRange } from "./UPlotChart";

export interface GraphProps {
  expr: string;
  endTime: number | null;
  range: number;
  resolution: GraphResolution;
  showExemplars: boolean;
  displayMode: GraphDisplayMode;
  retriggerIdx: number;
  onSelectRange: (start: number, end: number) => void;
}

const Graph: FC<GraphProps> = ({
  expr,
  endTime,
  range,
  resolution,
  showExemplars,
  displayMode,
  retriggerIdx,
  onSelectRange,
}) => {
  const { ref, width } = useElementSize();
  const [rerender, setRerender] = useState(true);

  const effectiveEndTime = (endTime !== null ? endTime : Date.now()) / 1000;
  const startTime = effectiveEndTime - range / 1000;
  const effectiveResolution = getEffectiveResolution(resolution, range) / 1000;

  const { data, error, isFetching, isLoading, refetch } =
    useAPIQuery<InstantQueryResult>({
      key: useId(),
      path: "/query_range",
      params: {
        query: expr,
        step: effectiveResolution.toString(),
        start: startTime.toString(),
        end: effectiveEndTime.toString(),
      },
      enabled: expr !== "",
    });

  // Keep the displayed chart range separate from the actual query range, so that
  // the chart will keep displaying the old range while a query for a new range
  // is still in progress.
  const [displayedChartRange, setDisplayedChartRange] =
    useState<UPlotChartRange>({
      startTime: startTime,
      endTime: effectiveEndTime,
      resolution: effectiveResolution,
    });

  useEffect(() => {
    setDisplayedChartRange({
      startTime: startTime,
      endTime: effectiveEndTime,
      resolution: effectiveResolution,
    });
    // We actually want to update the displayed range only once the new data is there.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  useEffect(() => {
    expr !== "" && refetch();
  }, [retriggerIdx, refetch, expr, endTime, range, resolution]);

  useEffect(() => {
    if (data !== undefined && rerender) {
      setRerender(false);
    }
  }, [data, rerender, setRerender]);

  // TODO: Share all the loading/error/empty data notices with the DataTable.

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

  if (resultType !== "matrix") {
    return (
      <Alert
        title="Invalid query result"
        icon={<IconAlertTriangle size={14} />}
      >
        This query returned a result of type "{resultType}", but a matrix was
        expected.
      </Alert>
    );
  }

  if (result.length === 0) {
    return (
      <Alert title="Empty query result" icon={<IconInfoCircle size={14} />}>
        This query returned no data.
      </Alert>
    );
  }

  return (
    <Box pos="relative" ref={ref} className={classes.chartWrapper}>
      <LoadingOverlay
        visible={isFetching}
        zIndex={1000}
        h={570}
        overlayProps={{ radius: "sm", blur: 0.5 }}
        loaderProps={{ type: "dots", color: "gray.6" }}
        // loaderProps={{
        //   children: <Skeleton m={0} w="100%" h="100%" />,
        // }}
        // styles={{ loader: { width: "100%", height: "100%" } }}
      />
      <UPlotChart
        data={data.data.result}
        range={displayedChartRange}
        width={width}
        showExemplars={showExemplars}
        displayMode={displayMode}
        onSelectRange={onSelectRange}
      />
    </Box>
  );
};

export default Graph;
