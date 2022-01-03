import React, { useEffect, useState } from 'react';
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
const initialNumberOfTargetDisplayed = 50;

interface ScrapePoolContentProps {
  targets: Target[];
}

export function ScrapePoolContent(props: ScrapePoolContentProps): JSX.Element {
  const [items, setItems] = useState<Target[]>(props.targets.slice(0, 50));
  const [index, setIndex] = useState<number>(initialNumberOfTargetDisplayed);
  const [hasMore, setHasMore] = useState<boolean>(props.targets.length > initialNumberOfTargetDisplayed);
  useEffect(() => {
    setItems(props.targets.slice(0, initialNumberOfTargetDisplayed));
    setHasMore(props.targets.length > initialNumberOfTargetDisplayed);
  }, [props.targets]);

  const fetchMoreData = () => {
    if (items.length === props.targets.length) {
      setHasMore(false);
    } else {
      const newIndex = index + initialNumberOfTargetDisplayed;
      setIndex(newIndex);
      setItems(props.targets.slice(0, newIndex));
    }
  };

  return (
    <InfiniteScroll
      next={fetchMoreData}
      hasMore={hasMore}
      loader={<h4>loading ....</h4>}
      dataLength={items.length}
      height={items.length > 25 ? '75vh' : ''}
    >
      <Table className={styles.table} size="sm" hover>
        <thead>
          <tr key="header">
            {columns.map((column) => (
              <th key={column}>{column}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.map((target, index) => {
            return (
              <tr key={index}>
                <td className={styles.endpoint}>
                  <EndpointLink endpoint={target.scrapeUrl} globalUrl={target.globalUrl} />
                </td>
                <td>
                  <Badge color={getColor(target.health)}>{target.health.toUpperCase()}</Badge>
                </td>
                <td>
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
            );
          })}
        </tbody>
      </Table>
    </InfiniteScroll>
  );
}
