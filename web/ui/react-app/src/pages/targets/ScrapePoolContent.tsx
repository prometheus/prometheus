import React, { FC, useState } from 'react';
import { getColor, Target } from './target';
import { Badge, Table } from 'reactstrap';
import TargetLabels from './TargetLabels';
import styles from './ScrapePoolPanel.module.css';
import { formatRelative } from '../../utils';
import { now } from 'moment';
import TargetScrapeDuration from './TargetScrapeDuration';
import EndpointLink from './EndpointLink';
import CustomInfiniteScroll, { InfiniteScrollItemsProps } from '../../components/CustomInfiniteScroll';
import { faSort, faSortDown, faSortUp } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

interface ScrapePoolContentProps {
  targets: Target[];
}

const ScrapePoolContentTable: FC<InfiniteScrollItemsProps<Target>> = ({ items }) => {
  const [sortState, setSortState] = useState<SortState>({ column: 'scrapeUrl', desc: true });
  return (
    <Table className={styles.table} size="sm" bordered hover striped>
      <thead>
        <tr key="header">
          <SortableColumnHeader setSortState={setSortState} column="scrapeUrl" label="Endpoint" sortState={sortState} />
          <SortableColumnHeader setSortState={setSortState} column="health" label="State" sortState={sortState} />
          {/* NOTE: we do not sort labels as an ordering here isn't completely obvious or useful. */}
          <th>Labels</th>
          <SortableColumnHeader setSortState={setSortState} column="lastScrape" label="Last Scrape" sortState={sortState} />
          <SortableColumnHeader
            setSortState={setSortState}
            column="lastScrapeDuration"
            label="Scrape Duration"
            sortState={sortState}
          />
          <SortableColumnHeader setSortState={setSortState} column="lastError" label="Error" sortState={sortState} />
        </tr>
      </thead>
      <tbody>
        {items.sort(compareFunc(sortState)).map((target, index) => (
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

const stringColumns = ['scrapeUrl', 'health', 'lastError', 'lastScrape'] as const;
type StringColumn = typeof stringColumns[number];
const numericColumns = ['lastScrapeDuration'] as const;
type NumericColumn = typeof numericColumns[number];
type SortableColumn = StringColumn | NumericColumn;

const compareFunc = (sortState: SortState) => (a: Target, b: Target) => {
  // We need to distinguish between string columns and number columns to handle
  // sorting appropriately.
  if (stringColumns.includes(sortState.column as StringColumn)) {
    return (
      (sortState.desc ? 1 : -1) * a[sortState.column as StringColumn].localeCompare(b[sortState.column as StringColumn])
    );
  } else if (numericColumns.includes(sortState.column as NumericColumn)) {
    return (sortState.desc ? 1 : -1) * a[sortState.column as NumericColumn] - b[sortState.column as NumericColumn];
  }

  return 0;
};

interface SortState {
  column: SortableColumn;
  desc: boolean;
}

interface SortableColumnHeaderProps {
  setSortState: (state: SortState) => void;
  column: SortableColumn;
  label: string;
  sortState: SortState;
}

const SortableColumnHeader: FC<SortableColumnHeaderProps> = ({ setSortState, column, label, sortState }) => (
  <th onClick={() => setSortState({ column, desc: !sortState.desc })} role="button">
    <div className="d-flex justify-content-between">
      {label}
      <FontAwesomeIcon
        className={column != sortState.column ? 'text-muted' : 'text-dark'}
        icon={column != sortState.column ? faSort : sortState.desc ? faSortDown : faSortUp}
      />
    </div>
  </th>
);

export const ScrapePoolContent: FC<ScrapePoolContentProps> = ({ targets }) => {
  return <CustomInfiniteScroll allItems={targets} child={ScrapePoolContentTable} />;
};
