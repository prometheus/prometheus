import React, { FC, useEffect, useState } from 'react';
import { getColor, Target } from './target';
import InfiniteScroll from 'react-infinite-scroll-component';
import { Badge, Table } from 'reactstrap';
import TargetLabels from './TargetLabels';
import styles from './ScrapePoolPanel.module.css';
import { formatRelative } from '../../utils';
import { now } from 'moment';
import TargetScrapeDuration from './TargetScrapeDuration';
import EndpointLink from './EndpointLink';

const columns = ['Endpoint', 'State', 'Labels', 'Last Scrape', 'Scrape Duration', 'Error'];
const initialNumberOfTargetsDisplayed = 50;

interface ScrapePoolContentProps {
  targets: Target[];
}

export const ScrapePoolContent: FC<ScrapePoolContentProps> = ({ targets }) => {
  const [items, setItems] = useState<Target[]>(targets.slice(0, 50));
  const [index, setIndex] = useState<number>(initialNumberOfTargetsDisplayed);
  const [hasMore, setHasMore] = useState<boolean>(targets.length > initialNumberOfTargetsDisplayed);

  useEffect(() => {
    setItems(targets.slice(0, initialNumberOfTargetsDisplayed));
    setHasMore(targets.length > initialNumberOfTargetsDisplayed);
  }, [targets]);

  const fetchMoreData = () => {
    if (items.length === targets.length) {
      setHasMore(false);
    } else {
      const newIndex = index + initialNumberOfTargetsDisplayed;
      setIndex(newIndex);
      setItems(targets.slice(0, newIndex));
    }
  };

  return (
    <InfiniteScroll
      next={fetchMoreData}
      hasMore={hasMore}
      loader={<h4>loading...</h4>}
      dataLength={items.length}
      height={items.length > 25 ? '75vh' : ''}
    >
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
    </InfiniteScroll>
  );
};
