import { FC, useEffect, useState } from "react";
import { RangeSamples } from "../../api/responseTypes/query";
import classes from "./Graph.module.css";
import { GraphDisplayMode } from "../../state/queryPageSlice";
import uPlot from "uplot";
import UplotReact from "uplot-react";
import { useSettings } from "../../state/settingsSlice";
import { useComputedColorScheme } from "@mantine/core";

import "uplot/dist/uPlot.min.css";
import "./uplot.css";
import { getUPlotData, getUPlotOptions } from "./uPlotChartHelpers";

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

const UPlotChart: FC<UPlotChartProps> = ({
  data,
  range: { startTime, endTime, resolution },
  width,
  onSelectRange,
}) => {
  const [options, setOptions] = useState<uPlot.Options | null>(null);
  const { useLocalTime } = useSettings();
  const theme = useComputedColorScheme();

  useEffect(() => {
    if (width === 0) {
      return;
    }

    setOptions(
      getUPlotOptions(
        width,
        data,
        useLocalTime,
        theme === "light",
        onSelectRange
      )
    );
  }, [width, data, useLocalTime, theme, onSelectRange]);

  const seriesData: uPlot.AlignedData = getUPlotData(
    data,
    startTime,
    endTime,
    resolution
  );

  if (options === null) {
    return;
  }

  return (
    <UplotReact
      options={options}
      data={seriesData}
      className={classes.uplotChart}
    />
  );
};

export default UPlotChart;
