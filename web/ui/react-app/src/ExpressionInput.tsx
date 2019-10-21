import React, { Component } from 'react';
import {
  Button,
  InputGroup,
  InputGroupAddon,
  InputGroupText,
  Input,
} from 'reactstrap';

import Downshift, { ControllerStateAndHelpers } from 'downshift';
import fuzzy from 'fuzzy';
import SanitizeHTML from './components/SanitizeHTML';

import { library } from '@fortawesome/fontawesome-svg-core';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faSpinner } from '@fortawesome/free-solid-svg-icons';

library.add(faSearch, faSpinner);

interface ExpressionInputProps {
  value: string;
  metricNames: string[];
  executeQuery: (expr: string) => void;
  loading: boolean;
}

interface ExpressionInputState {
  height: number | string;
  value: string;
}

class ExpressionInput extends Component<ExpressionInputProps, ExpressionInputState> {
  private prevNoMatchValue: string | null = null;
  private exprInputRef = React.createRef<HTMLInputElement>();

  constructor(props: ExpressionInputProps) {
    super(props);
    this.state = {
      value: props.value,
      height: 'auto'
    }
  }

  componentDidMount() {
    this.setHeight();
  }


  setHeight = () => {
    const { offsetHeight, clientHeight, scrollHeight } = this.exprInputRef.current!;
    const offset = offsetHeight - clientHeight; // Needed in order for the height to be more accurate.
    this.setState({ height: scrollHeight + offset });
  }

  handleInput = () => {
    this.setState({
      height: 'auto',
      value: this.exprInputRef.current!.value
    }, this.setHeight);
  }

  handleDropdownSelection = (value: string) => this.setState({ value });
  
  handleKeyPress = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      this.props.executeQuery(this.exprInputRef.current!.value);
      event.preventDefault();
    }
  }

  executeQuery = () => this.props.executeQuery(this.exprInputRef.current!.value)

  renderAutosuggest = (downshift: ControllerStateAndHelpers<any>) => {
    const { inputValue } = downshift
    if (!inputValue || (this.prevNoMatchValue && inputValue.includes(this.prevNoMatchValue))) {
      // This is ugly but is needed in order to sync state updates.
      // This way we force downshift to wait React render call to complete before closeMenu to be triggered.
      setTimeout(downshift.closeMenu);
      return null;
    }

    const matches = fuzzy.filter(inputValue.replace(/ /g, ''), this.props.metricNames, {
      pre: "<strong>",
      post: "</strong>",
    });

    if (matches.length === 0) {
      this.prevNoMatchValue = inputValue;
      setTimeout(downshift.closeMenu);
      return null;
    }

    return (
      <ul className="autosuggest-dropdown" {...downshift.getMenuProps()}>
        {
          matches
            .slice(0, 200) // Limit DOM rendering to 100 results, as DOM rendering is sloooow.
            .map((item, index) => (
              <li
                {...downshift.getItemProps({
                  key: item.original,
                  index,
                  item: item.original,
                  style: {
                    backgroundColor:
                      downshift.highlightedIndex === index ? 'lightgray' : 'white',
                    fontWeight: downshift.selectedItem === item ? 'bold' : 'normal',
                  },
                })}
              >
                <SanitizeHTML inline={true} allowedTags={['strong']}>
                  {item.string}
                </SanitizeHTML>
              </li>
            ))
        }
      </ul>
    );
  }

  render() {
    const { value, height } = this.state;
    return (
      <Downshift
        onChange={this.handleDropdownSelection}
        inputValue={value}
      >
        {(downshift) => (
          <div>
            <InputGroup className="expression-input">
              <InputGroupAddon addonType="prepend">
                <InputGroupText>
                {this.props.loading ? <FontAwesomeIcon icon="spinner" spin/> : <FontAwesomeIcon icon="search"/>}
                </InputGroupText>
              </InputGroupAddon>
              <Input
                onInput={this.handleInput}
                style={{ height }}
                autoFocus
                type="textarea"
                rows="1"
                onKeyPress={this.handleKeyPress}
                placeholder="Expression (press Shift+Enter for newlines)"
                innerRef={this.exprInputRef}
                {...downshift.getInputProps({
                  onKeyDown: (event: React.KeyboardEvent): void => {
                    switch (event.key) {
                      case 'Home':
                      case 'End':
                        // We want to be able to jump to the beginning/end of the input field.
                        // By default, Downshift otherwise jumps to the first/last suggestion item instead.
                        (event.nativeEvent as any).preventDownshiftDefault = true;
                        break;
                      case 'ArrowUp':
                      case 'ArrowDown':
                        if (!downshift.isOpen) {
                          (event.nativeEvent as any).preventDownshiftDefault = true;
                        }
                        break;
                      case 'Enter':
                        downshift.closeMenu();
                        break;
                      case 'Escape':
                        if (!downshift.isOpen) {
                          this.exprInputRef.current!.blur();
                        }
                        break;
                      default:
                    }
                  }
                } as any)}
              />
              <InputGroupAddon addonType="append">
                <Button
                  className="execute-btn"
                  color="primary"
                  onClick={this.executeQuery}
                >
                  Execute
                </Button>
              </InputGroupAddon>
            </InputGroup>
            {downshift.isOpen && this.renderAutosuggest(downshift)}
          </div>
        )}
      </Downshift>
    );
  }
}

export default ExpressionInput;
