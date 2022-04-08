import React, { FC } from 'react';
import { getColor, Target } from './target';
import { Badge, Table } from 'reactstrap';
import TargetLabels from './TargetLabels';
import styles from './ScrapePoolPanel.module.css';
import { formatRelative } from '../../utils';
import { now } from 'moment';
import TargetScrapeDuration from './TargetScrapeDuration';
import EndpointLink from './EndpointLink';
import CustomInfiniteScroll, { InfiniteScrollItemsProps } from '../../components/CustomInfiniteScroll';

const columns = ['Endpoint', 'State', 'Labels', 'Last Scrape', 'Scrape Duration', 'Error'];

interface ScrapePoolContentProps {
  targets: Target[];
}

const ScrapePoolContentTable: FC<InfiniteScrollItemsProps<Target>> = ({ items }) => {
  return (
    <Table className={styles.table} size="sm" bordered hover striped>
      <thead>
        <tr key="header">
          {columns.map((column) => (
            <th key={column}>{column}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {items.map((target, index) => (
          <tr key={index}>
            <td className={styles.endpoint}>
              <EndpointLink endpoint={target.scrapeUrl} globalUrl={target.globalUrl} />
            </td>
            <td className={styles.state}>
              <Badge color={getColor(target.health)}>{target.health.toUpperCase()}</Badge>
            </td>
            <td className={styles.labels}>
              <TargetLabels
                discoveredLabels={target.discoveredLabels}
                labels={target.labels}
                scrapePool={target.scrapePool}
                idx={index}
              />
            </td>
            <td className={styles['last-scrape']}>{formatRelative(target.lastScrape, now())}</td>
            <td className={styles['scrape-duration']}>
              <TargetScrapeDuration
                duration={target.lastScrapeDuration}
                scrapePool={target.scrapePool}
                idx={index}
                interval={target.scrapeInterval}
                timeout={target.scrapeTimeout}
              />
            </td>
            <td className={styles.errors}>
              {target.lastError ? <span className="text-danger">{target.lastError}</span> : null}
            </td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
};

export const ScrapePoolContent: FC<ScrapePoolContentProps> = ({ targets }) => {
  return <CustomInfiniteScroll allItems={targets} child={ScrapePoolContentTable} />;
};
