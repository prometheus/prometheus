import { EditorView } from '@codemirror/view';
import { HighlightStyle, tags } from '@codemirror/highlight';

// promQLHighlightMaterialTheme is based on the material theme defined here:
// https://codemirror.net/theme/material.css
export const promQLHighlightMaterialTheme = HighlightStyle.define([
  {
    tag: tags.deleted,
    textDecoration: 'line-through',
  },
  {
    tag: tags.inserted,
    textDecoration: 'underline',
  },
  {
    tag: tags.link,
    textDecoration: 'underline',
  },
  {
    tag: tags.strong,
    fontWeight: 'bold',
  },
  {
    tag: tags.emphasis,
    fontStyle: 'italic',
  },
  {
    tag: tags.invalid,
    color: '#f00',
  },
  {
    tag: tags.keyword,
    color: '#C792EA',
  },
  {
    tag: tags.operator,
    color: '#89DDFF',
  },
  {
    tag: tags.atom,
    color: '#F78C6C',
  },
  {
    tag: tags.number,
    color: '#FF5370',
  },
  {
    tag: tags.string,
    color: '#99b867',
  },
  {
    tag: [tags.escape, tags.regexp],
    color: '#e40',
  },
  {
    tag: tags.definition(tags.variableName),
    color: '#f07178',
  },
  {
    tag: tags.labelName,
    color: '#f07178',
  },
  {
    tag: tags.typeName,
    color: '#085',
  },
  {
    tag: tags.function(tags.variableName),
    color: '#C792EA',
  },
  {
    tag: tags.definition(tags.propertyName),
    color: '#00c',
  },
  {
    tag: tags.comment,
    color: '#546E7A',
  },
]);

export const customTheme = EditorView.theme({
  $completionDetail: {
    marginLeft: '0.5em',
    float: 'right',
    color: '#9d4040',
  },
  $completionMatchedText: {
    color: '#83080a',
    textDecoration: 'none',
    fontWeight: 'bold',
  },
});
