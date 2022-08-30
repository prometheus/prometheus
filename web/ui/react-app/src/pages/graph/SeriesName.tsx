import React, { FC } from 'react';
import toast from 'react-hot-toast';
import { metricToSeriesName } from '../../utils';

interface SeriesNameProps {
  labels: { [key: string]: string } | null;
  format: boolean;
}

const SeriesName: FC<SeriesNameProps> = ({ labels, format }) => {
  const toClipboard = (e: React.MouseEvent<HTMLSpanElement>) => {
    let copyText = e.currentTarget.innerText || '';
    if (copyText[0] === ',') {
      copyText = copyText.slice(1);
    }
    navigator.clipboard
      .writeText(copyText)
      .then(() => {
        toast.success('label selector   copied to clipboard', {
          id: 'series-clipboard-toast',
        });
      })
      .catch((reason) => {
        console.error(`unable to copy text: ${reason}`);
      });
  };

  const renderFormatted = (): React.ReactElement => {
    const labelNodes: React.ReactElement[] = [];
    let first = true;
    for (const label in labels) {
      if (label === '__name__') {
        continue;
      }

      labelNodes.push(
        <span className="legend-label-container" onClick={toClipboard} key={label}>
          {!first && ', '}
          <span className="legend-label-name">{label}</span>=<span className="legend-label-value">"{labels[label]}"</span>
        </span>
      );

      if (first) {
        first = false;
      }
    }

    return (
      <div>
        <span className="legend-metric-name">{labels ? labels.__name__ : ''}</span>
        <span className="legend-label-brace">{'{'}</span>
        {labelNodes}
        <span className="legend-label-brace">{'}'}</span>
      </div>
    );
  };

  if (labels === null) {
    return <>scalar</>;
  }

  if (format) {
    return renderFormatted();
  }
  // Return a simple text node. This is much faster to scroll through
  // for longer lists (hundreds of items).
  return <>{metricToSeriesName(labels)}</>;
};

export default SeriesName;
