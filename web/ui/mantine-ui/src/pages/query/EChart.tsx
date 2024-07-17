// Copyright 2023 The Perses Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import React, { useEffect, useLayoutEffect, useRef } from "react";
import {
  ECharts,
  EChartsCoreOption,
  EChartsOption,
  init,
  connect,
  BarSeriesOption,
  LineSeriesOption,
  GaugeSeriesOption,
} from "echarts";
import isEqual from "lodash/isEqual";
import debounce from "lodash/debounce";
import { Box } from "@mantine/core";

// https://github.com/apache/echarts/issues/12489#issuecomment-643185207
export interface EChartsTheme extends EChartsOption {
  bar?: BarSeriesOption;
  line?: LineSeriesOption;
  gauge?: GaugeSeriesOption;
}

// see docs for info about each property: https://echarts.apache.org/en/api.html#events
export interface MouseEventsParameters<T> {
  componentType: string;
  seriesType: string;
  seriesIndex: number;
  seriesName: string;
  name: string;
  dataIndex: number;
  data: Record<string, unknown> & T;
  dataType: string;
  value: number | number[];
  color: string;
  info: Record<string, unknown>;
}

type OnEventFunction<T> = (
  params: MouseEventsParameters<T>,
  // This is potentially undefined for testing purposes
  instance?: ECharts
) => void;

const mouseEvents = [
  "click",
  "dblclick",
  "mousedown",
  "mousemove",
  "mouseup",
  "mouseover",
  "mouseout",
  "globalout",
  "contextmenu",
] as const;

export type MouseEventName = (typeof mouseEvents)[number];

// batch event types
export interface DataZoomPayloadBatchItem {
  dataZoomId: string;
  // start and end not returned unless dataZoom is based on percentProp,
  // which is for cases when a dataZoom component controls multiple axes
  start?: number;
  end?: number;
  // startValue and endValue return data index for 'category' axes,
  // for axis types 'value' and 'time', actual values are returned
  startValue?: number;
  endValue?: number;
}

export interface HighlightPayloadBatchItem {
  dataIndex: number;
  dataIndexInside: number;
  seriesIndex: number;
  // highlight action can effect multiple connected charts
  escapeConnect?: boolean;
  // whether blur state was triggered
  notBlur?: boolean;
}

export interface BatchEventsParameters {
  type: BatchEventName;
  batch: DataZoomPayloadBatchItem[] & HighlightPayloadBatchItem[];
}

type OnBatchEventFunction = (params: BatchEventsParameters) => void;

const batchEvents = ["datazoom", "downplay", "highlight"] as const;

export type BatchEventName = (typeof batchEvents)[number];

type ChartEventName = "finished";

type EventName = MouseEventName | ChartEventName | BatchEventName;

export type OnEventsType<T> = {
  [mouseEventName in MouseEventName]?: OnEventFunction<T>;
} & {
  [batchEventName in BatchEventName]?: OnBatchEventFunction;
} & {
  [eventName in ChartEventName]?: () => void;
};

export interface EChartsProps<T> {
  option: EChartsCoreOption;
  theme?: string | EChartsTheme;
  renderer?: "canvas" | "svg";
  onEvents?: OnEventsType<T>;
  _instance?: React.MutableRefObject<ECharts | undefined>;
  syncGroup?: string;
  onChartInitialized?: (instance: ECharts) => void;
}

export const chartHeight = 500;
export const legendHeight = (numSeries: number) => numSeries * 24;
export const legendMargin = 25;

export const EChart = React.memo(function EChart<T>({
  option,
  theme,
  renderer,
  onEvents,
  _instance,
  syncGroup,
  onChartInitialized,
}: EChartsProps<T>) {
  const initialOption = useRef<EChartsCoreOption>(option);
  const prevOption = useRef<EChartsCoreOption>(option);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const chartElement = useRef<ECharts | null>(null);

  // Initialize chart, dispose on unmount
  useLayoutEffect(() => {
    if (containerRef.current === null || chartElement.current !== null) return;
    chartElement.current = init(containerRef.current, theme, {
      renderer: renderer ?? "canvas",
    });
    if (chartElement.current === undefined) return;
    chartElement.current.setOption(initialOption.current, true);
    onChartInitialized?.(chartElement.current);
    if (_instance !== undefined) {
      _instance.current = chartElement.current;
    }
    return () => {
      if (chartElement.current === null) return;
      chartElement.current.dispose();
      chartElement.current = null;
    };
  }, [_instance, onChartInitialized, theme, renderer]);

  // When syncGroup is explicitly set, charts within same group share interactions such as crosshair
  useEffect(() => {
    if (!chartElement.current || !syncGroup) return;
    chartElement.current.group = syncGroup;
    connect([chartElement.current]); // more info: https://echarts.apache.org/en/api.html#echarts.connect
  }, [syncGroup, chartElement]);

  // Update chart data when option changes
  useEffect(() => {
    if (prevOption.current === undefined || isEqual(prevOption.current, option))
      return;
    if (!chartElement.current) return;
    chartElement.current.setOption(option, true);
    prevOption.current = option;
  }, [option]);

  // Resize chart, cleanup listener on unmount
  useLayoutEffect(() => {
    const updateSize = debounce(() => {
      if (!chartElement.current) return;
      chartElement.current.resize();
    }, 200);
    window.addEventListener("resize", updateSize);
    updateSize();
    return () => {
      window.removeEventListener("resize", updateSize);
    };
  }, []);

  // Bind and unbind chart events passed as prop
  useEffect(() => {
    const chart = chartElement.current;
    if (!chart || onEvents === undefined) return;
    bindEvents(chart, onEvents);
    return () => {
      if (chart === undefined) return;
      if (chart.isDisposed() === true) return;
      for (const event in onEvents) {
        chart.off(event);
      }
    };
  }, [onEvents]);

  // // TODO: re-evaluate how this is triggered. It's technically working right
  // // now because the sx prop is an object that gets re-created, but that also
  // // means it runs unnecessarily some of the time and theoretically might
  // // not run in some other cases. Maybe it should use a resize observer?
  // useEffect(() => {
  //   // TODO: fix this debouncing. This likely isn't working as intended because
  //   // the debounced function is re-created every time this useEffect is called.
  //   const updateSize = debounce(
  //     () => {
  //       if (!chartElement.current) return;
  //       chartElement.current.resize();
  //     },
  //     200,
  //     {
  //       leading: true,
  //     }
  //   );
  //   updateSize();
  // }, [sx]);

  return (
    <Box
      w="100%"
      h={
        chartHeight +
        legendMargin +
        legendHeight((option as { series: unknown[] }).series.length)
      }
      ref={containerRef}
    ></Box>
  );
});

// Validate event config and bind custom events
function bindEvents<T>(instance: ECharts, events?: OnEventsType<T>) {
  if (events === undefined) return;

  function bindEvent(eventName: EventName, OnEventFunction: unknown) {
    if (typeof OnEventFunction === "function") {
      if (isMouseEvent(eventName)) {
        instance.on(eventName, (params) => OnEventFunction(params, instance));
      } else if (isBatchEvent(eventName)) {
        instance.on(eventName, (params) => OnEventFunction(params));
      } else {
        instance.on(eventName, () => OnEventFunction(null, instance));
      }
    }
  }

  for (const eventName in events) {
    if (Object.prototype.hasOwnProperty.call(events, eventName)) {
      const customEvent = events[eventName as EventName] ?? null;
      if (customEvent) {
        bindEvent(eventName as EventName, customEvent);
      }
    }
  }
}

function isMouseEvent(eventName: EventName): eventName is MouseEventName {
  return (mouseEvents as readonly string[]).includes(eventName);
}

function isBatchEvent(eventName: EventName): eventName is BatchEventName {
  return (batchEvents as readonly string[]).includes(eventName);
}
