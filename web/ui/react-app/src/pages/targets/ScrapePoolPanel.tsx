import React, { FC } from 'react';
import { ScrapePool, getColor } from './target';
import { Collapse, Table, Badge } from 'reactstrap';
import styles from './ScrapePoolPanel.module.css';
import { Target } from './target';
import EndpointLink from './EndpointLink';
import TargetLabels from './TargetLabels';
import TargetScrapeDuration from './TargetScrapeDuration';
import { now } from 'moment';
import { ToggleMoreLess } from '../../components/ToggleMoreLess';
import { formatRelative } from '../../utils';

interface PanelProps {
  scrapePool: string;
  targetGroup: ScrapePool;
  expanded: boolean;
  toggleExpanded: () => void;
}

export const columns = ['Endpoint', 'State', 'Labels', 'Last Scrape', 'Scrape Duration', 'Error'];

const ScrapePoolPanel: FC<PanelProps> = ({ scrapePool, targetGroup, expanded, toggleExpanded }) => {
  const modifier = targetGroup.upCount < targetGroup.targets.length ? 'danger' : 'normal';
  const id = `pool-${scrapePool}`;
  const anchorProps = {
    href: `#${id}`,
    id,
  };

  return (
    <div className={styles.container}>
      <ToggleMoreLess event={toggleExpanded} showMore={expanded}>
        <a className={styles[modifier]} {...anchorProps}>
          {`${scrapePool} (${targetGroup.upCount}/${targetGroup.targets.length} up)`}
        </a>
      </ToggleMoreLess>
      <Collapse isOpen={expanded}>
        <Table className={styles.table} size="sm" bordered hover striped>
          <thead>
            <tr key="header">
              {columns.map((column) => (
                <th key={column}>{column}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {targetGroup.targets.map((target: Target, idx: number) => {
              const {
                discoveredLabels,
                labels,
                scrapePool,
                scrapeUrl,
                globalUrl,
                lastError,
                lastScrape,
                lastScrapeDuration,
                health,
                scrapeInterval,
                scrapeTimeout,
              } = target;
              const color = getColor(health);

              return (
                <tr key={scrapeUrl}>
                  <td className={styles.endpoint}>
                    <EndpointLink endpoint={scrapeUrl} globalUrl={globalUrl} />
                  </td>
                  <td className={styles.state}>
                    <Badge color={color}>{health.toUpperCase()}</Badge>
                  </td>
                  <td className={styles.labels}>
                    <TargetLabels discoveredLabels={discoveredLabels} labels={labels} scrapePool={scrapePool} idx={idx} />
                  </td>
                  <td className={styles['last-scrape']}>{formatRelative(lastScrape, now())}</td>
                  <td className={styles['scrape-duration']}>
                    <TargetScrapeDuration
                      duration={lastScrapeDuration}
                      scrapePool={scrapePool}
                      idx={idx}
                      interval={scrapeInterval}
                      timeout={scrapeTimeout}
                    />
                  </td>
                  <td className={styles.errors}>{lastError ? <span className="text-danger">{lastError}</span> : null}</td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      </Collapse>
    </div>
  );
};

export default ScrapePoolPanel;
