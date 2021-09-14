import React, { FC, Fragment, useState } from 'react';
import { Tooltip } from 'reactstrap';
import 'css.escape';
import { humanizeDuration } from '../../utils';

export interface TargetScrapeDurationProps {
  duration: number;
  interval: string;
  timeout: string;
  idx: number;
  scrapePool: string;
}

const TargetScrapeDuration: FC<TargetScrapeDurationProps> = ({ duration, interval, timeout, idx, scrapePool }) => {
  const [scrapeTooltipOpen, setScrapeTooltipOpen] = useState<boolean>(false);
  const id = `scrape-duration-${scrapePool}-${idx}`;

  return (
    <>
      <div id={id} className="scrape-duration-container">
        {humanizeDuration(duration * 1000)}
      </div>
      <Tooltip
        isOpen={scrapeTooltipOpen}
        toggle={() => setScrapeTooltipOpen(!scrapeTooltipOpen)}
        target={CSS.escape(id)}
        style={{ maxWidth: 'none', textAlign: 'left' }}
      >
        <Fragment>
          <span>Interval: {interval}</span>
          <br />
        </Fragment>
        <Fragment>
          <span>Timeout: {timeout}</span>
        </Fragment>
      </Tooltip>
    </>
  );
};

export default TargetScrapeDuration;
