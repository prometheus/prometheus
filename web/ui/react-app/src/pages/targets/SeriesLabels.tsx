import * as React from 'react';
import { Tooltip } from 'reactstrap';
import SeriesLabel from './SeriesLabel';

interface Labels {
  [key: string]: string;
}

export interface SeriesLabelsProps {
  discoveredLabels: Labels;
  labels: Labels;
}

const formatLabels = (labels: Labels): string[] => Object.keys(labels).map(key => `${key}="${labels[key]}"`);

const SeriesLabels: React.FC<SeriesLabelsProps> = ({ discoveredLabels, labels }: SeriesLabelsProps) => {
  const [tooltipOpen, setTooltipOpen] = React.useState(false);

  const toggle = (): void => setTooltipOpen(!tooltipOpen);

  return (
    <>
      <div id="series-labels" className="series-labels-container">
        {Object.keys(labels).map(key => {
          return <SeriesLabel key={key} labelKey={key} labelValue={labels[key]} />;
        })}
      </div>
      <Tooltip isOpen={tooltipOpen} target="series-labels" toggle={toggle}>
        <b>Before relabeling:</b>
        {formatLabels(discoveredLabels).map((s: string, idx: number) => (
          <React.Fragment key={idx}>
            <br />
            <span>{s}</span>
          </React.Fragment>
        ))}
      </Tooltip>
    </>
  );
};

export default SeriesLabels;
