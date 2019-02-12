import React, { Component } from 'react';
import {
  Button,
  InputGroup,
  InputGroupAddon,
  InputGroupText,
  Input,
} from 'reactstrap';

import Downshift from 'downshift';
import fuzzy from 'fuzzy';

import { library } from '@fortawesome/fontawesome-svg-core';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch, faSpinner } from '@fortawesome/free-solid-svg-icons';

library.add(faSearch, faSpinner);

class ExpressionInput extends Component {
  handleKeyPress = (event) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      this.props.executeQuery();
      event.preventDefault();
    }
  }

  stateReducer = (state, changes) => {
    return changes;
    // // TODO: Remove this whole function if I don't notice any odd behavior without it.
    // // I don't remember why I had to add this and currently things seem fine without it.
    // switch (changes.type) {
    //   case Downshift.stateChangeTypes.keyDownEnter:
    //   case Downshift.stateChangeTypes.clickItem:
    //   case Downshift.stateChangeTypes.changeInput:
    //     return {
    //       ...changes,
    //       selectedItem: changes.inputValue,
    //     };
    //   default:
    //     return changes;
    // }
  }

  renderAutosuggest = (downshift) => {
    if (this.prevNoMatchValue && downshift.inputValue.includes(this.prevNoMatchValue)) {
      // TODO: Is this still correct with fuzzy?
      return null;
    }

    let matches = fuzzy.filter(downshift.inputValue.replace(/ /g, ''), this.props.metrics, {
      pre: "<strong>",
      post: "</strong>",
    });

    if (matches.length === 0) {
      this.prevNoMatchValue = downshift.inputValue;
      return null;
    }

    if (!downshift.isOpen) {
      return null; // TODO CHECK NEED FOR THIS
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
                    This will be a problem when we save history and the user enters HTML into a query. q*/}
                <span dangerouslySetInnerHTML={{__html: item.string}}></span>
              </li>
            ))
        }
      </ul>
    );
  }

  componentDidMount() {
    const $exprInput = window.$(this.exprInputRef);
    $exprInput.on('input', () => {
      const el = $exprInput.get(0);
      const offset = el.offsetHeight - el.clientHeight;
      $exprInput.css('height', 'auto').css('height', el.scrollHeight + offset);
    });
  }

  render() {
    return (
        <Downshift
          inputValue={this.props.value}
          onInputValueChange={this.props.onChange}
          selectedItem={this.props.value}
        >
          {downshift => (
            <div>
              <InputGroup className="expression-input">
                <InputGroupAddon addonType="prepend">
                  <InputGroupText>
                  {this.props.loading ? <FontAwesomeIcon icon="spinner" spin/> : <FontAwesomeIcon icon="search"/>}
                  </InputGroupText>
                </InputGroupAddon>

                <Input
                  autoFocus
                  type="textarea"
                  rows={1}
                  onKeyPress={this.handleKeyPress}
                  placeholder="Expression (press Shift+Enter for newlines)"
                  innerRef={ref => this.exprInputRef = ref}
                  //onChange={selection => alert(`You selected ${selection}`)}
                  {...downshift.getInputProps({
                    onKeyDown: event => {
                      switch (event.key) {
                        case 'Home':
                        case 'End':
                          // We want to be able to jump to the beginning/end of the input field.
                          // By default, Downshift otherwise jumps to the first/last suggestion item instead.
                          event.nativeEvent.preventDownshiftDefault = true;
                          break;
                        case 'Enter':
                          downshift.closeMenu();
                          break;
                        default:
                      }
                    }
                  })}
                />
                <InputGroupAddon addonType="append">
                  <Button className="execute-btn" color="primary" onClick={this.props.executeQuery}>Execute</Button>
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
