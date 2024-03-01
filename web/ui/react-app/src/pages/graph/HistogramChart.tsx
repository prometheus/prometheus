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

  // The count of a histogram bucket is represented by the frequency distribution (FD) rather than its height.
  // This means it considers both the count and the width of the bar. The FD is calculated as the count divided by
  // the width (range) of the bucket. Therefore, we can set a height proportional to fd.
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
            {formatter.format(fdMax * i)}
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
}

const RenderHistogramBars: FC<RenderHistogramProps> = ({ buckets, scale, rangeMin, rangeMax, index, fds, fdMax }) => {
  return (
    <React.Fragment>
      {buckets.map((b, bIdx) => {
        const bucketIdx = `bucket-${index}-${bIdx}-${Math.ceil(parseFloat(b[3]) * 100)}`;
        const bucketLeft =
          scale === 'linear'
            ? ((parseFloat(b[1]) - rangeMin) / (rangeMax - rangeMin)) * 100 + '%'
            : (bIdx / buckets.length) * 100 + '%';
        const bucketWidth =
          scale === 'linear'
            ? ((parseFloat(b[2]) - parseFloat(b[1])) / (rangeMax - rangeMin)) * 100 + '%'
            : 100 / buckets.length + '%';
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
                  height: (fds[bIdx] / fdMax) * 100 + '%',
                }}
              ></div>
              <UncontrolledTooltip
                style={{ maxWidth: 'unset', padding: 10, textAlign: 'left' }}
                placement="bottom"
                target={bucketIdx}
              >
                <strong>range:</strong> {bucketRangeString(b)}
                <br />
                <strong>count:</strong> {b[3]}
              </UncontrolledTooltip>
            </div>
          </React.Fragment>
        );
      })}
    </React.Fragment>
  );
};

export default HistogramChart;
