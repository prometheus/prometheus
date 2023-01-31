import React, { FC } from 'react';
import { UncontrolledTooltip } from 'reactstrap';
import { Histogram } from '../../types/types';
import { bucketRangeString } from './DataTable';

const HistogramChart: FC<{ histogram: Histogram; index: number }> = ({ index, histogram }) => {
  const { buckets } = histogram;
  const rangeMax = buckets ? parseFloat(buckets[buckets.length - 1][2]) : 0;
  const countMax = buckets ? buckets.map((b) => parseFloat(b[3])).reduce((a, b) => Math.max(a, b)) : 0;
  const formatter = Intl.NumberFormat('en', { notation: 'compact' });
  return (
    <div className="histogram-y-wrapper">
      <div className="histogram-y-labels">
        {[1, 0.75, 0.5, 0.25].map((i) => (
          <div key={i} className="histogram-y-label">
            {formatter.format(countMax * i)}
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
              <div className="histogram-x-tick" style={{ left: i * 100 + '%' }}></div>
            </React.Fragment>
          ))}
          {buckets?.map((b, bIdx) => (
            <React.Fragment key={bIdx}>
              <div
                id={`bucket-${index}-${bIdx}`}
                className="histogram-bucket-slot"
                style={{
                  left: (parseFloat(b[1]) / rangeMax) * 100 + '%',
                  width: ((parseFloat(b[2]) - parseFloat(b[1])) / rangeMax) * 100 + '%',
                }}
              >
                <div
                  id={`bucket-${index}-${bIdx}`}
                  className="histogram-bucket"
                  style={{
                    height: (parseFloat(b[3]) / countMax) * 100 + '%',
                  }}
                ></div>
                <UncontrolledTooltip
                  style={{ maxWidth: 'unset', padding: 10, textAlign: 'left' }}
                  placement="bottom"
                  target={`bucket-${index}-${bIdx}`}
                >
                  <strong>range:</strong> {bucketRangeString(b)}
                  <br />
                  <strong>count:</strong> {b[3]}
                </UncontrolledTooltip>
              </div>
            </React.Fragment>
          ))}
          <div className="histogram-axes"></div>
        </div>
        <div className="histogram-x-labels">
          <div key={0} className="histogram-x-label" style={{ width: 0 }}>
            0
          </div>
          {[0.25, 0.5, 0.75, 1].map((i) => (
            <div key={i} className="histogram-x-label">
              <div style={{ position: 'absolute', right: i === 1 ? 0 : -18 }}>{formatter.format(rangeMax * i)}</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

export default HistogramChart;
