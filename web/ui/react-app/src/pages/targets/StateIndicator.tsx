import * as React from 'react';
import { Alert } from 'reactstrap';
import styles from './StateIndicator.module.css';

export interface StateIndicatorProps {
  health: string;
  message?: string;
}

const getColor = (health: string): string => {
  switch (health.toLowerCase()) {
    case 'up':
      return 'success';
    case 'down':
      return 'danger';
    default:
      return 'warning';
  }
};

const StateIndicator: React.FC<StateIndicatorProps> = ({ health, message }) => {
  const alertProps = {
    className: styles.message,
    color: getColor(health),
  };

  return <Alert {...alertProps}>{message ? message : health.toUpperCase()}</Alert>;
};

export default StateIndicator;
