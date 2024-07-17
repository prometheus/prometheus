import { FC, useEffect, useId } from "react";
import { Alert, Skeleton, Box, LoadingOverlay } from "@mantine/core";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { InstantQueryResult } from "../../api/responseTypes/query";
import { useAPIQuery } from "../../api/api";
import classes from "./DataTable.module.css";
import { GraphDisplayMode } from "../../state/queryPageSlice";
import { EChart, chartHeight, legendMargin } from "./EChart";
import { formatSeries } from "../../lib/formatSeries";
import { EChartsOption } from "echarts";

export interface GraphProps {
  expr: string;
  endTime: number | null;
  range: number;
  resolution: number | null;
  showExemplars: boolean;
  displayMode: GraphDisplayMode;
  retriggerIdx: number;
}

const Graph: FC<GraphProps> = ({
  expr,
  endTime,
  range,
  resolution,
  displayMode,
  retriggerIdx,
}) => {
  const realEndTime = (endTime !== null ? endTime : Date.now()) / 1000;
  const { data, error, isFetching, isLoading, refetch } =
    useAPIQuery<InstantQueryResult>({
      key: useId(),
      path: "/query_range",
      params: {
        query: expr,
        step: (
          resolution || Math.max(Math.floor(range / 250000), 1)
        ).toString(),
        start: (realEndTime - range / 1000).toString(),
        end: realEndTime.toString(),
      },
      enabled: expr !== "",
    });

  useEffect(() => {
    expr !== "" && refetch();
  }, [retriggerIdx, refetch, expr, endTime, range, resolution]);

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

  const option: EChartsOption = {
    animation: false,
    grid: {
      left: 20,
      top: 20,
      right: 20,
      bottom: 20 + result.length * 24,
      containLabel: true,
    },
    legend: {
      type: "scroll",
      icon: "square",
      orient: "vertical",
      top: chartHeight + legendMargin,
      bottom: 20,
      left: 30,
      right: 20,
    },
    xAxis: {
      type: "category",
      // min: realEndTime * 1000 - range,
      // max: realEndTime * 1000,
      data: result[0].values?.map((v) => Math.round(v[0] * 1000)),
      axisLine: {
        show: true,
      },
    },
    yAxis: {
      type: "value",
      axisLabel: {
        formatter: formatValue,
      },
      axisLine: {
        // symbol: "arrow",
        show: true,
        // lineStyle: {
        //   type: "dashed",
        //   color: "rgba(0, 0, 0, 0.5)",
        // },
      },
    },
    tooltip: {
      show: true,
      trigger: "item",
      transitionDuration: 0,
      axisPointer: {
        type: "cross",
        // snap: true,
      },
    },
    series: result.map((series) => ({
      name: formatSeries(series.metric),
      // data: series.values?.map((v) => [v[0] * 1000, parseFloat(v[1])]),
      data: series.values?.map((v) => parseFloat(v[1])),
      type: "line",
      stack: displayMode === "stacked" ? "total" : undefined,
      // showSymbol: false,
      // fill: displayMode === "stacked" ? "tozeroy" : undefined,
    })),
  };

  console.log(option);

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
      <EChart
        option={option}
        // theme={chartsTheme.echartsTheme}
        // onEvents={handleEvents}
        // _instance={chartRef}
        // syncGroup={syncGroup}
      />
    </Box>
  );
};

const formatValue = (y: number | null): string => {
  if (y === null) {
    return "null";
  }
  const absY = Math.abs(y);

  if (absY >= 1e24) {
    return (y / 1e24).toFixed(2) + "Y";
  } else if (absY >= 1e21) {
    return (y / 1e21).toFixed(2) + "Z";
  } else if (absY >= 1e18) {
    return (y / 1e18).toFixed(2) + "E";
  } else if (absY >= 1e15) {
    return (y / 1e15).toFixed(2) + "P";
  } else if (absY >= 1e12) {
    return (y / 1e12).toFixed(2) + "T";
  } else if (absY >= 1e9) {
    return (y / 1e9).toFixed(2) + "G";
  } else if (absY >= 1e6) {
    return (y / 1e6).toFixed(2) + "M";
  } else if (absY >= 1e3) {
    return (y / 1e3).toFixed(2) + "k";
  } else if (absY >= 1) {
    return y.toFixed(2);
  } else if (absY === 0) {
    return y.toFixed(2);
  } else if (absY < 1e-23) {
    return (y / 1e-24).toFixed(2) + "y";
  } else if (absY < 1e-20) {
    return (y / 1e-21).toFixed(2) + "z";
  } else if (absY < 1e-17) {
    return (y / 1e-18).toFixed(2) + "a";
  } else if (absY < 1e-14) {
    return (y / 1e-15).toFixed(2) + "f";
  } else if (absY < 1e-11) {
    return (y / 1e-12).toFixed(2) + "p";
  } else if (absY < 1e-8) {
    return (y / 1e-9).toFixed(2) + "n";
  } else if (absY < 1e-5) {
    return (y / 1e-6).toFixed(2) + "µ";
  } else if (absY < 1e-2) {
    return (y / 1e-3).toFixed(2) + "m";
  } else if (absY <= 1) {
    return y.toFixed(2);
  }
  throw Error("couldn't format a value, this is a bug");
};

export default Graph;
