import React, { FC } from 'react';
import { UncontrolledTooltip } from 'reactstrap';
import { Histogram } from '../../types/types';
import { bucketRangeString } from './DataTable';

type ScaleType = 'linear' | 'exponential';

const HistogramChart: FC<{ histogram: Histogram; index: number; scale: ScaleType }> = ({ index, histogram, scale }) => {
  const { buckets } = histogram;
  if (!buckets) {
    return <div>No data</div>;
  }
  const formatter = Intl.NumberFormat('en', { notation: 'compact' });

  const rangeMax = parseFloat(buckets[buckets.length - 1][2]);
  const rangeMin = parseFloat(buckets[0][1]);

  const expBucketWidth = Math.abs(
    Math.log(Math.abs(parseFloat(buckets[buckets.length - 1][2]))) -
      Math.log(Math.abs(parseFloat(buckets[buckets.length - 1][1])))
  ); //bw
  const countMax = buckets.map((b) => parseFloat(b[3])).reduce((a, b) => Math.max(a, b));

  // The count of a histogram bucket is represented by its area rather than its height. This means it considers
  // both the count and the width (range) of the bucket. For this, we can set the height of the bucket proportional
  // to its frequency density (fd). The fd is the count of the bucket divided by the width of the bucket.

  // Frequency density histograms are necessary when the bucekts are of unequal width. If the buckets are collected in
  // intervals of equal width, then there is no difference between frequency and frequency density histograms.
  const fds = buckets.map((b) => {
    return parseFloat(b[3]) / (parseFloat(b[2]) - parseFloat(b[1]));
  });
  const fdMax = fds.reduce((a, b) => Math.max(a, b));
  const { zeroBucket, zeroBucketIdx } = findZeroBucket(buckets);
  console.log('ZERO BUCKET IS', zeroBucket);

  const maxPositive = parseFloat(buckets[buckets.length - 1][2]) > 0 ? parseFloat(buckets[buckets.length - 1][2]) : 0;
  const minPositive = findMinPositive();
  const maxNegative = findMaxNegative();
  console.log('MAX NEGATIVE', maxNegative, 'MIN POSITIVE', minPositive, 'MAX POSITIVE', maxPositive);
  const minNegative = parseFloat(buckets[0][1]) < 0 ? parseFloat(buckets[0][1]) : 0;
  const startNegative = minNegative !== 0 ? -Math.log(Math.abs(minNegative)) : 0; //start_neg
  const endNegative = maxNegative !== 0 ? -Math.log(Math.abs(maxNegative)) : 0; //end_neg
  const startPositive = minPositive !== 0 ? Math.log(minPositive) : 0; //start_pos
  const endPositive = maxPositive !== 0 ? Math.log(maxPositive) : 0; //end_pos

  const widthNegative = endNegative - startNegative; //width_neg
  const widthPositive = endPositive - startPositive; //width_pos
  const widthTotal = widthNegative + expBucketWidth + widthPositive; //width_total

  const zeroAxisLeft = findZeroAxisLeft();
  const zeroAxis = showZeroAxis();

  function findMinPositive() {
    if (buckets) {
      for (let i = 0; i < buckets.length; i++) {
        if (parseFloat(buckets[i][1]) > 0) {
          return parseFloat(buckets[i][1]);
        }
      }
      return 0; // all buckets are negative
    }
    return 0; // no buckets
  }

  function findMaxNegative() {
    if (buckets) {
      for (let i = 0; i < buckets.length; i++) {
        if (parseFloat(buckets[i][2]) > 0) {
          if (i === 0) {
            return 0; // all buckets are positive
          }
          return parseFloat(buckets[i - 1][2]); // return the last negative bucket
        }
      }
      return parseFloat(buckets[buckets.length - 1][2]); // all buckets are negative
    }
    return 0; // no buckets
  }

  function findZeroAxisLeft() {
    if (scale === 'linear') {
      return ((0 - rangeMin) / (rangeMax - rangeMin)) * 100 + '%';
    } else {
      if (maxNegative === 0) {
        return '0%';
      }
      if (minPositive === 0) {
        return '100%';
      }
      if (zeroBucketIdx === -1) {
        // if there is no zero bucket, we must zero axis between buckets around zero
        return (widthNegative / widthTotal) * 100 + '%';
      }
      if ((widthNegative + 0.5 * expBucketWidth) / widthTotal > 0) {
        return ((widthNegative + 0.5 * expBucketWidth) / widthTotal) * 100 + '%';
      } else {
        return '0%';
      }
    }
  }
  function showZeroAxis() {
    const axisNumber = parseFloat(zeroAxisLeft.slice(0, -1));
    if (5 < axisNumber && axisNumber < 95) {
      return true;
    }
    return false;
  }

  function findZeroBucket(buckets: [number, string, string, string][]): {
    zeroBucket: [number, string, string, string];
    zeroBucketIdx: number;
  } {
    for (let i = 0; i < buckets.length; i++) {
      const left = parseFloat(buckets[i][1]);
      const right = parseFloat(buckets[i][2]);
      if (left <= 0 && right >= 0) {
        return { zeroBucket: buckets[i], zeroBucketIdx: i };
      }
    }
    return { zeroBucket: [-1, '', '', ''], zeroBucketIdx: -1 };
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
              minPositive={minPositive}
              maxNegative={maxNegative}
              startPositive={startPositive}
              startNegative={startNegative}
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
              {rangeMin < 0 && zeroAxis && <div style={{ position: 'absolute', left: zeroAxisLeft }}>0</div>}
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
  minPositive: number;
  maxNegative: number;
  startPositive: number;
  startNegative: number;
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
  minPositive,
  maxNegative,
  startPositive,
  startNegative,
  widthNegative,
  widthTotal,
}) => {
  return (
    <React.Fragment>
      {buckets.map((b, bIdx) => {
        const left = parseFloat(b[1]);
        const right = parseFloat(b[2]);
        const count = parseFloat(b[3]);
        const bucketIdx = `bucket-${index}-${bIdx}-${Math.ceil(parseFloat(b[3]) * 100)}`;

        const expBucketWidth = Math.abs(Math.log(Math.abs(right)) - Math.log(Math.abs(left))); //bw

        let bucketWidth = '';
        let bucketLeft = '';
        let bucketHeight = '';

        switch (scale) {
          case 'linear':
            bucketWidth = ((right - left) / (rangeMax - rangeMin)) * 100 + '%';
            bucketLeft = ((left - rangeMin) / (rangeMax - rangeMin)) * 100 + '%';
            console.log(
              bucketIdx,
              'LINbucketleft= (',
              left,
              '-',
              rangeMin,
              ')/(',
              rangeMax,
              '-',
              rangeMin,
              ')=',
              bucketLeft
            );

            bucketHeight = (fds[bIdx] / fdMax) * 100 + '%';
            break;
          case 'exponential':
            let adjust = 0; // if buckets are all positive/negative, we need to adjust the width and positioning accordingly
            if (minPositive === 0 || maxNegative === 0) {
              adjust = bw;
            }
            bucketWidth = ((expBucketWidth === 0 ? bw : expBucketWidth) / (widthTotal - adjust)) * 100 + '%';
            if (left < 0) {
              // negative buckets boundary
              bucketLeft = (-(Math.log(Math.abs(left)) + startNegative) / (widthTotal - adjust)) * 100 + '%';
              console.log(
                bucketIdx,
                'EXPbucketleftNEG= (',
                -Math.log(Math.abs(left)),
                '+',
                startNegative,
                ')/(',
                widthTotal,
                ')=',
                bucketLeft
              );
            } else {
              // positive buckets boundary
              bucketLeft =
                ((Math.log(left) - startPositive + bw + widthNegative - adjust) / (widthTotal - adjust)) * 100 + '%';
              console.log(
                bucketIdx,
                'EXPbucketleftPOS= (',
                Math.log(left),
                '-',
                startPositive,
                '+',
                bw,
                '+',
                widthNegative,
                ')/(',
                widthTotal,
                ')=',
                bucketLeft
              );
            }
            if (left < 0 && right > 0) {
              bucketLeft = (widthNegative / widthTotal) * 100 + '%';
              console.log(bucketIdx, 'EXPbucketleftZERO= (', widthNegative, ')/(', widthTotal, ')=', bucketLeft);
            }

            bucketHeight = (count / countMax) * 100 + '%';
            break;
          default:
            console.error('Invalid scale type');
        }

        console.log(
          'ID',
          bucketIdx,
          '\n',
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
          startPositive,
          'rangeMax',
          rangeMax,
          'rangeMin',
          rangeMin
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
