import * as React from 'react';
import { shallow } from 'enzyme';
import Checkbox from './Checkbox';
import { FormGroup, Label, Input } from 'reactstrap';

const MockCmp: React.FC = () => <div className="mock" />;

describe('Checkbox', () => {
  it('renders with subcomponents', () => {
    const checkBox = shallow(<Checkbox />);
    [FormGroup, Input, Label].forEach((component) => expect(checkBox.find(component)).toHaveLength(1));
  });

  it('passes down the correct FormGroup props', () => {
    const checkBoxProps = { wrapperStyles: { color: 'orange' } };
    const checkBox = shallow(<Checkbox {...checkBoxProps} />);
    const formGroup = checkBox.find(FormGroup);
    expect(Object.keys(formGroup.props())).toHaveLength(4);
    expect(formGroup.prop('className')).toEqual('custom-control custom-checkbox');
    expect(formGroup.prop('children')).toHaveLength(2);
    expect(formGroup.prop('style')).toEqual({ color: 'orange' });
    expect(formGroup.prop('tag')).toEqual('div');
  });

  it('passes down the correct FormGroup Input props', () => {
    const results: string[] = [];
    const checkBoxProps = {
      onChange: (): void => {
        results.push('clicked');
      },
    };
    const checkBox = shallow(<Checkbox {...checkBoxProps} id="1" />);
    const input = checkBox.find(Input);
    expect(Object.keys(input.props())).toHaveLength(4);
    expect(input.prop('className')).toEqual('custom-control-input');
    expect(input.prop('id')).toMatch('1');
    expect(input.prop('type')).toEqual('checkbox');
    input.simulate('change');
    expect(results).toHaveLength(1);
    expect(results[0]).toEqual('clicked');
  });

  it('passes down the correct Label props', () => {
    const checkBox = shallow(
      <Checkbox id="1">
        <MockCmp />
      </Checkbox>
    );
    const label = checkBox.find(Label);
    expect(Object.keys(label.props())).toHaveLength(6);
    expect(label.prop('className')).toEqual('custom-control-label');
    expect(label.find(MockCmp)).toHaveLength(1);
    expect(label.prop('for')).toMatch('1');
    expect(label.prop('style')).toEqual({ userSelect: 'none' });
    expect(label.prop('tag')).toEqual('label');
  });

  it('shares checkbox `id` uuid with Input/Label subcomponents', () => {
    const checkBox = shallow(<Checkbox id="2" />);
    const input = checkBox.find(Input);
    const label = checkBox.find(Label);
    expect(label.prop('for')).toBeDefined();
    expect(label.prop('for')).toEqual(input.prop('id'));
  });
});
