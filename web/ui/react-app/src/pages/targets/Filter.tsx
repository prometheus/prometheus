import React, { Dispatch, FC, SetStateAction } from 'react';
import { Button, ButtonGroup } from 'reactstrap';
import styles from './Filter.module.css';

export interface FilterData {
  showHealthy: boolean;
  showUnhealthy: boolean;
}

export interface Expanded {
  [scrapePool: string]: boolean;
}

export interface FilterProps {
  filter: FilterData;
  setFilter: Dispatch<SetStateAction<FilterData>>;
  expanded: Expanded;
  setExpanded: Dispatch<SetStateAction<Expanded>>;
}

const Filter: FC<FilterProps> = ({ filter, setFilter, expanded, setExpanded }) => {
  const { showHealthy } = filter;
  const allExpanded = Object.values(expanded).every((v: boolean): boolean => v);
  const mapExpansion = (next: boolean): Expanded =>
    Object.keys(expanded).reduce(
      (acc: { [scrapePool: string]: boolean }, scrapePool: string) => ({
        ...acc,
        [scrapePool]: next,
      }),
      {}
    );
  const btnProps = {
    all: {
      active: showHealthy,
      className: `all ${styles.btn}`,
      color: 'primary',
      onClick: (): void => setFilter({ ...filter, showHealthy: true }),
    },
    unhealthy: {
      active: !showHealthy,
      className: `unhealthy ${styles.btn}`,
      color: 'primary',
      onClick: (): void => setFilter({ ...filter, showHealthy: false }),
    },
    expansionState: {
      active: false,
      className: `expansion ${styles.btn}`,
      color: 'primary',
      onClick: (): void => setExpanded(mapExpansion(!allExpanded)),
    },
  };
  return (
    <ButtonGroup>
      <Button {...btnProps.all}>All</Button>
      <Button {...btnProps.unhealthy}>Unhealthy</Button>
      <Button {...btnProps.expansionState}>{allExpanded ? 'Collapse All' : 'Expand All'}</Button>
    </ButtonGroup>
  );
};

export default Filter;
