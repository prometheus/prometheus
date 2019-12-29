import React, { FC, useState } from 'react';
import { Button } from 'reactstrap';

interface ToggleProps {
  onClick(): void;
}

export const ExpandableButton: FC<ToggleProps> = ({ children, onClick }) => {
  const [expanded, setExpanded] = useState(false);
  const event = () => {
    onClick();
    setExpanded(!expanded);
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
        show {expanded ? 'less' : 'more'}
      </Button>
    </h3>
  );
};
