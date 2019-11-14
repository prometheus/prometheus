import React, { Dispatch, FC, SetStateAction } from 'react';
import { Button, ButtonGroup } from 'reactstrap';
import styles from './Filter.module.css';

export interface FilterData {
  showHealthy: boolean;
  showUnhealthy: boolean;
}

export interface FilterProps {
  filter: FilterData;
  setFilter: Dispatch<SetStateAction<FilterData>>;
}

const Filter: FC<FilterProps> = ({ filter, setFilter }) => {
  const { showHealthy } = filter;
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
  };
  return (
    <ButtonGroup>
      <Button {...btnProps.all}>All</Button>
      <Button {...btnProps.unhealthy}>Unhealthy</Button>
    </ButtonGroup>
  );
};

export default Filter;
