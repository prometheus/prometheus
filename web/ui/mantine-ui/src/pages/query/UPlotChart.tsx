import { FC, useEffect, useState } from "react";
import { RangeSamples } from "../../api/responseTypes/query";
import classes from "./Graph.module.css";
import { GraphDisplayMode } from "../../state/queryPageSlice";
import uPlot from "uplot";
import UplotReact from "uplot-react";
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
  onSelectRange: (start: number, end: number) => void;
}

// This wrapper component translates the incoming Prometheus RangeSamples[] data to the
// uPlot format and sets up the uPlot options object depending on the UI settings.
const UPlotChart: FC<UPlotChartProps> = ({
  data,
  range: { startTime, endTime, resolution },
  width,
  displayMode,
  onSelectRange,
}) => {
  const [options, setOptions] = useState<uPlot.Options | null>(null);
  const [processedData, setProcessedData] = useState<uPlot.AlignedData | null>(
    null
  );
  const { useLocalTime } = useSettings();
  const theme = useComputedColorScheme();

  useEffect(() => {
    if (width === 0) {
      return;
    }

    const seriesData: uPlot.AlignedData = getUPlotData(
      data,
      startTime,
      endTime,
      resolution
    );

    const opts = getUPlotOptions(
      seriesData,
      width,
      data,
      useLocalTime,
      theme === "light",
      onSelectRange
    );

    if (displayMode === GraphDisplayMode.Stacked) {
      setProcessedData(setStackedOpts(opts, seriesData).data);
    } else {
      setProcessedData(seriesData);
    }

    setOptions(opts);
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
  ]);

  if (options === null || processedData === null) {
    return;
  }

  return (
    <>
      <UplotReact
        options={options}
        data={processedData}
        className={classes.uplotChart}
      />
      <Text fz="xs" c="dimmed" ml={40} mt={-25} mb="lg">
        Click: show single series,{" "}
        {navigator.userAgent.includes("Mac") ? "⌘" : "Ctrl"} + click: hide
        single series
      </Text>
    </>
  );
};

export default UPlotChart;
