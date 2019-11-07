import React, { Component } from 'react';
import { Button, InputGroup, InputGroupAddon, InputGroupText, Input } from 'reactstrap';

import Downshift, { ControllerStateAndHelpers } from 'downshift';
import fuzzy from 'fuzzy';
import SanitizeHTML from './components/SanitizeHTML';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faSpinner } from '@fortawesome/free-solid-svg-icons';

interface ExpressionInputProps {
  value: string;
  autocompleteSections: { [key: string]: string[] };
  executeQuery: (expr: string) => void;
  loading: boolean;
}

interface ExpressionInputState {
  height: number | string;
  value: string;
}

class ExpressionInput extends Component<ExpressionInputProps, ExpressionInputState> {
  private exprInputRef = React.createRef<HTMLInputElement>();

  constructor(props: ExpressionInputProps) {
    super(props);
    this.state = {
      value: props.value,
      height: 'auto',
    };
  }

  componentDidMount() {
    this.setHeight();
  }

  setHeight = () => {
    const { offsetHeight, clientHeight, scrollHeight } = this.exprInputRef.current!;
    const offset = offsetHeight - clientHeight; // Needed in order for the height to be more accurate.
    this.setState({ height: scrollHeight + offset });
  };

  handleInput = () => {
    this.setState(
      {
        height: 'auto',
        value: this.exprInputRef.current!.value,
      },
      this.setHeight
    );
  };

  handleDropdownSelection = (value: string) => {
    this.setState({ value, height: 'auto' }, this.setHeight);
  };

  handleKeyPress = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      this.executeQuery();
      event.preventDefault();
    }
  };

  executeQuery = () => this.props.executeQuery(this.exprInputRef.current!.value);

  getSearchMatches = (input: string, expressions: string[]) => {
    return fuzzy.filter(input.replace(/ /g, ''), expressions, {
      pre: '<strong>',
      post: '</strong>',
    });
  };

  createAutocompleteSection = (downshift: ControllerStateAndHelpers<any>) => {
    const { inputValue = '', closeMenu, highlightedIndex } = downshift;
    const { autocompleteSections } = this.props;
    let index = 0;
    const sections = inputValue!.length
      ? Object.entries(autocompleteSections).reduce(
          (acc, [title, items]) => {
            const matches = this.getSearchMatches(inputValue!, items);
            return !matches.length
              ? acc
              : [
                  ...acc,
                  <ul className="autosuggest-dropdown-list" key={title}>
                    <li className="autosuggest-dropdown-header">{title}</li>
                    {matches
                      .slice(0, 100) // Limit DOM rendering to 100 results, as DOM rendering is sloooow.
                      .map(({ original, string }) => {
                        const itemProps = downshift.getItemProps({
                          key: original,
                          index,
                          item: original,
                          style: {
                            backgroundColor: highlightedIndex === index++ ? 'lightgray' : 'white',
                          },
                        });
                        return (
                          <SanitizeHTML tag="li" {...itemProps} allowedTags={['strong']}>
                            {string}
                          </SanitizeHTML>
                        );
                      })}
                  </ul>,
                ];
          },
          [] as JSX.Element[]
        )
      : [];

    if (!sections.length) {
      // This is ugly but is needed in order to sync state updates.
      // This way we force downshift to wait React render call to complete before closeMenu to be triggered.
      setTimeout(closeMenu);
      return null;
    }

    return (
      <div {...downshift.getMenuProps()} className="autosuggest-dropdown">
        {sections}
      </div>
    );
  };

  render() {
    const { value, height } = this.state;
    return (
      <Downshift onSelect={this.handleDropdownSelection}>
        {downshift => (
          <div>
            <InputGroup className="expression-input">
              <InputGroupAddon addonType="prepend">
                <InputGroupText>
                  {this.props.loading ? <FontAwesomeIcon icon={faSpinner} spin /> : <FontAwesomeIcon icon={faSearch} />}
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
                  },
                } as any)}
                value={value}
              />
              <InputGroupAddon addonType="append">
                <Button className="execute-btn" color="primary" onClick={this.executeQuery}>
                  Execute
                </Button>
              </InputGroupAddon>
            </InputGroup>
            {downshift.isOpen && this.createAutocompleteSection(downshift)}
          </div>
        )}
      </Downshift>
    );
  }
}

export default ExpressionInput;
