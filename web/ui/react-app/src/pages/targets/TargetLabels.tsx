import React, { FC, Fragment, useState } from 'react';
import { Badge, Tooltip } from 'reactstrap';
import 'css.escape';
import styles from './TargetLabels.module.css';

interface Labels {
  [key: string]: string;
}

export interface TargetLabelsProps {
  discoveredLabels: Labels;
  labels: Labels;
  idx: number;
  scrapePool: string;
}

const formatLabels = (labels: Labels): string[] => Object.keys(labels).map((key) => `${key}="${labels[key]}"`);

const TargetLabels: FC<TargetLabelsProps> = ({ discoveredLabels, labels, idx, scrapePool }) => {
  const [tooltipOpen, setTooltipOpen] = useState(false);

  const toggle = (): void => setTooltipOpen(!tooltipOpen);
  const id = `series-labels-${scrapePool}-${idx}`;

  return (
    <>
      <div id={id} className="series-labels-container">
        {Object.keys(labels).map((labelName) => {
          return (
            <Badge color="primary" className="mr-1" key={labelName}>
              {`${labelName}="${labels[labelName]}"`}
            </Badge>
          );
        })}
      </div>
      <Tooltip isOpen={tooltipOpen} target={CSS.escape(id)} toggle={toggle} style={{ maxWidth: 'none', textAlign: 'left' }}>
        <b>Before relabeling:</b>
        {formatLabels(discoveredLabels).map((s: string, idx: number) => (
          <Fragment key={idx}>
            <br />
            <span className={styles.discovered}>{s}</span>
          </Fragment>
        ))}
      </Tooltip>
    </>
  );
};

export default TargetLabels;
