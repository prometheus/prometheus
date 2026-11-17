import React, { FC } from "react";
import { Histogram } from "../../types/types";
import { getHistogramLayout, bucketRangeString } from "./HistogramHelpers";
import classes from "./HistogramChart.module.css";
import { Tooltip } from "@mantine/core";

interface HistogramChartProps {
  histogram: Histogram;
  index: number;
  scale: string;
}

const HistogramChart: FC<HistogramChartProps> = ({
  index,
  histogram,
  scale,
}) => {
  const { buckets } = histogram;
  if (!buckets || buckets.length === 0) {
    return <div>No data</div>;
  }
  const formatter = Intl.NumberFormat("en", { notation: "compact" });

  const layout = getHistogramLayout(buckets, scale);
  if (layout === null) {
    return (
      <div role="alert">Invalid histogram bucket boundaries or counts.</div>
    );
  }
  const { countMax, bars } = layout;
  const zeroAxisLeft = `${layout.zeroAxisLeft}%`;
  const zeroAxis = layout.zeroAxisLeft > 5 && layout.zeroAxisLeft < 95;
  const rangeMin = buckets[0][1];
  const rangeMax = buckets[buckets.length - 1][2];
  const hasNegativeBuckets = rangeMin === "-Inf" || Number(rangeMin) < 0;
  const formatBoundary = (value: string) =>
    Number.isFinite(Number(value)) ? formatter.format(Number(value)) : value;

  return (
    <div className={classes.histogramYWrapper}>
      <div className={classes.histogramYLabels}>
        {[1, 0.75, 0.5, 0.25].map((i) => (
          <div key={i} className={classes.histogramYLabel}>
            {scale === "linear" ? "" : formatter.format(countMax * i)}
          </div>
        ))}
        <div key={0} className={classes.histogramYLabel} style={{ height: 0 }}>
          0
        </div>
      </div>
      <div className={classes.histogramXWrapper}>
        <div className={classes.histogramContainer}>
          {[0, 0.25, 0.5, 0.75, 1].map((i) => (
            <React.Fragment key={i}>
              <div
                className={classes.histogramYGrid}
                style={{ bottom: i * 100 + "%" }}
              ></div>
              <div
                className={classes.histogramYTick}
                style={{ bottom: i * 100 + "%" }}
              ></div>
              <div
                className={classes.histogramXGrid}
                style={{ left: i * 100 + "%" }}
              ></div>
            </React.Fragment>
          ))}
          <div className={classes.histogramXTick} style={{ left: "0%" }}></div>
          <div
            className={classes.histogramXTick}
            style={{ left: zeroAxisLeft }}
          ></div>
          <div
            className={classes.histogramXGrid}
            style={{ left: zeroAxisLeft }}
          ></div>
          <div
            className={classes.histogramXTick}
            style={{ left: "100%" }}
          ></div>

          {buckets.map((bucket, bIdx) => {
            const bucketIdx = `bucket-${index}-${bIdx}-${Math.ceil(parseFloat(bucket[3]) * 100)}`;
            return (
              <Tooltip label={`range: ${bucketRangeString(bucket)}`} key={bIdx}>
                <div
                  id={bucketIdx}
                  className={classes.histogramBucketSlot}
                  style={{
                    left: `${bars[bIdx].left}%`,
                    width: `${bars[bIdx].width}%`,
                  }}
                >
                  <div
                    id={bucketIdx}
                    className={classes.histogramBucket}
                    style={{ height: `${bars[bIdx].height}%` }}
                  ></div>
                </div>
              </Tooltip>
            );
          })}

          <div className={classes.histogramAxes}></div>
        </div>
        <div className={classes.histogramXLabels}>
          <div className={classes.histogramXLabel}>
            <React.Fragment>
              <div style={{ position: "absolute", left: 0 }}>
                {formatBoundary(rangeMin)}
              </div>
              {hasNegativeBuckets && zeroAxis && (
                <div style={{ position: "absolute", left: zeroAxisLeft }}>
                  0
                </div>
              )}
              <div style={{ position: "absolute", right: 0 }}>
                {formatBoundary(rangeMax)}
              </div>
            </React.Fragment>
          </div>
        </div>
      </div>
    </div>
  );
};

export default HistogramChart;
