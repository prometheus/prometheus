import React, { FC } from 'react';
import { UncontrolledTooltip } from 'reactstrap';
import { Histogram } from '../../types/types';
import { bucketRangeString } from './DataTable';

type ScaleType = 'linear' | 'exponential';

type closestToZeroType = {
  closest: number;
  closestIdx: number;
};

const HistogramChart: FC<{ histogram: Histogram; index: number; scale: ScaleType }> = ({ index, histogram, scale }) => {
  const { buckets } = histogram;
  const formatter = Intl.NumberFormat('en', { notation: 'compact' });

  const rangeMax = buckets ? parseFloat(buckets[buckets.length - 1][2]) : 0;
  const rangeMin = buckets ? parseFloat(buckets[0][1]) : 0;

  const expBucketWidth = buckets
    ? Math.abs(
        Math.log(Math.abs(parseFloat(buckets[buckets.length - 1][2]))) -
          Math.log(Math.abs(parseFloat(buckets[buckets.length - 1][1])))
      )
    : 0; //bw

  const countMax = buckets ? buckets.map((b) => parseFloat(b[3])).reduce((a, b) => Math.max(a, b)) : 0;

  // The count of a histogram bucket is represented by its area rather than its height. This means it considers
  // both the count and the width (range) of the bucket. For this, we can set the height of the bucket proportional
  // to its frequency density (fd). The fd is the count of the bucket divided by the width of the bucket.

  // Frequency density histograms are necessary when the bucekts are of unequal width. If the buckets are collected in
  // intervals of equal width, then there is no difference between frequency and frequency density histograms.
  const fds = buckets
    ? buckets.map((b) => {
        return parseFloat(b[3]) / (parseFloat(b[2]) - parseFloat(b[1]));
      })
    : [];
  const fdMax = fds.reduce((a, b) => Math.max(a, b));
  const closestToZero = buckets ? findClosestToZero(buckets.map((b) => parseFloat(b[1]))) : { closest: 0, closestIdx: 0 };

  const zeroAxisLeft =
    scale === 'linear'
      ? ((0 - rangeMin) / (rangeMax - rangeMin)) * 100 + '%'
      : (closestToZero.closestIdx / (buckets ? buckets.length : 1)) * 100 + '%';

  const maxPositive = buckets ? parseFloat(buckets[buckets.length - 1][2]) : 0;
  const minPositive = buckets ? parseFloat(buckets[closestToZero.closestIdx + 1][1]) : 0;
  const maxNegative = buckets ? parseFloat(buckets[closestToZero.closestIdx - 1][2]) : 0;
  const minNegative = buckets ? parseFloat(buckets[0][1]) : 0;
  const startNegative = buckets ? -Math.log(Math.abs(minNegative)) : 0; //start_neg
  const endNegative = buckets ? -Math.log(Math.abs(maxNegative)) : 0; //end_neg
  const startPositive = buckets ? Math.log(minPositive) : 0; //start_pos
  const endPositive = buckets ? Math.log(maxPositive) : 0; //end_pos

  const widthNegative = endNegative - startNegative; //width_neg
  const widthPositive = endPositive - startPositive; //width_pos
  const widthTotal = widthNegative + expBucketWidth + widthPositive; //width_total

  function findClosestToZero(numbers: number[]): closestToZeroType {
    let closest = numbers[0];
    let closestIdx = 0;
    let minDistance = Math.abs(numbers[0]);

    for (let i = 1; i < numbers.length; i++) {
      const distance = Math.abs(numbers[i]);

      if (distance < minDistance) {
        closest = numbers[i];
        minDistance = distance;
        closestIdx = i;
      } else if (distance === minDistance && numbers[i] > 0) {
        closest = numbers[i];
        closestIdx = i;
      }
    }

    return { closest, closestIdx };
  }

  return (
    <div className="histogram-y-wrapper">
      <div className="histogram-y-labels">
        {[1, 0.75, 0.5, 0.25].map((i) => (
          <div key={i} className="histogram-y-label">
            {scale === 'linear' ? '' : formatter.format(countMax * i)}
          </div>
        ))}
        <div key={0} className="histogram-y-label" style={{ height: 0 }}>
          0
        </div>
      </div>
      <div className="histogram-x-wrapper">
        <div className="histogram-container">
          {[0, 0.25, 0.5, 0.75, 1].map((i) => (
            <React.Fragment key={i}>
              <div className="histogram-y-grid" style={{ bottom: i * 100 + '%' }}></div>
              <div className="histogram-y-tick" style={{ bottom: i * 100 + '%' }}></div>
              <div className="histogram-x-grid" style={{ left: i * 100 + '%' }}></div>
            </React.Fragment>
          ))}
          <div className="histogram-x-tick" style={{ left: '0%' }}></div>
          <div className="histogram-x-tick" style={{ left: zeroAxisLeft }}></div>
          <div className="histogram-x-grid" style={{ left: zeroAxisLeft }}></div>
          <div className="histogram-x-tick" style={{ left: '100%' }}></div>

          {buckets && (
            <RenderHistogramBars
              buckets={buckets}
              scale={scale}
              rangeMin={rangeMin}
              rangeMax={rangeMax}
              index={index}
              fds={fds}
              fdMax={fdMax}
              countMax={countMax}
              bw={expBucketWidth}
              maxPositive={maxPositive}
              startPositive={startPositive}
              startNegative={startNegative}
              endPositive={endPositive}
              widthNegative={widthNegative}
              widthTotal={widthTotal}
            />
          )}

          <div className="histogram-axes"></div>
        </div>
        <div className="histogram-x-labels">
          <div className="histogram-x-label">
            <React.Fragment>
              <div style={{ position: 'absolute', left: 0 }}>{formatter.format(rangeMin)}</div>
              {rangeMin < 0 && <div style={{ position: 'absolute', left: zeroAxisLeft }}>0</div>}
              <div style={{ position: 'absolute', right: 0 }}>{formatter.format(rangeMax)}</div>
            </React.Fragment>
          </div>
        </div>
      </div>
    </div>
  );
};

interface RenderHistogramProps {
  buckets: [number, string, string, string][];
  scale: ScaleType;
  rangeMin: number;
  rangeMax: number;
  index: number;
  fds: number[];
  fdMax: number;
  countMax: number;
  bw: number;
  maxPositive: number;
  startPositive: number;
  startNegative: number;
  endPositive: number;
  widthNegative: number;
  widthTotal: number;
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
  bw,
  maxPositive,
  startPositive,
  startNegative,
  endPositive,
  widthNegative,
  widthTotal,
}) => {
  return (
    <React.Fragment>
      {buckets.map((b, bIdx) => {
        const left = parseFloat(b[1]);
        const right = parseFloat(b[2]);
        const count = parseFloat(b[3]);
        console.log('new stuff');

        const bucketIdx = `bucket-${index}-${bIdx}-${Math.ceil(parseFloat(b[3]) * 100)}`;
        const bucketWidth =
          scale === 'linear' ? ((right - left) / (rangeMax - rangeMin)) * 100 + '%' : (bw / widthTotal) * 100 + '%';
        const bucketLeft =
          scale === 'linear'
            ? ((left - rangeMin) / (rangeMax - rangeMin)) * 100 + '%'
            : left < 0
            ? (-(Math.log(Math.abs(left)) + startNegative) / widthTotal) * 100 + '%' // negative buckets boundary
            : ((Math.log(left) - startPositive + bw + widthNegative) / widthTotal) * 100 + '%'; // positive buckets boundary
        const bucketHeight = scale === 'linear' ? (fds[bIdx] / fdMax) * 100 + '%' : (count / countMax) * 100 + '%';

        console.log(
          'left',
          left,
          '\n',
          'right',
          right,
          '\n',
          'count',
          count,
          '\n',
          'bucketWidth',
          bucketWidth,
          '\n',
          'bucketLeft',
          bucketLeft,
          '\n',
          'bw',
          bw,
          '\n',
          'widthTotal',
          widthTotal,
          '\n',
          'widthNegative',
          widthNegative,
          '\n',
          'startPositive',
          startPositive
        );
        return (
          <React.Fragment key={bIdx}>
            <div
              id={bucketIdx}
              className="histogram-bucket-slot"
              style={{
                left: bucketLeft,
                width: bucketWidth,
              }}
            >
              <div
                id={bucketIdx}
                className="histogram-bucket"
                style={{
                  height: bucketHeight,
                }}
              ></div>
              <UncontrolledTooltip
                style={{ maxWidth: 'unset', padding: 10, textAlign: 'left' }}
                placement="bottom"
                target={bucketIdx}
              >
                <strong>range:</strong> {bucketRangeString(b)}
                <br />
                <strong>count:</strong> {count}
              </UncontrolledTooltip>
            </div>
          </React.Fragment>
        );
      })}
    </React.Fragment>
  );
};

export default HistogramChart;
