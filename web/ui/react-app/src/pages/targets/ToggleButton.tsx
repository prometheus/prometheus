import React, { FC } from 'react';
import { Button } from 'reactstrap';

interface ToggleProps {
  child: string;
  onClick(): void;
}

const ToggleButtonProps = {
  children: '',
  color: 'primary',
  onClick: (): void => {},
  size: 'xs',
  style: {
    padding: '0.3em 0.3em 0.25em 0.3em',
    fontSize: '0.375em',
    marginLeft: '1em',
    verticalAlign: 'baseline',
  },
};

export const ToggleButton: FC<ToggleProps> = ({ child, onClick }) => {
  ToggleButtonProps.onClick = onClick;
  ToggleButtonProps.children = child;
  return <Button {...ToggleButtonProps} />;
};
