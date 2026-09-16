import { FC, useId } from "react";
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
                offsetExpr: node.offsetExpr,
                timestamp: node.timestamp,
                startOrEnd: node.startOrEnd,
                anchored: node.anchored,
                smoothed: node.smoothed,
              }
            : node,
        );

  const effectiveResolution = getEffectiveResolution(resolution, range) / 1000;
  const {
    data: dataAndRange,
    error,
    isFetching,
    isLoading,
  } = useAPIQuery<
    RangeQueryResult,
    { data: SuccessAPIResponse<RangeQueryResult>; range: UPlotChartRange }
  >({
    key: [
      useId(),
      "/query_range",
      effectiveExpr,
      endTime,
      range,
      effectiveResolution,
      retriggerIdx,
    ],
    path: "/query_range",
    params: (requestTimeMs) => {
      const end = (endTime ?? requestTimeMs) / 1000;
      return {
        query: effectiveExpr,
        step: effectiveResolution.toString(),
        start: (end - range / 1000).toString(),
        end: end.toString(),
      };
    },
    enabled: effectiveExpr !== "",
    keepPreviousData: true,
    select: (data, { params }) => ({
      data,
      range: {
        startTime: Number(params.start),
        endTime: Number(params.end),
        resolution: Number(params.step),
      },
    }),
  });

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

  if (dataAndRange === undefined) {
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
