import React, { FC } from "react";
import { Histogram } from "../../types/types";
import {
  calculateDefaultExpBucketWidth,
  findMinPositive,
  findMaxNegative,
  findZeroAxisLeft,
  showZeroAxis,
  findZeroBucket,
  bucketRangeString,
} from "./HistogramHelpers";
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

  // For linear scales, the count of a histogram bucket is represented by its area rather than its height. This means it considers
  // both the count and the range (width) of the bucket. For this, we can set the height of the bucket proportional
  // to its frequency density (fd). The fd is the count of the bucket divided by the width of the bucket.
  const fds = [];
  for (const bucket of buckets) {
    const left = parseFloat(bucket[1]);
    const right = parseFloat(bucket[2]);
    const count = parseFloat(bucket[3]);
    const width = right - left;

    // This happens when a user want observations of precisely zero to be included in the zero bucket
    if (width === 0) {
      fds.push(0);
      continue;
    }
    fds.push(count / width);
  }
  const fdMax = Math.max(...fds);

  const first = buckets[0];
  const last = buckets[buckets.length - 1];

  const rangeMax = parseFloat(last[2]);
  const rangeMin = parseFloat(first[1]);
  const countMax = Math.max(...buckets.map((b) => parseFloat(b[3])));

  const defaultExpBucketWidth = calculateDefaultExpBucketWidth(last, buckets);

  const maxPositive = rangeMax > 0 ? rangeMax : 0;
  const minPositive = findMinPositive(buckets);
  const maxNegative = findMaxNegative(buckets);
  const minNegative = parseFloat(first[1]) < 0 ? parseFloat(first[1]) : 0;

  // Calculate the borders of positive and negative buckets in the exponential scale from left to right
  const startNegative =
    minNegative !== 0 ? -Math.log(Math.abs(minNegative)) : 0;
  const endNegative = maxNegative !== 0 ? -Math.log(Math.abs(maxNegative)) : 0;
  const startPositive = minPositive !== 0 ? Math.log(minPositive) : 0;
  const endPositive = maxPositive !== 0 ? Math.log(maxPositive) : 0;

  // Calculate the width of negative, positive, and all exponential bucket ranges on the x-axis
  const xWidthNegative = endNegative - startNegative;
  const xWidthPositive = endPositive - startPositive;
  const xWidthTotal = xWidthNegative + defaultExpBucketWidth + xWidthPositive;

  const zeroBucketIdx = findZeroBucket(buckets);
  const zeroAxisLeft = findZeroAxisLeft(
    scale,
    rangeMin,
    rangeMax,
    minPositive,
    maxNegative,
    zeroBucketIdx,
    xWidthNegative,
    xWidthTotal,
    defaultExpBucketWidth,
  );
  const zeroAxis = showZeroAxis(zeroAxisLeft);

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

          <RenderHistogramBars
            buckets={buckets}
            scale={scale}
            rangeMin={rangeMin}
            rangeMax={rangeMax}
            index={index}
            fds={fds}
            fdMax={fdMax}
            countMax={countMax}
            defaultExpBucketWidth={defaultExpBucketWidth}
            minPositive={minPositive}
            maxNegative={maxNegative}
            startPositive={startPositive}
            startNegative={startNegative}
            xWidthNegative={xWidthNegative}
            xWidthTotal={xWidthTotal}
          />

          <div className={classes.histogramAxes}></div>
        </div>
        <div className={classes.histogramXLabels}>
          <div className={classes.histogramXLabel}>
            <React.Fragment>
              <div style={{ position: "absolute", left: 0 }}>
                {formatter.format(rangeMin)}
              </div>
              {rangeMin < 0 && zeroAxis && (
                <div style={{ position: "absolute", left: zeroAxisLeft }}>
                  0
                </div>
              )}
              <div style={{ position: "absolute", right: 0 }}>
                {formatter.format(rangeMax)}
              </div>
            </React.Fragment>
          </div>
        </div>
      </div>
    </div>
  );
};

interface RenderHistogramProps {
  buckets: [number, string, string, string][];
  scale: string;
  rangeMin: number;
  rangeMax: number;
  index: number;
  fds: number[];
  fdMax: number;
  countMax: number;
  defaultExpBucketWidth: number;
  minPositive: number;
  maxNegative: number;
  startPositive: number;
  startNegative: number;
  xWidthNegative: number;
  xWidthTotal: number;
}

const RenderHistogramBars: FC<RenderHistogramProps> = ({
  buckets,
  scale,
  rangeMin,
  rangeMax,
  index,
  fds,
  fdMax,
  countMax,
  defaultExpBucketWidth,
  minPositive,
  maxNegative,
  startPositive,
  startNegative,
  xWidthNegative,
  xWidthTotal,
}) => {
  return (
    <React.Fragment>
      {buckets.map((b, bIdx) => {
        const left = parseFloat(b[1]);
        const right = parseFloat(b[2]);
        const count = parseFloat(b[3]);
        const bucketIdx = `bucket-${index}-${bIdx}-${Math.ceil(parseFloat(b[3]) * 100)}`;

        const logWidth = Math.abs(
          Math.log(Math.abs(right)) - Math.log(Math.abs(left)),
        );
        const expBucketWidth =
          logWidth === 0 ? defaultExpBucketWidth : logWidth;

        let bucketWidth = "";
        let bucketLeft = "";
        let bucketHeight = "";

        switch (scale) {
          case "linear": {
            bucketWidth = ((right - left) / (rangeMax - rangeMin)) * 100 + "%";
            bucketLeft =
              ((left - rangeMin) / (rangeMax - rangeMin)) * 100 + "%";
            if (left === 0 && right === 0) {
              bucketLeft = "0%"; // do not render zero-width zero bucket
              bucketWidth = "0%";
            }
            bucketHeight = (fds[bIdx] / fdMax) * 100 + "%";
            break;
          }
          case "exponential": {
            let adjust = 0; // if buckets are all positive/negative, we need to remove the width of the zero bucket
            if (minPositive === 0 || maxNegative === 0) {
              adjust = defaultExpBucketWidth;
            }
            bucketWidth = (expBucketWidth / (xWidthTotal - adjust)) * 100 + "%";
            if (left < 0) {
              // negative buckets boundary
              bucketLeft =
                (-(Math.log(Math.abs(left)) + startNegative) /
                  (xWidthTotal - adjust)) *
                  100 +
                "%";
            } else {
              // positive buckets boundary
              bucketLeft =
                ((Math.log(left) -
                  startPositive +
                  defaultExpBucketWidth +
                  xWidthNegative -
                  adjust) /
                  (xWidthTotal - adjust)) *
                  100 +
                "%";
            }
            if (left < 0 && right > 0) {
              // if the bucket crosses the zero axis
              bucketLeft = (xWidthNegative / xWidthTotal) * 100 + "%";
            }
            if (left === 0 && right === 0) {
              // do not render zero width zero bucket
              bucketLeft = "0%";
              bucketWidth = "0%";
            }

            bucketHeight = (count / countMax) * 100 + "%";
            break;
          }
          default:
            throw new Error("Invalid scale");
        }

        return (
          <Tooltip label={`range: ${bucketRangeString(b)}`} key={bIdx}>
            <div
              id={bucketIdx}
              className={classes.histogramBucketSlot}
              style={{
                left: bucketLeft,
                width: bucketWidth,
              }}
            >
              <div
                id={bucketIdx}
                className={classes.histogramBucket}
                style={{
                  height: bucketHeight,
                }}
              ></div>
            </div>
          </Tooltip>
        );
      })}
    </React.Fragment>
  );
};

export default HistogramChart;
