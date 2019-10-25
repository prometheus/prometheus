import React, { FC, HTMLProps, memo } from 'react';
import { FormGroup, Label, Input } from 'reactstrap';
import { uuidGen } from './utils/func';

const Checkbox: FC<HTMLProps<HTMLSpanElement>> = ({ children, onChange, style }) => {
  const id = uuidGen();
  return (
    <FormGroup className="custom-control custom-checkbox" style={style}>
      <Input
        onChange={onChange}
        type="checkbox"
        className="custom-control-input"
        id={`checkbox_${id}`}
        placeholder="password placeholder" />
      <Label style={{ userSelect: 'none' }} className="custom-control-label" for={`checkbox_${id}`}>
        {children}
      </Label>
    </FormGroup>
  )
}

export default memo(Checkbox);
