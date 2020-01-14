import React, { FC } from 'react';
import { Button } from 'reactstrap';

interface ToggleMoreLessProps {
  event(): void;
  showMore: boolean;
}

export const ToggleMoreLess: FC<ToggleMoreLessProps> = ({ children, event, showMore }) => {
  return (
    <h3>
      {children}
      <Button
        size="xs"
        onClick={event}
        style={{
          padding: '0.3em 0.3em 0.25em 0.3em',
          fontSize: '0.375em',
          marginLeft: '1em',
          verticalAlign: 'baseline',
        }}
        color="primary"
      >
        show {showMore ? 'less' : 'more'}
      </Button>
    </h3>
  );
};
