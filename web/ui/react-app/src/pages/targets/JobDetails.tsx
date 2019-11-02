import React, { FC } from 'react';
import { TargetGroup } from './target';
import { Table } from 'reactstrap';
import StateIndicator from './StateIndicator';
import EndpointLink from './EndpointLink';
import { humanizeDuration, formatRelative, now } from '../../utils/timeFormat';
import SeriesLabels from './SeriesLabels';
import styles from './JobDetails.module.css';

export const columns = ['Endpoint', 'State', 'Labels', 'Last Scrape', 'Scrape Duration', 'Error'];

interface JobDetailsProps {
  targetGroup: TargetGroup;
}

const JobDetails: FC<JobDetailsProps> = ({ targetGroup }) => {
  return (
    <Table className={styles.table} size="sm" bordered hover striped>
      <thead>
        <tr key="header">
          {columns.map(column => (
            <th key={column}>{column}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {targetGroup.targets.map(target => {
          const { discoveredLabels, labels, scrapeUrl, lastError, lastScrape, lastScrapeDuration, health } = target;

          return (
            <tr key={scrapeUrl}>
              <td className={styles.endpoint}>
                <EndpointLink endpoint={scrapeUrl} />
              </td>
              <td className={styles.state}>
                <StateIndicator health={health} />
              </td>
              <td className={styles.labels}>
                <SeriesLabels discoveredLabels={discoveredLabels} labels={labels} />
              </td>
              <td className={styles['last-scrape']}>{formatRelative(lastScrape, now())}</td>
              <td className={styles['scrape-duration']}>{humanizeDuration(lastScrapeDuration / 1000000)}</td>
              <td className={styles.errors}>{lastError ? <StateIndicator health={health} message={lastError} /> : null}</td>
            </tr>
          );
        })}
      </tbody>
    </Table>
  );
};

export default JobDetails;
