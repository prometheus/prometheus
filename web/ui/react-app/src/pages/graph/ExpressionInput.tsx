import React, { Component } from 'react';
import { Button, InputGroup, InputGroupAddon, InputGroupText, Input } from 'reactstrap';

import Downshift, { ControllerStateAndHelpers } from 'downshift';
import fuzzy from 'fuzzy';
import sanitizeHTML from 'sanitize-html';

import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faSpinner } from '@fortawesome/free-solid-svg-icons';

interface ExpressionInputProps {
  value: string;
  onExpressionChange: (expr: string) => void;
  autocompleteSections: { [key: string]: string[] };
  executeQuery: () => void;
  loading: boolean;
}

interface ExpressionInputState {
  height: number | string;
}

class ExpressionInput extends Component<ExpressionInputProps, ExpressionInputState> {
  private exprInputRef = React.createRef<HTMLInputElement>();

  constructor(props: ExpressionInputProps) {
    super(props);
    this.state = {
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
    this.setValue(this.exprInputRef.current!.value);
  };

  setValue = (value: string) => {
    const { onExpressionChange } = this.props;
    onExpressionChange(value);
    this.setState({ height: 'auto' }, this.setHeight);
  };

  componentDidUpdate(prevProps: ExpressionInputProps) {
    const { value } = this.props;
    if (value !== prevProps.value) {
      this.setValue(value);
    }
  }

  handleKeyPress = (event: React.KeyboardEvent<HTMLInputElement>) => {
    const { executeQuery } = this.props;
    if (event.key === 'Enter' && !event.shiftKey) {
      executeQuery();
      event.preventDefault();
    }
  };

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
      ? Object.entries(autocompleteSections).reduce((acc, [title, items]) => {
          const matches = this.getSearchMatches(inputValue!, items);
          return !matches.length
            ? acc
            : [
                ...acc,
                <ul className="autosuggest-dropdown-list" key={title}>
                  <li className="autosuggest-dropdown-header">{title}</li>
                  {matches
                    .slice(0, 100) // Limit DOM rendering to 100 results, as DOM rendering is sloooow.
                    .map(({ original, string: text }) => {
                      const itemProps = downshift.getItemProps({
                        key: original,
                        index,
                        item: original,
                        style: {
                          backgroundColor: highlightedIndex === index++ ? 'lightgray' : 'white',
                        },
                      });
                      return (
                        <li
                          key={title}
                          {...itemProps}
                          dangerouslySetInnerHTML={{ __html: sanitizeHTML(text, { allowedTags: ['strong'] }) }}
                        />
                      );
                    })}
                </ul>,
              ];
        }, [] as JSX.Element[])
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
    const { executeQuery, value } = this.props;
    const { height } = this.state;
    return (
      <Downshift onSelect={this.setValue}>
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
                <Button className="execute-btn" color="primary" onClick={executeQuery}>
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
