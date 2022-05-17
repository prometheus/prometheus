import React, { FC, useEffect, useRef } from 'react';
import { InputGroup, InputGroupAddon, InputGroupText } from 'reactstrap';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faSearch } from '@fortawesome/free-solid-svg-icons';
import { EditorState } from '@codemirror/state';
import { baseTheme, lightTheme, promqlHighlighter } from '../pages/graph/CMTheme';
import { EditorView, highlightSpecialChars, keymap, placeholder as placeholderFunc, ViewUpdate } from '@codemirror/view';
import { history, historyKeymap } from '@codemirror/history';
import { indentOnInput } from '@codemirror/language';
import { bracketMatching } from '@codemirror/matchbrackets';
import { closeBrackets, closeBracketsKeymap } from '@codemirror/closebrackets';
import { autocompletion, completionKeymap } from '@codemirror/autocomplete';
import { highlightSelectionMatches } from '@codemirror/search';
import { defaultKeymap } from '@codemirror/commands';
import { commentKeymap } from '@codemirror/comment';
import { lintKeymap } from '@codemirror/lint';
import { KVSearchExtension } from '@nexucis/codemirror-kvsearch';

export interface SearchBarProps {
  handleChange: (state: EditorState) => void;
  placeholder: string;
  defaultValue: string;
  objects?: Record<string, unknown>[];
}

const SearchBar: FC<SearchBarProps> = ({ handleChange, placeholder, defaultValue, objects }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const filterTimeoutRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    const kvsearchExtension = new KVSearchExtension(objects);
    const handleSearchChange = (state: EditorState) => {
      let filterTimeout = filterTimeoutRef.current;
      if (filterTimeout !== null) {
        clearTimeout(filterTimeout);
      }

      filterTimeout = setTimeout(() => {
        handleChange(state);
      }, 300);
      filterTimeoutRef.current = filterTimeout;
    };
    // Create or reconfigure the editor.
    let view = viewRef.current;
    if (view === null) {
      // If the editor does not exist yet, create it.
      if (!containerRef.current) {
        throw new Error('expected CodeMirror container element to exist');
      }
      const startState = EditorState.create({
        doc: defaultValue,
        extensions: [
          baseTheme,
          promqlHighlighter,
          lightTheme,
          highlightSpecialChars(),
          history(),
          EditorState.allowMultipleSelections.of(true),
          indentOnInput(),
          bracketMatching(),
          closeBrackets(),
          autocompletion(),
          highlightSelectionMatches(),
          EditorView.lineWrapping,
          keymap.of([
            ...closeBracketsKeymap,
            ...defaultKeymap,
            ...historyKeymap,
            ...commentKeymap,
            ...completionKeymap,
            ...lintKeymap,
          ]),
          placeholderFunc(placeholder),
          keymap.of([
            {
              key: 'Escape',
              run: (v: EditorView): boolean => {
                v.contentDOM.blur();
                return false;
              },
            },
          ]),
          EditorView.updateListener.of((update: ViewUpdate): void => {
            handleSearchChange(update.state);
          }),
          kvsearchExtension.asExtension(),
        ],
      });

      view = new EditorView({
        state: startState,
        parent: containerRef.current,
      });
      viewRef.current = view;

      view.focus();
    }
    handleChange(view.state);
  }, [defaultValue, handleChange, objects, placeholder]);

  return (
    <InputGroup>
      <InputGroupAddon addonType="prepend">
        <InputGroupText>{<FontAwesomeIcon icon={faSearch} />}</InputGroupText>
      </InputGroupAddon>
      <div ref={containerRef} className="cm-expression-input" />
    </InputGroup>
  );
};

export default SearchBar;
