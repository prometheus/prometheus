import React, { FC, useState, HTMLProps, memo } from 'react';
import { FontAwesomeIcon, FontAwesomeIconProps } from '@fortawesome/react-fontawesome';

interface CheckboxProps extends Omit<HTMLProps<HTMLSpanElement>, 'size' | 'onClick'> {
  onToggle: (selected: boolean) => void
}

const Checkbox: FC<CheckboxProps & Pick<FontAwesomeIconProps, 'size'>> = props => {
  const { children, onToggle, size = 'sm', checked = false, ...rest } = props;
  const [selected, setSelected] = useState(checked);
  const handleClick = () => {
    setSelected(!selected);
    onToggle && onToggle(!selected);
  }
  return (
    <span {...rest} onClick={handleClick} className={`checkbox${(selected ? ' checked' : '')}`}>
      <FontAwesomeIcon size={size} icon={['far', (selected ? 'check-square' : 'square')]}/>
      <span className="checkbox-label">{children}</span>
    </span>
  )
};

export default memo(Checkbox);