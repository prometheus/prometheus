import React, { FC, Fragment, useState } from 'react';
import { Tooltip } from 'reactstrap';
import 'css.escape';
import { humanizeDuration } from '../../utils';

interface Labels {
  [key: string]: string;
}

export interface TargetScrapeDurationProps {
  duration: number;
  labels: Labels;
  idx: number;
  scrapePool: string;
}

const TargetScrapeDuration: FC<TargetScrapeDurationProps> = ({ duration, labels, idx, scrapePool }) => {
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
        {labels['__scrape_interval__'] ? (
          <Fragment>
            <span>Interval: {labels['__scrape_interval__']}</span>
            <br />
          </Fragment>
        ) : null}
        {labels['__scrape_timeout__'] ? (
          <Fragment>
            <span>Timeout: {labels['__scrape_timeout__']}</span>
          </Fragment>
        ) : null}
      </Tooltip>
    </>
  );
};

export default TargetScrapeDuration;
