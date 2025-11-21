import { FC, useEffect, useId, useState } from "react";
import { Alert, Skeleton, Box, LoadingOverlay, Stack } from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { RangeQueryResult } from "../../api/responseTypes/query";
import { SuccessAPIResponse, useAPIQuery } from "../../api/api";
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
import ASTNode, { nodeType } from "../../promql/ast";
import serializeNode from "../../promql/serialize";
import { useSettings } from "../../state/settingsSlice";

export interface GraphProps {
  expr: string;
  node: ASTNode | null;
  endTime: number | null;
  range: number;
  resolution: GraphResolution;
  showExemplars: boolean;
  displayMode: GraphDisplayMode;
  yAxisMin: number | null;
  retriggerIdx: number;
  onSelectRange: (start: number, end: number) => void;
}

const Graph: FC<GraphProps> = ({
  expr,
  node,
  endTime,
  range,
  resolution,
  showExemplars,
  displayMode,
  yAxisMin,
  retriggerIdx,
  onSelectRange,
}) => {
  const { ref, width } = useElementSize();
  const [rerender, setRerender] = useState(true);
  const { showQueryWarnings, showQueryInfoNotices } = useSettings();

  const effectiveExpr =
    node === null
      ? expr
      : serializeNode(
          node.type === nodeType.matrixSelector
            ? {
                type: nodeType.vectorSelector,
                name: node.name,
                matchers: node.matchers,
                offset: node.offset,
                timestamp: node.timestamp,
                startOrEnd: node.startOrEnd,
                anchored: node.anchored,
                smoothed: node.smoothed,
              }
            : node
        );

  const effectiveEndTime = (endTime !== null ? endTime : Date.now()) / 1000;
  const startTime = effectiveEndTime - range / 1000;
  const effectiveResolution = getEffectiveResolution(resolution, range) / 1000;

  const { data, error, isFetching, isLoading, refetch } =
    useAPIQuery<RangeQueryResult>({
      key: [useId()],
      path: "/query_range",
      params: {
        query: effectiveExpr,
        step: effectiveResolution.toString(),
        start: startTime.toString(),
        end: effectiveEndTime.toString(),
      },
      enabled: effectiveExpr !== "",
    });

  // Bundle the chart data and the displayed range together. This has two purposes:
  // 1. If we update them separately, we cause unnecessary rerenders of the uPlot chart itself.
  // 2. We want to keep displaying the old range in the chart while a query for a new range
  //    is still in progress.
  const [dataAndRange, setDataAndRange] = useState<{
    data: SuccessAPIResponse<RangeQueryResult>;
    range: UPlotChartRange;
  } | null>(null);

  // Helper function to render warnings.
  const renderAlerts = (warnings?: string[], infos?: string[]) => {
    return (
      <>
        {showQueryWarnings &&
          warnings?.map((w, idx) => (
            <Alert
              key={idx}
              color="red"
              title="Query warning"
              icon={<IconAlertTriangle />}
            >
              {w}
            </Alert>
          ))}
        {showQueryInfoNotices &&
          infos?.map((w, idx) => (
            <Alert
              key={idx}
              color="yellow"
              title="Query notice"
              icon={<IconInfoCircle />}
            >
              {w}
            </Alert>
          ))}
      </>
    );
  };

  useEffect(() => {
    if (data !== undefined) {
      setDataAndRange({
        data: data,
        range: {
          startTime: startTime,
          endTime: effectiveEndTime,
          resolution: effectiveResolution,
        },
      });
    }
    // We actually want to update the displayed range only once the new data is there,
    // so we don't want to include any of the range-related parameters in the dependencies.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data]);

  // Re-execute the query when the user presses Enter (or hits the Execute button).
  useEffect(() => {
    if (effectiveExpr !== "") {
      refetch();
    }
  }, [retriggerIdx, refetch, effectiveExpr, endTime, range, resolution]);

  // The useElementSize hook above only gets a valid size on the second render, so this
  // is a workaround to make the component render twice after mount.
  useEffect(() => {
    if (dataAndRange !== null && rerender) {
      setRerender(false);
    }
  }, [dataAndRange, rerender, setRerender]);

  // TODO: Share all the loading/error/empty data notices with the DataTable?

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
        icon={<IconAlertTriangle />}
      >
        {error.message}
      </Alert>
    );
  }

  if (dataAndRange === null) {
    return <Alert variant="transparent">No data queried yet</Alert>;
  }

  const { result } = dataAndRange.data.data;

  if (result.length === 0) {
    return (
      <Stack>
        <Alert title="Empty query result" icon={<IconInfoCircle />}>
          This query returned no data.
        </Alert>
        {renderAlerts(dataAndRange.data.warnings)}
      </Stack>
    );
  }

  return (
    <Stack>
      {node !== null && node.type === nodeType.matrixSelector && (
        <Alert
          color="orange"
          title="Graphing modified expression"
          icon={<IconAlertTriangle />}
        >
          <strong>Note:</strong> Range vector selectors can't be graphed, so
          graphing the equivalent instant vector selector instead.
        </Alert>
      )}
      {renderAlerts(dataAndRange.data.warnings, dataAndRange.data.infos)}
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
          data={dataAndRange.data.data.result}
          range={dataAndRange.range}
          width={width}
          showExemplars={showExemplars}
          displayMode={displayMode}
          yAxisMin={yAxisMin}
          onSelectRange={onSelectRange}
        />
      </Box>
    </Stack>
  );
};

export default Graph;
