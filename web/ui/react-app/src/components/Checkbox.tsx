import React, { FC, memo, CSSProperties, ReactNode } from 'react';
import { FormGroup, Label, Input, InputProps } from 'reactstrap';

interface CheckboxProps extends InputProps {
  children?: ReactNode;
  wrapperStyles?: CSSProperties;
}

const Checkbox: FC<CheckboxProps> = ({ children, wrapperStyles, id, ...rest }) => {
  return (
    <FormGroup className="custom-control custom-checkbox" style={wrapperStyles}>
      <Input {...rest} id={id} type="checkbox" className="custom-control-input" />
      <Label style={{ userSelect: 'none' }} className="custom-control-label" for={id}>
        {children}
      </Label>
    </FormGroup>
  );
};

export default memo(Checkbox);
