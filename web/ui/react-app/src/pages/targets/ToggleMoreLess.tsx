import React, { FC, useState } from 'react';
import { Button } from 'reactstrap';

interface ToggleProps {
  onClick(): void;
}

export const ToggleMoreLess: FC<ToggleProps> = ({ children, onClick }) => {
  const [showMoreLess, toggleMoreLess] = useState(false);
  const event = () => {
    onClick();
    toggleMoreLess(!showMoreLess);
  };
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
        show {showMoreLess ? 'less' : 'more'}
      </Button>
    </h3>
  );
};
