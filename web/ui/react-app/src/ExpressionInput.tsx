import React, { Component } from 'react';
import {
  Button,
  InputGroup,
  InputGroupAddon,
  InputGroupText,
  Input,
} from 'reactstrap';

import Downshift, { ChildrenFunction, ControllerStateAndHelpers, DownshiftInterface } from 'downshift';
import fuzzy from 'fuzzy';

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
  height: number | string
  value: string
}

class ExpressionInput extends Component<ExpressionInputProps, ExpressionInputState> {
  prevNoMatchValue: string | null = null;
  private exprInputRef = React.createRef<HTMLInputElement>();

  constructor(props: ExpressionInputProps) {
    super(props)
    this.state = {
      value: props.value,
      height: 'auto'
    }
  }

  componentDidMount() {
    this.setHeight()
  }

  handleKeyPress = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      this.props.executeQuery(this.exprInputRef.current!.value);
      event.preventDefault();
    }
  }

  renderAutosuggest = (downshift: any) => {
    if (!downshift.isOpen) {
      return null;
    }

    if (this.prevNoMatchValue && downshift.inputValue.includes(this.prevNoMatchValue)) {
      return null;
    }

    let matches = fuzzy.filter(downshift.inputValue.replace(/ /g, ''), this.props.metricNames, {
      pre: "<strong>",
      post: "</strong>",
    });

    if (matches.length === 0) {
      this.prevNoMatchValue = downshift.inputValue;
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
                {/* TODO: Find better way than setting inner HTML dangerously. We just want the <strong> to not be escaped.
                    This will be a problem when we save history and the user enters HTML into a query. */}
                <span dangerouslySetInnerHTML={{__html: item.string}}></span>
              </li>
            ))
        }
      </ul>
    );
  }

  setHeight = () => {
    const { offsetHeight, clientHeight, scrollHeight } = this.exprInputRef.current!;
    const offset = offsetHeight - clientHeight; // needed in order height to be more accurate
    this.setState({ height: scrollHeight + offset });
  }

  handleInput = () => {
    this.setState({
      height: 'auto',
      value: this.exprInputRef.current!.value
    }, this.setHeight);
  }

  handleDropdownSelection = (value: string) => this.setState({ value });

  render() {
    const { value, height } = this.state;
    return (
        <Downshift
          //inputValue={this.props.value}
          //onInputValueChange={this.props.onChange}
          inputValue={value}
          onChange={this.handleDropdownSelection}
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
                  <Button className="execute-btn" color="primary" onClick={() => this.props.executeQuery(this.exprInputRef.current!.value)}>Execute</Button>
                </InputGroupAddon>
              </InputGroup>
              {this.renderAutosuggest(downshift)}
            </div>
          )}
        </Downshift>
    );
  }
}

export default ExpressionInput;
