import * as React from 'react';
import { mount, ReactWrapper } from 'enzyme';
import ExpressionInput from './ExpressionInput';
import { Button, InputGroup, InputGroupAddon } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faSpinner } from '@fortawesome/free-solid-svg-icons';

describe('ExpressionInput', () => {
  const expressionInputProps = {
    value: 'node_cpu',
    queryHistory: [],
    metricNames: [],
    executeQuery: (): void => {
      // Do nothing.
    },
    onExpressionChange: (): void => {
      // Do nothing.
    },
    loading: false,
    enableAutocomplete: true,
    enableHighlighting: true,
    enableLinter: true,
  };

  let expressionInput: ReactWrapper;
  beforeEach(() => {
    expressionInput = mount(<ExpressionInput {...expressionInputProps} />);
  });

  it('renders an InputGroup', () => {
    const inputGroup = expressionInput.find(InputGroup);
    expect(inputGroup.prop('className')).toEqual('expression-input');
  });

  it('renders a search icon when it is not loading', () => {
    const addon = expressionInput.find(InputGroupAddon).filterWhere((addon) => addon.prop('addonType') === 'prepend');
    const icon = addon.find(FontAwesomeIcon);
    expect(icon.prop('icon')).toEqual(faSearch);
  });

  it('renders a loading icon when it is loading', () => {
    const expressionInput = mount(<ExpressionInput {...expressionInputProps} loading={true} />);
    const addon = expressionInput.find(InputGroupAddon).filterWhere((addon) => addon.prop('addonType') === 'prepend');
    const icon = addon.find(FontAwesomeIcon);
    expect(icon.prop('icon')).toEqual(faSpinner);
    expect(icon.prop('spin')).toBe(true);
  });

  it('renders a CodeMirror expression input', () => {
    const input = expressionInput.find('div.cm-expression-input');
    expect(input.text()).toContain('node_cpu');
  });

  it('renders an execute button', () => {
    const addon = expressionInput.find(InputGroupAddon).filterWhere((addon) => addon.prop('addonType') === 'append');
    const button = addon.find(Button).find('.execute-btn').first();
    expect(button.prop('color')).toEqual('primary');
    expect(button.text()).toEqual('Execute');
  });

  it('executes the query when clicking the execute button', () => {
    const spyExecuteQuery = jest.fn();
    const props = { ...expressionInputProps, executeQuery: spyExecuteQuery };
    const wrapper = mount(<ExpressionInput {...props} />);
    const btn = wrapper.find(Button).filterWhere((btn) => btn.hasClass('execute-btn'));
    btn.simulate('click');
    expect(spyExecuteQuery).toHaveBeenCalledTimes(1);
  });
});
