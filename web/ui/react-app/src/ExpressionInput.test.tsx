import * as React from 'react';
import { mount, ReactWrapper } from 'enzyme';
import ExpressionInput from './ExpressionInput';
import Downshift from 'downshift';
import { Button, InputGroup, InputGroupAddon, Input } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faSpinner } from '@fortawesome/free-solid-svg-icons';
import SanitizeHTML from './components/SanitizeHTML';

const getKeyEvent = (key: string): React.KeyboardEvent<HTMLInputElement> =>
  ({
    key,
    nativeEvent: {},
    preventDefault: () => {},
  } as React.KeyboardEvent<HTMLInputElement>);

describe('ExpressionInput', () => {
  const metricNames = ['instance:node_cpu_utilisation:rate1m', 'node_cpu_guest_seconds_total', 'node_cpu_seconds_total'];
  const expressionInputProps = {
    value: 'node_cpu',
    autocompleteSections: {
      'Query History': [],
      'Metric Names': metricNames,
    },
    executeQuery: (): void => {},
    loading: false,
  };

  let expressionInput: ReactWrapper;
  beforeEach(() => {
    expressionInput = mount(<ExpressionInput {...expressionInputProps} />);
  });

  it('renders a downshift component', () => {
    const downshift = expressionInput.find(Downshift);
    expect(downshift).toHaveLength(1);
  });

  it('renders an InputGroup', () => {
    const inputGroup = expressionInput.find(InputGroup);
    expect(inputGroup.prop('className')).toEqual('expression-input');
  });

  it('renders a search icon when it is not loading', () => {
    const addon = expressionInput.find(InputGroupAddon).filterWhere(addon => addon.prop('addonType') === 'prepend');
    const icon = addon.find(FontAwesomeIcon);
    expect(icon.prop('icon')).toEqual(faSearch);
  });

  it('renders a loading icon when it is loading', () => {
    const expressionInput = mount(<ExpressionInput {...expressionInputProps} loading={true} />);
    const addon = expressionInput.find(InputGroupAddon).filterWhere(addon => addon.prop('addonType') === 'prepend');
    const icon = addon.find(FontAwesomeIcon);
    expect(icon.prop('icon')).toEqual(faSpinner);
    expect(icon.prop('spin')).toBe(true);
  });

  it('renders an Input', () => {
    const input = expressionInput.find(Input);
    expect(input.prop('style')).toEqual({ height: 0 });
    expect(input.prop('autoFocus')).toEqual(true);
    expect(input.prop('type')).toEqual('textarea');
    expect(input.prop('rows')).toEqual('1');
    expect(input.prop('placeholder')).toEqual('Expression (press Shift+Enter for newlines)');
    expect(expressionInput.state('value')).toEqual('node_cpu');
  });

  describe('when autosuggest is closed', () => {
    it('prevents Downshift default on Home, End, Arrows', () => {
      const downshift = expressionInput.find(Downshift);
      const input = downshift.find(Input);
      downshift.setState({ isOpen: false });
      const onKeyDown = input.prop('onKeyDown');
      ['Home', 'End', 'ArrowUp', 'ArrowDown'].forEach(key => {
        const event = getKeyEvent(key);
        input.simulate('keydown', event);
        const nativeEvent = event.nativeEvent as any;
        expect(nativeEvent.preventDownshiftDefault).toBe(true);
      });
    });

    it('does not render an autosuggest', () => {
      const downshift = expressionInput.find(Downshift);
      downshift.setState({ isOpen: false });
      const ul = downshift.find('ul');
      expect(ul).toHaveLength(0);
    });
  });

  describe('handleInput', () => {
    it('should call setState', () => {
      const instance: any = expressionInput.instance();
      const stateSpy = jest.spyOn(instance, 'setState');
      instance.handleInput();
      expect(stateSpy).toHaveBeenCalled();
    });
  });

  describe('onSelect', () => {
    it('should call setState with selected value', () => {
      const instance: any = expressionInput.instance();
      const stateSpy = jest.spyOn(instance, 'setState');
      instance.setValue('foo');
      expect(stateSpy).toHaveBeenCalledWith({ value: 'foo', height: 'auto' }, expect.anything());
    });
  });

  describe('handleKeyPress', () => {
    it('should call executeQuery on Enter key pressed', () => {
      const spyExecuteQuery = jest.fn();
      const input = mount(<ExpressionInput executeQuery={spyExecuteQuery} {...({} as any)} />);
      const instance: any = input.instance();
      instance.handleKeyPress({ preventDefault: jest.fn, key: 'Enter' });
      expect(spyExecuteQuery).toHaveBeenCalled();
    });
    it('should NOT call executeQuery on Enter + Shift', () => {
      const spyExecuteQuery = jest.fn();
      const input = mount(<ExpressionInput executeQuery={spyExecuteQuery} {...({} as any)} />);
      const instance: any = input.instance();
      instance.handleKeyPress({ preventDefault: jest.fn, key: 'Enter', shiftKey: true });
      expect(spyExecuteQuery).not.toHaveBeenCalled();
    });
  });

  describe('getSearchMatches', () => {
    it('should return matched value', () => {
      const instance: any = expressionInput.instance();
      expect(instance.getSearchMatches('foo', ['barfoobaz', 'bazasdbaz'])).toHaveLength(1);
    });
    it('should return empty array if no match found', () => {
      const instance: any = expressionInput.instance();
      expect(instance.getSearchMatches('foo', ['barbaz', 'bazasdbaz'])).toHaveLength(0);
    });
  });

  describe('createAutocompleteSection', () => {
    it('should close menu if no matches found', () => {
      const input = mount(<ExpressionInput autocompleteSections={{ title: ['foo', 'bar', 'baz'] }} {...({} as any)} />);
      const instance: any = input.instance();
      const spyCloseMenu = jest.fn();
      instance.createAutocompleteSection({ inputValue: 'qqqqqq', closeMenu: spyCloseMenu });
      setTimeout(() => {
        expect(spyCloseMenu).toHaveBeenCalled();
      });
    });
    it('should not render lsit if inputValue not exist', () => {
      const input = mount(<ExpressionInput autocompleteSections={{ title: ['foo', 'bar', 'baz'] }} {...({} as any)} />);
      const instance: any = input.instance();
      const spyCloseMenu = jest.fn();
      instance.createAutocompleteSection({ closeMenu: spyCloseMenu });
      setTimeout(() => expect(spyCloseMenu).toHaveBeenCalled());
    });
    it('should render autosuggest-dropdown', () => {
      const input = mount(<ExpressionInput autocompleteSections={{ title: ['foo', 'bar', 'baz'] }} {...({} as any)} />);
      const instance: any = input.instance();
      const spyGetMenuProps = jest.fn();
      const sections = instance.createAutocompleteSection({
        inputValue: 'foo',
        highlightedIndex: 0,
        getMenuProps: spyGetMenuProps,
        getItemProps: jest.fn,
      });
      expect(sections.props.className).toEqual('autosuggest-dropdown');
    });
  });

  describe('when downshift is open', () => {
    it('closes the menu on "Enter"', () => {
      const downshift = expressionInput.find(Downshift);
      const input = downshift.find(Input);
      downshift.setState({ isOpen: true });
      const event = getKeyEvent('Enter');
      input.simulate('keydown', event);
      expect(downshift.state('isOpen')).toBe(false);
    });

    it('should blur input on escape', () => {
      const downshift = expressionInput.find(Downshift);
      const instance: any = expressionInput.instance();
      const spyBlur = jest.spyOn(instance.exprInputRef.current, 'blur');
      const input = downshift.find(Input);
      downshift.setState({ isOpen: false });
      const event = getKeyEvent('Escape');
      input.simulate('keydown', event);
      expect(spyBlur).toHaveBeenCalled();
    });

    it('noops on ArrowUp or ArrowDown', () => {
      const downshift = expressionInput.find(Downshift);
      const input = downshift.find(Input);
      downshift.setState({ isOpen: true });
      ['ArrowUp', 'ArrowDown'].forEach(key => {
        const event = getKeyEvent(key);
        input.simulate('keydown', event);
        const nativeEvent = event.nativeEvent as any;
        expect(nativeEvent.preventDownshiftDefault).toBeUndefined();
      });
    });

    it('does not render an autosuggest if there are no matches', () => {
      const downshift = expressionInput.find(Downshift);
      downshift.setState({ isOpen: true });
      const ul = downshift.find('ul');
      expect(ul).toHaveLength(0);
    });

    it('renders an autosuggest if there are matches', () => {
      const downshift = expressionInput.find(Downshift);
      downshift.setState({ isOpen: true });
      setTimeout(() => {
        const ul = downshift.find('ul');
        expect(ul.prop('className')).toEqual('card list-group');
        const items = ul.find(SanitizeHTML);
        expect(items.map(item => item.text()).join(', ')).toEqual(
          'node_cpu_guest_seconds_total, node_cpu_seconds_total, instance:node_cpu_utilisation:rate1m'
        );
      });
    });
  });

  it('renders an execute Button', () => {
    const addon = expressionInput.find(InputGroupAddon).filterWhere(addon => addon.prop('addonType') === 'append');
    const button = addon.find(Button);
    expect(button.prop('className')).toEqual('execute-btn');
    expect(button.prop('color')).toEqual('primary');
    expect(button.text()).toEqual('Execute');
  });
});
