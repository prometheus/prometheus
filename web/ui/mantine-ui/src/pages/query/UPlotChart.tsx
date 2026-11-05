import { FC, useEffect, useMemo, useRef } from "react";
import { RangeSamples } from "../../api/responseTypes/query";
import classes from "./Graph.module.css";
import { GraphDisplayMode } from "../../state/queryPageSlice";
import uPlot from "uplot";
import { useSettings } from "../../state/settingsSlice";
import { useComputedColorScheme, Text } from "@mantine/core";

import "uplot/dist/uPlot.min.css";
import "./uplot.css";
import { getUPlotData, getUPlotOptions } from "./uPlotChartHelpers";
import { setStackedOpts } from "./uPlotStackHelpers";

export interface UPlotChartRange {
  startTime: number;
  endTime: number;
  resolution: number;
}

export interface UPlotChartProps {
  data: RangeSamples[];
  range: UPlotChartRange;
  width: number;
  showExemplars: boolean;
  displayMode: GraphDisplayMode;
  yAxisMin: number | null;
  onSelectRange: (start: number, end: number) => void;
}

// This wrapper component translates the incoming Prometheus RangeSamples[] data to the
// uPlot format and sets up the uPlot options object depending on the UI settings.
const UPlotChart: FC<UPlotChartProps> = ({
  data,
  range: { startTime, endTime, resolution },
  width,
  displayMode,
  yAxisMin,
  onSelectRange,
}) => {
  const hostRef = useRef<HTMLDivElement>(null);
  const { useLocalTime } = useSettings();
  const theme = useComputedColorScheme();

  const chartSpec = useMemo(() => {
    if (width === 0) {
      return null;
    }

    const seriesData: uPlot.AlignedData = getUPlotData(
      data,
      startTime,
      endTime,
      resolution,
    );

    const opts = getUPlotOptions(
      seriesData,
      width,
      data,
      useLocalTime,
      yAxisMin,
      theme === "light",
      onSelectRange,
    );

    return {
      options: opts,
      data:
        displayMode === GraphDisplayMode.Stacked
          ? setStackedOpts(opts, seriesData).data
          : seriesData,
    };
  }, [
    width,
    data,
    displayMode,
    startTime,
    endTime,
    resolution,
    useLocalTime,
    theme,
    onSelectRange,
    yAxisMin,
  ]);

  useEffect(() => {
    if (chartSpec === null || hostRef.current === null) {
      return;
    }
    const chart = new uPlot(chartSpec.options, chartSpec.data, hostRef.current);
    return () => chart.destroy();
  }, [chartSpec]);

  if (chartSpec === null) {
    return null;
  }

  return (
    <>
      <div ref={hostRef} className={classes.uplotChart} />
      <Text fz="xs" c="dimmed" ml={40} mt={-25} mb="lg">
        Click: show single series,{" "}
        {navigator.userAgent.includes("Mac") ? "⌘" : "Ctrl"} + click: hide
        single series
      </Text>
    </>
  );
};

export default UPlotChart;
