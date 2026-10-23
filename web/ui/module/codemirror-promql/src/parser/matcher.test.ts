// Copyright 2021 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import {
  EqlRegex,
  EqlSingle,
  LabelMatchers,
  Neq,
  NeqRegex,
  QuotedLabelMatcher,
  QuotedLabelName,
  UnquotedLabelMatcher,
} from '@prometheus-io/lezer-promql';
import { syntaxTree } from '@codemirror/language';
import { buildLabelMatchers, labelMatchersToString } from './matcher';
import { Matcher } from '../types';
import { createEditorState } from '../test/utils-test';

describe('labelMatchersToString test', () => {
  const testCases = [
    {
      title: 'metric_name',
      metricName: 'metric_name',
      labelName: undefined,
      matchers: [] as Matcher[],
      result: 'metric_name',
    },
    {
      title: 'metric_name 2',
      metricName: 'metric_name',
      labelName: undefined,
      matchers: undefined,
      result: 'metric_name',
    },
    {
      title: 'metric_name{}',
      metricName: 'metric_name',
      labelName: undefined,
      matchers: [
        {
          type: EqlSingle,
          name: 'LabelName',
          value: '',
        },
      ] as Matcher[],
      result: 'metric_name{}',
    },
    {
      title: 'sum{LabelName!="LabelValue"}',
      metricName: 'sum',
      labelName: undefined,
      matchers: [
        {
          type: Neq,
          name: 'LabelName',
          value: 'LabelValue',
        },
      ] as Matcher[],
      result: 'sum{LabelName!="LabelValue"}',
    },
    {
      title: 'rate{LabelName=~"label.+"}',
      metricName: 'rate',
      labelName: undefined,
      matchers: [
        {
          type: EqlSingle,
          name: 'LabelName',
          value: '',
        },
        {
          type: EqlRegex,
          name: 'LabelName',
          value: 'label.+',
        },
      ] as Matcher[],
      result: 'rate{LabelName=~"label.+"}',
    },
    {
      title: 'rate{LabelName="l1",labelName2=~"label.+",labelName3!~"label.+"}',
      metricName: 'rate',
      labelName: undefined,
      matchers: [
        {
          type: EqlSingle,
          name: 'LabelName',
          value: 'l1',
        },
        {
          type: EqlRegex,
          name: 'labelName2',
          value: 'label.+',
        },
        {
          type: NeqRegex,
          name: 'labelName3',
          value: 'label.+',
        },
      ] as Matcher[],
      result: 'rate{LabelName="l1",labelName2=~"label.+",labelName3!~"label.+"}',
    },
    {
      title: 'utf-8 metric name without matchers',
      metricName: 'http.requests',
      labelName: undefined,
      matchers: [] as Matcher[],
      result: '{"http.requests"}',
    },
    {
      title: 'utf-8 metric name, utf-8 label name and value to escape',
      metricName: 'http.requests',
      labelName: undefined,
      matchers: [
        { type: EqlSingle, name: 'http.method', value: 'say "hi"\\' },
        { type: Neq, name: 'code', value: '200' },
      ] as Matcher[],
      result: '{"http.requests","http.method"="say \\"hi\\"\\\\",code!="200"}',
    },
    {
      title: 'utf-8 metric name when all the matchers are skipped',
      metricName: 'http.requests',
      labelName: 'a',
      matchers: [
        { type: EqlSingle, name: 'a', value: 'b' },
        { type: EqlSingle, name: 'c', value: '' },
      ] as Matcher[],
      result: '{"http.requests"}',
    },
    {
      title: 'quoted metric name already in the matchers is not repeated',
      metricName: 'http.requests',
      labelName: 'a',
      matchers: [
        { type: EqlSingle, name: '__name__', value: 'http.requests' },
        { type: EqlSingle, name: 'c', value: 'd' },
      ] as Matcher[],
      result: '{"http.requests",c="d"}',
    },
    {
      title: 'other __name__ matchers are kept',
      metricName: 'http.requests',
      labelName: undefined,
      matchers: [{ type: EqlRegex, name: '__name__', value: 'http.*' }] as Matcher[],
      result: '{"http.requests",__name__=~"http.*"}',
    },
    {
      title: 'legacy metric name with a colon',
      metricName: 'job:rate:5m',
      labelName: undefined,
      matchers: [] as Matcher[],
      result: 'job:rate:5m',
    },
    {
      title: 'label name containing a colon has to be quoted',
      metricName: 'metric_name',
      labelName: undefined,
      matchers: [{ type: EqlSingle, name: 'a:b', value: 'c' }] as Matcher[],
      result: 'metric_name{"a:b"="c"}',
    },
    {
      title: 'label name that is a keyword-like legacy name is not quoted',
      metricName: 'metric_name',
      labelName: undefined,
      matchers: [{ type: EqlSingle, name: '__name__', value: 'sum' }] as Matcher[],
      result: 'metric_name{__name__="sum"}',
    },
    {
      title: 'value containing line breaks and tabs is escaped',
      metricName: 'metric_name',
      labelName: undefined,
      matchers: [{ type: EqlSingle, name: 'a', value: 'x\ny\tz\r' }] as Matcher[],
      result: 'metric_name{a="x\\ny\\tz\\r"}',
    },
    {
      title: 'regex value keeps its backslashes',
      metricName: 'metric_name',
      labelName: undefined,
      matchers: [{ type: EqlRegex, name: 'a', value: 'x\\.y' }] as Matcher[],
      result: 'metric_name{a=~"x\\\\.y"}',
    },
    {
      title: 'utf-8 label name and value, no metric name',
      metricName: '',
      labelName: undefined,
      matchers: [{ type: NeqRegex, name: 'é.x', value: '😀' }] as Matcher[],
      result: '{"é.x"!~"😀"}',
    },
    {
      title: 'rate{LabelName="l1",labelName2=~"label.+",labelName3!~"label.+"}',
      metricName: 'rate',
      labelName: 'LabelName',
      matchers: [
        {
          type: EqlSingle,
          name: 'LabelName',
          value: 'l1',
        },
        {
          type: EqlRegex,
          name: 'labelName2',
          value: 'label.+',
        },
        {
          type: NeqRegex,
          name: 'labelName3',
          value: 'label.+',
        },
        {
          type: Neq,
          name: 'labelName4',
          value: '',
        },
      ] as Matcher[],
      result: 'rate{labelName2=~"label.+",labelName3!~"label.+"}',
    },
  ];

  testCases.forEach((value) => {
    it(value.title, () => {
      expect(labelMatchersToString(value.metricName, value.matchers, value.labelName)).toEqual(value.result);
    });
  });
});

describe('buildLabelMatchers test', () => {
  const testCases = [
    { title: 'unquoted label name', expr: '{a="b"}', expected: { type: EqlSingle, name: 'a', value: 'b' } },
    { title: 'quoted label name', expr: '{"a.b"="c"}', expected: { type: EqlSingle, name: 'a.b', value: 'c' } },
    { title: 'single-quoted label name and value', expr: "{'a.b'!='c'}", expected: { type: Neq, name: 'a.b', value: 'c' } },
    { title: 'backtick value keeps its backslashes', expr: '{a=~`x\\.y`}', expected: { type: EqlRegex, name: 'a', value: 'x\\.y' } },
    { title: 'double-quoted regex value', expr: '{a!~"x\\\\.y"}', expected: { type: NeqRegex, name: 'a', value: 'x\\.y' } },
    { title: 'escaped quotes in the value', expr: '{a="say \\"hi\\""}', expected: { type: EqlSingle, name: 'a', value: 'say "hi"' } },
    { title: 'escaped quote in the name', expr: '{"a\\"b"="c"}', expected: { type: EqlSingle, name: 'a"b', value: 'c' } },
    { title: 'unicode escapes', expr: '{a="\\u00e9\\U0001F600"}', expected: { type: EqlSingle, name: 'a', value: 'é😀' } },
    { title: 'hexadecimal and octal escapes', expr: '{a="\\x41\\102"}', expected: { type: EqlSingle, name: 'a', value: 'AB' } },
    { title: 'hexadecimal escapes of an UTF-8 sequence', expr: '{a="\\xc3\\xa9"}', expected: { type: EqlSingle, name: 'a', value: 'é' } },
    { title: 'octal escapes of an UTF-8 sequence', expr: '{a="\\303\\251"}', expected: { type: EqlSingle, name: 'a', value: 'é' } },
    { title: 'unknown escape is kept', expr: '{a="\\q"}', expected: { type: EqlSingle, name: 'a', value: '\\q' } },
    { title: 'quoted metric name', expr: '{"foo.bar"}', expected: { type: EqlSingle, name: '__name__', value: 'foo.bar' } },
    { title: 'unterminated value ending with an escaped quote', expr: '{a="x\\"', expected: { type: EqlSingle, name: 'a', value: 'x"' } },
  ];
  testCases.forEach((value) => {
    it(value.title, () => {
      const state = createEditorState(value.expr);
      const matchers = syntaxTree(state).topNode.getChild('VectorSelector')?.getChild(LabelMatchers);
      expect(matchers).toBeTruthy();
      const nodes = [QuotedLabelName, QuotedLabelMatcher, UnquotedLabelMatcher].flatMap((id) => matchers?.getChildren(id) ?? []);
      expect(buildLabelMatchers(nodes, state)).toEqual([value.expected]);
    });
  });
});
