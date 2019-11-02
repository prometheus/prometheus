import * as React from 'react';
import { Badge } from 'reactstrap';
import styles from './SeriesLabel.module.css';

export interface SeriesLabelProps {
  labelKey: string;
  labelValue: string;
}

const SeriesLabel: React.FC<SeriesLabelProps> = ({ labelKey, labelValue }) => {
  return (
    <Badge color="primary" className={styles['series-label']}>
      {`${labelKey}="${labelValue}"`}
    </Badge>
  );
};

export default SeriesLabel;
