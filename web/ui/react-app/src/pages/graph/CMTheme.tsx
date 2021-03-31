import { HighlightStyle, tags } from '@codemirror/highlight';
import { EditorView } from '@codemirror/view';

export const theme = EditorView.theme({
  '&': {
    '&.cm-focused': {
      outline: 'none',
      outline_fallback: 'none',
    },
  },
  '.cm-scroller': {
    overflow: 'hidden',
    fontFamily: '"DejaVu Sans Mono", monospace',
  },
  '.cm-placeholder': {
    fontFamily:
      '-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,"Helvetica Neue",Arial,"Noto Sans","Liberation Sans",sans-serif,"Apple Color Emoji","Segoe UI Emoji","Segoe UI Symbol","Noto Color Emoji"',
  },

  '.cm-matchingBracket': {
    color: '#000',
    backgroundColor: '#dedede',
    fontWeight: 'bold',
    outline: '1px dashed transparent',
  },
  '.cm-nonmatchingBracket': { borderColor: 'red' },

  '.cm-tooltip': {
    backgroundColor: '#f8f8f8',
    borderColor: 'rgba(52, 79, 113, 0.2)',
  },

  '.cm-tooltip.cm-tooltip-autocomplete': {
    '& > ul': {
      maxHeight: '350px',
      fontFamily: '"DejaVu Sans Mono", monospace',
      maxWidth: 'unset',
    },
    '& > ul > li': {
      padding: '2px 1em 2px 3px',
    },
    '& li:hover': {
      backgroundColor: '#ddd',
    },
    '& > ul > li[aria-selected]': {
      backgroundColor: '#d6ebff',
      color: 'unset',
    },
    minWidth: '30%',
  },

  '.cm-completionDetail': {
    float: 'right',
    color: '#999',
  },

  '.cm-tooltip.cm-completionInfo': {
    marginTop: '-11px',
    padding: '10px',
    fontFamily: "'Open Sans', 'Lucida Sans Unicode', 'Lucida Grande', sans-serif;",
    border: 'none',
    backgroundColor: '#d6ebff',
    minWidth: '250px',
    maxWidth: 'min-content',
  },

  '.cm-completionInfo.cm-completionInfo-right': {
    '&:before': {
      content: "' '",
      height: '0',
      position: 'absolute',
      width: '0',
      left: '-20px',
      border: '10px solid transparent',
      borderRightColor: '#d6ebff',
    },
    marginLeft: '12px',
  },
  '.cm-completionInfo.cm-completionInfo-left': {
    '&:before': {
      content: "' '",
      height: '0',
      position: 'absolute',
      width: '0',
      right: '-20px',
      border: '10px solid transparent',
      borderLeftColor: '#d6ebff',
    },
    marginRight: '12px',
  },

  '.cm-completionMatchedText': {
    textDecoration: 'none',
    fontWeight: 'bold',
    color: '#0066bf',
  },

  '.cm-line': {
    '&::selection': {
      backgroundColor: '#add6ff',
    },
    '& > span::selection': {
      backgroundColor: '#add6ff',
    },
  },

  '.cm-selectionMatch': {
    backgroundColor: '#e6f3ff',
  },

  '.cm-diagnostic': {
    '&.cm-diagnostic-error': {
      borderLeft: '3px solid #e65013',
    },
  },

  '.cm-completionIcon': {
    boxSizing: 'content-box',
    fontSize: '16px',
    lineHeight: '1',
    marginRight: '10px',
    verticalAlign: 'top',
    '&:after': { content: "'\\ea88'" },
    fontFamily: 'codicon',
    paddingRight: '0',
    opacity: '1',
    color: '#007acc',
  },

  '.cm-completionIcon-function, .cm-completionIcon-method': {
    '&:after': { content: "'\\ea8c'" },
    color: '#652d90',
  },
  '.cm-completionIcon-class': {
    '&:after': { content: "'○'" },
  },
  '.cm-completionIcon-interface': {
    '&:after': { content: "'◌'" },
  },
  '.cm-completionIcon-variable': {
    '&:after': { content: "'𝑥'" },
  },
  '.cm-completionIcon-constant': {
    '&:after': { content: "'\\eb5f'" },
    color: '#007acc',
  },
  '.cm-completionIcon-type': {
    '&:after': { content: "'𝑡'" },
  },
  '.cm-completionIcon-enum': {
    '&:after': { content: "'∪'" },
  },
  '.cm-completionIcon-property': {
    '&:after': { content: "'□'" },
  },
  '.cm-completionIcon-keyword': {
    '&:after': { content: "'\\eb62'" },
    color: '#616161',
  },
  '.cm-completionIcon-namespace': {
    '&:after': { content: "'▢'" },
  },
  '.cm-completionIcon-text': {
    '&:after': { content: "'\\ea95'" },
    color: '#ee9d28',
  },
});

export const promqlHighlighter = HighlightStyle.define([
  { tag: tags.name, color: '#000' },
  { tag: tags.number, color: '#09885a' },
  { tag: tags.string, color: '#a31515' },
  { tag: tags.keyword, color: '#008080' },
  { tag: tags.function(tags.variableName), color: '#008080' },
  { tag: tags.labelName, color: '#800000' },
  { tag: tags.operator },
  { tag: tags.modifier, color: '#008080' },
  { tag: tags.paren },
  { tag: tags.squareBracket },
  { tag: tags.brace },
  { tag: tags.invalid, color: 'red' },
  { tag: tags.comment, color: '#888', fontStyle: 'italic' },
]);
