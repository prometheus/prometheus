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

import { analyzeCompletion, computeStartCompletePosition, ContextKind } from './hybrid';
import { createEditorState, mockedMetricsTerms, mockPrometheusServer } from '../test/utils-test';
import { Completion, CompletionContext } from '@codemirror/autocomplete';
import {
  aggregateOpModifierTerms,
  aggregateOpTerms,
  atModifierTerms,
  binOpModifierTerms,
  binOpTerms,
  durationTerms,
  functionIdentifierTerms,
  matchOpTerms,
  numberTerms,
  snippets,
} from './promql.terms';
import { EqlSingle, Neq } from '@prometheus-io/lezer-promql';
import { syntaxTree } from '@codemirror/language';
import { newCompleteStrategy } from './index';

describe('analyzeCompletion test', () => {
  const testCases = [
    {
      title: 'empty expr',
      expr: '',
      pos: 0,
      expectedContext: [
        {
          kind: ContextKind.MetricName,
          metricName: '',
        },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.Number },
      ],
    },
    {
      title: 'simple metric & number autocompletion',
      expr: 'go_',
      pos: 3, // cursor is at the end of the expr
      expectedContext: [
        {
          kind: ContextKind.MetricName,
          metricName: 'go_',
        },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.Number },
      ],
    },
    {
      title: 'metric/function/aggregation autocompletion',
      expr: 'sum()',
      pos: 4,
      expectedContext: [
        {
          kind: ContextKind.MetricName,
          metricName: '',
        },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
      ],
    },
    {
      title: 'metric/function/aggregation autocompletion 2',
      expr: 'sum(rat)',
      pos: 7,
      expectedContext: [
        {
          kind: ContextKind.MetricName,
          metricName: 'rat',
        },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
      ],
    },
    {
      title: 'metric/function/aggregation autocompletion 3',
      expr: 'sum(rate())',
      pos: 9,
      expectedContext: [
        {
          kind: ContextKind.MetricName,
          metricName: '',
        },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
      ],
    },
    {
      title: 'metric/function/aggregation autocompletion 4',
      expr: 'sum(rate(my_))',
      pos: 12,
      expectedContext: [
        {
          kind: ContextKind.MetricName,
          metricName: 'my_',
        },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
      ],
    },
    {
      title: 'autocomplete binOp modifier or metric or number',
      expr: 'metric_name / ignor',
      pos: 19,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'ignor' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
      ],
    },
    {
      title: 'autocomplete binOp modifier or metric or number 2',
      expr: 'sum(http_requests_total{method="GET"} / o)',
      pos: 41,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'o' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
      ],
    },
    {
      title: 'autocomplete bool/binOp modifier/metric/number 1',
      expr: '1 > b)',
      pos: 5,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'b' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
        { kind: ContextKind.Bool },
      ],
    },
    {
      title: 'autocomplete bool/binOp modifier/metric/number 2',
      expr: '1 == b)',
      pos: 6,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'b' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
        { kind: ContextKind.Bool },
      ],
    },
    {
      title: 'autocomplete bool/binOp modifier/metric/number 3',
      expr: '1 != b)',
      pos: 6,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'b' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
        { kind: ContextKind.Bool },
      ],
    },
    {
      title: 'autocomplete bool/binOp modifier/metric/number 4',
      expr: '1 > b)',
      pos: 5,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'b' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
        { kind: ContextKind.Bool },
      ],
    },
    {
      title: 'autocomplete bool/binOp modifier/metric/number 5',
      expr: '1 >= b)',
      pos: 6,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'b' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
        { kind: ContextKind.Bool },
      ],
    },
    {
      title: 'autocomplete bool/binOp modifier/metric/number 6',
      expr: '1 <= b)',
      pos: 6,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'b' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
        { kind: ContextKind.Bool },
      ],
    },
    {
      title: 'autocomplete bool/binOp modifier/metric/number 7',
      expr: '1 < b)',
      pos: 5,
      expectedContext: [
        { kind: ContextKind.MetricName, metricName: 'b' },
        { kind: ContextKind.Function },
        { kind: ContextKind.Aggregation },
        { kind: ContextKind.BinOpModifier },
        { kind: ContextKind.Number },
        { kind: ContextKind.Bool },
      ],
    },
    {
      title: 'starting to autocomplete labelName in aggregate modifier',
      expr: 'sum by ()',
      pos: 8, // cursor is between the bracket
      expectedContext: [{ kind: ContextKind.LabelName, metricName: '' }],
    },
    {
      title: 'starting to autocomplete labelName in aggregate modifier with metric name',
      expr: 'sum(up) by ()',
      pos: 12, // cursor is between ()
      expectedContext: [{ kind: ContextKind.LabelName, metricName: 'up' }],
    },
    {
      title: 'starting to autocomplete labelName in aggregate modifier with metric name in front',
      expr: 'sum by ()(up)',
      pos: 8, // cursor is between ()
      expectedContext: [{ kind: ContextKind.LabelName, metricName: 'up' }],
    },
    {
      title: 'continue to autocomplete labelName in aggregate modifier',
      expr: 'sum by (myL)',
      pos: 11, // cursor is between the bracket after the string myL
      expectedContext: [{ kind: ContextKind.LabelName }],
    },
    {
      title: 'continue to autocomplete QuotedLabelName in aggregate modifier',
      expr: 'sum by ("myL")',
      pos: 12, // cursor is between the bracket after the string myL
      expectedContext: [{ kind: ContextKind.LabelName }],
    },
    {
      title: 'autocomplete labelName in a list',
      expr: 'sum by (myLabel1,)',
      pos: 17, // cursor is between the bracket after the string myLab
      expectedContext: [{ kind: ContextKind.LabelName, metricName: '' }],
    },
    {
      title: 'autocomplete labelName in a list 2',
      expr: 'sum by (myLabel1, myLab)',
      pos: 23, // cursor is between the bracket after the string myLab
      expectedContext: [{ kind: ContextKind.LabelName }],
    },
    {
      title: 'autocomplete labelName in a list 2',
      expr: 'sum by ("myLabel1", "myLab")',
      pos: 27, // cursor is between the bracket after the string myLab
      expectedContext: [{ kind: ContextKind.LabelName }],
    },
    {
      title: 'autocomplete labelName associated to a metric',
      expr: 'metric_name{}',
      pos: 12, // cursor is between the bracket
      expectedContext: [{ kind: ContextKind.LabelName, metricName: 'metric_name' }],
    },
    {
      title: 'autocomplete labelName that defined a metric',
      expr: '{}',
      pos: 1, // cursor is between the bracket
      expectedContext: [{ kind: ContextKind.LabelName, metricName: '' }],
    },
    {
      title: 'continue to autocomplete labelName associated to a metric',
      expr: 'metric_name{myL}',
      pos: 15, // cursor is between the bracket after the string myL
      expectedContext: [{ kind: ContextKind.LabelName, metricName: 'metric_name' }],
    },
    {
      title: 'continue to autocomplete labelName associated to a metric 2',
      expr: 'metric_name{myLabel="labelValue",}',
      pos: 33, // cursor is between the bracket after the comma
      expectedContext: [{ kind: ContextKind.LabelName, metricName: 'metric_name' }],
    },
    {
      title: 'continue autocomplete labelName that defined a metric',
      expr: '{myL}',
      pos: 4, // cursor is between the bracket after the string myL
      expectedContext: [{ kind: ContextKind.LabelName, metricName: '' }],
    },
    {
      title: 'continue autocomplete labelName that defined a metric 2',
      expr: '{myLabel="labelValue",}',
      pos: 22, // cursor is between the bracket after the comma
      expectedContext: [{ kind: ContextKind.LabelName, metricName: '' }],
    },
    {
      title: 'continue to autocomplete quoted labelName associated to a metric',
      expr: '{"metric_"}',
      pos: 10, // cursor is between the bracket after the string metric_
      expectedContext: [{ kind: ContextKind.MetricName, metricName: 'metric_' }],
    },
    {
      title: 'autocomplete the labelValue with metricName + labelName',
      expr: 'metric_name{labelName=""}',
      pos: 23, // cursor is between the quotes
      expectedContext: [
        {
          kind: ContextKind.LabelValue,
          metricName: 'metric_name',
          labelName: 'labelName',
          matchers: [
            {
              name: 'labelName',
              type: EqlSingle,
              value: '',
            },
          ],
        },
      ],
    },
    {
      title: 'autocomplete the labelValue with metricName + labelName 2',
      expr: 'metric_name{labelName="labelValue", labelName!=""}',
      pos: 48, // cursor is between the quotes
      expectedContext: [
        {
          kind: ContextKind.LabelValue,
          metricName: 'metric_name',
          labelName: 'labelName',
          matchers: [
            {
              name: 'labelName',
              type: EqlSingle,
              value: 'labelValue',
            },
            {
              name: 'labelName',
              type: Neq,
              value: '',
            },
          ],
        },
      ],
    },
    {
      title: 'autocomplete the labelValue with metricName + quoted labelName',
      expr: 'metric_name{labelName="labelValue", "labelName"!=""}',
      pos: 50, // cursor is between the quotes
      expectedContext: [
        {
          kind: ContextKind.LabelValue,
          metricName: 'metric_name',
          labelName: 'labelName',
          matchers: [
            {
              name: 'labelName',
              type: Neq,
              value: '',
            },
            {
              name: 'labelName',
              type: EqlSingle,
              value: 'labelValue',
            },
          ],
        },
      ],
    },
    {
      title: 'autocomplete the labelValue associated to a labelName',
      expr: '{labelName=""}',
      pos: 12, // cursor is between the quotes
      expectedContext: [
        {
          kind: ContextKind.LabelValue,
          metricName: '',
          labelName: 'labelName',
          matchers: [
            {
              name: 'labelName',
              type: EqlSingle,
              value: '',
            },
          ],
        },
      ],
    },
    {
      title: 'autocomplete the labelValue associated to a labelName 2',
      expr: '{labelName="labelValue", labelName!=""}',
      pos: 37, // cursor is between the quotes
      expectedContext: [
        {
          kind: ContextKind.LabelValue,
          metricName: '',
          labelName: 'labelName',
          matchers: [
            {
              name: 'labelName',
              type: EqlSingle,
              value: 'labelValue',
            },
            {
              name: 'labelName',
              type: Neq,
              value: '',
            },
          ],
        },
      ],
    },
    {
      title: 'autocomplete AggregateOpModifier or BinOp',
      expr: 'sum() b',
      pos: 7, // cursor is after the 'b'
      expectedContext: [{ kind: ContextKind.AggregateOpModifier }, { kind: ContextKind.BinOp }],
    },
    {
      title: 'autocomplete AggregateOpModifier or BinOp 2',
      expr: 'sum(rate(foo[5m])) an',
      pos: 21,
      expectedContext: [{ kind: ContextKind.AggregateOpModifier }, { kind: ContextKind.BinOp }],
    },
    {
      title: 'autocomplete AggregateOpModifier or BinOp or Offset',
      expr: 'sum b',
      pos: 5, // cursor is after 'b'
      expectedContext: [{ kind: ContextKind.AggregateOpModifier }, { kind: ContextKind.BinOp }, { kind: ContextKind.Offset }],
    },
    {
      title: 'autocomplete binOp',
      expr: 'metric_name !',
      pos: 13,
      expectedContext: [{ kind: ContextKind.BinOp }],
    },
    {
      title: 'autocomplete binOp 2',
      expr: 'metric_name =',
      pos: 13,
      expectedContext: [{ kind: ContextKind.BinOp }],
    },
    {
      title: 'autocomplete matchOp',
      expr: 'go{instance=""}',
      pos: 12, // cursor is after the 'equal'
      expectedContext: [{ kind: ContextKind.MatchOp }],
    },
    {
      title: 'autocomplete matchOp 2',
      expr: 'metric_name{labelName!}',
      pos: 22, // cursor is after '!'
      expectedContext: [{ kind: ContextKind.MatchOp }],
    },
    {
      title: 'autocomplete matchOp 3',
      expr: 'metric_name{"labelName"!}',
      pos: 24, // cursor is after '!'
      expectedContext: [{ kind: ContextKind.BinOp }],
    },
    {
      title: 'autocomplete duration with offset',
      expr: 'http_requests_total offset 5',
      pos: 28,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration with offset',
      expr: 'sum(http_requests_total{method="GET"} offset 4)',
      pos: 46,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete offset or binOp',
      expr: 'http_requests_total off',
      pos: 23,
      expectedContext: [{ kind: ContextKind.BinOp }, { kind: ContextKind.Offset }],
    },
    {
      title: 'autocomplete offset or binOp 2',
      expr: 'metric_name unle',
      pos: 16,
      expectedContext: [{ kind: ContextKind.BinOp }, { kind: ContextKind.Offset }],
    },
    {
      title: 'autocomplete offset or binOp 3',
      expr: 'http_requests_total{method="GET"} off',
      pos: 37,
      expectedContext: [{ kind: ContextKind.BinOp }, { kind: ContextKind.Offset }],
    },
    {
      title: 'autocomplete offset or binOp 4',
      expr: 'rate(foo[5m]) un',
      pos: 16,
      expectedContext: [{ kind: ContextKind.BinOp }, { kind: ContextKind.Offset }],
    },
    {
      title: 'autocomplete offset or binop 5',
      expr: 'sum(http_requests_total{method="GET"} off)',
      pos: 41,
      expectedContext: [{ kind: ContextKind.BinOp }, { kind: ContextKind.Offset }],
    },
    {
      title: 'not autocompleting duration for a matrixSelector',
      expr: 'go[]',
      pos: 3,
      expectedContext: [],
    },
    {
      title: 'not autocompleting duration for a matrixSelector 2',
      expr: 'go{l1="l2"}[]',
      pos: 12,
      expectedContext: [],
    },
    {
      title: 'autocomplete duration for a matrixSelector',
      expr: 'go[5]',
      pos: 4,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration for a matrixSelector 2',
      expr: 'go[5d1]',
      pos: 6,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration for a matrixSelector 3',
      expr: 'rate(my_metric{l1="l2"}[25])',
      pos: 26,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration for a matrixSelector 4',
      expr: 'rate(my_metric{l1="l2"}[25d5])',
      pos: 28,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration for a subQuery',
      expr: 'go[5d:5]',
      pos: 7,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration for a subQuery 2',
      expr: 'go[5d:5d4]',
      pos: 9,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration for a subQuery 3',
      expr: 'rate(my_metric{l1="l2"}[25d:6])',
      pos: 29,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete duration for a subQuery 4',
      expr: 'rate(my_metric{l1="l2"}[25d:6d5])',
      pos: 31,
      expectedContext: [{ kind: ContextKind.Duration }],
    },
    {
      title: 'autocomplete at modifiers',
      expr: '1 @ s',
      pos: 5,
      expectedContext: [{ kind: ContextKind.AtModifiers }],
    },
    {
      title: 'autocomplete topk params',
      expr: 'topk()',
      pos: 5,
      expectedContext: [{ kind: ContextKind.Number }],
    },
    {
      title: 'autocomplete topk params 2',
      expr: 'topk(inf,)',
      pos: 9,
      expectedContext: [{ kind: ContextKind.MetricName, metricName: '' }, { kind: ContextKind.Function }, { kind: ContextKind.Aggregation }],
    },
    {
      title: 'autocomplete topk params 3',
      expr: 'topk(inf,r)',
      pos: 10,
      expectedContext: [{ kind: ContextKind.MetricName, metricName: 'r' }, { kind: ContextKind.Function }, { kind: ContextKind.Aggregation }],
    },
    {
      title: 'autocomplete topk params 4',
      expr: 'topk by(instance) ()',
      pos: 19,
      expectedContext: [{ kind: ContextKind.Number }],
    },
    {
      title: 'autocomplete topk params 5',
      expr: 'topk by(instance) (inf,r)',
      pos: 24,
      expectedContext: [{ kind: ContextKind.MetricName, metricName: 'r' }, { kind: ContextKind.Function }, { kind: ContextKind.Aggregation }],
    },
  ];
  testCases.forEach((value) => {
    it(value.title, () => {
      const state = createEditorState(value.expr);
      const node = syntaxTree(state).resolve(value.pos, -1);
      const result = analyzeCompletion(state, node, value.pos);
      expect(value.expectedContext).toEqual(result);
    });
  });
});

describe('computeStartCompletePosition test', () => {
  const testCases = [
    {
      title: 'empty bracket',
      expr: '{}',
      pos: 1, // cursor is between the bracket
      expectedStart: 1,
    },
    {
      title: 'empty bracket 2',
      expr: 'metricName{}',
      pos: 11, // cursor is between the bracket
      expectedStart: 11,
    },
    {
      title: 'empty bracket 3',
      expr: 'sum by()',
      pos: 7, // cursor is between the bracket
      expectedStart: 7,
    },
    {
      title: 'empty bracket 4',
      expr: 'sum by(test) ()',
      pos: 14, // cursor is between the bracket
      expectedStart: 14,
    },
    {
      title: 'empty bracket 5',
      expr: 'sum()',
      pos: 4, // cursor is between the bracket
      expectedStart: 4,
    },
    {
      title: 'empty bracket 6',
      expr: 'sum(rate())',
      pos: 9, // cursor is between the bracket
      expectedStart: 9,
    },
    {
      title: 'bracket containing a substring',
      expr: '{myL}',
      pos: 4, // cursor is between the bracket
      expectedStart: 1,
    },
    {
      title: 'bracket containing a substring 2',
      expr: '{myLabel="LabelValue",}',
      pos: 22, // cursor is after the comma
      expectedStart: 22,
    },
    {
      title: 'bracket containing a substring 2',
      expr: 'metricName{myL}',
      pos: 14, // cursor is between the bracket
      expectedStart: 11,
    },
    {
      title: 'bracket containing a substring 3',
      expr: 'metricName{myLabel="LabelValue",}',
      pos: 32, // cursor is after the comma
      expectedStart: 32,
    },
    {
      title: 'bracket containing a substring 4',
      expr: 'sum by(myL)',
      pos: 10, // cursor is between the bracket
      expectedStart: 7,
    },
    {
      title: 'bracket containing a substring 5',
      expr: 'sum by(myLabel,)',
      pos: 15, // cursor is after the comma
      expectedStart: 15,
    },
    {
      title: 'bracket containing a substring 6',
      expr: 'sum(ra)',
      pos: 6, // cursor is between the bracket
      expectedStart: 4,
    },
    {
      title: 'bracket containing a substring 7',
      expr: 'sum(rate(my))',
      pos: 11, // cursor is between the bracket
      expectedStart: 9,
    },
    {
      title: 'start should not be at the beginning of the substring',
      expr: 'metric_name{labelName!}',
      pos: 22,
      expectedStart: 21,
    },
    {
      title: 'start should not be at the beginning of the substring 2',
      expr: 'metric_name{labelName!="labelValue"}',
      pos: 22,
      expectedStart: 21,
    },
    {
      title: 'start should be equal to the pos for the duration of an offset',
      expr: 'http_requests_total offset 5',
      pos: 28,
      expectedStart: 28,
    },
    {
      title: 'start should be equal to the pos for the duration of an offset 2',
      expr: 'http_requests_total offset 587',
      pos: 30,
      expectedStart: 30,
    },
    {
      title: 'start should be equal to the pos for the duration of an offset 3',
      expr: 'http_requests_total offset 587',
      pos: 29,
      expectedStart: 29,
    },
    {
      title: 'start should be equal to the pos for the duration of an offset 4',
      expr: 'sum(http_requests_total{method="GET"} offset 4)',
      pos: 46,
      expectedStart: 46,
    },
    {
      title: 'start should not be equal to the pos for the duration in a matrix selector',
      expr: 'go[]',
      pos: 3,
      expectedStart: 0,
    },
    {
      title: 'start should be equal to the pos for the duration in a matrix selector',
      expr: 'go[5]',
      pos: 4,
      expectedStart: 4,
    },
    {
      title: 'start should be equal to the pos for the duration in a matrix selector 2',
      expr: 'go[5d5]',
      pos: 6,
      expectedStart: 6,
    },
    {
      title: 'start should be equal to the pos for the duration in a matrix selector 3',
      expr: 'rate(my_metric{l1="l2"}[25])',
      pos: 26,
      expectedStart: 26,
    },
    {
      title: 'start should be equal to the pos for the duration in a matrix selector 4',
      expr: 'rate(my_metric{l1="l2"}[25d5])',
      pos: 28,
      expectedStart: 28,
    },
    {
      title: 'start should be equal to the pos for the duration in a subquery selector',
      expr: 'go[5d:5]',
      pos: 7,
      expectedStart: 7,
    },
    {
      title: 'start should be equal to the pos for the duration in a subquery selector 2',
      expr: 'go[5d:5d5]',
      pos: 9,
      expectedStart: 9,
    },
    {
      title: 'start should be equal to the pos for the duration in a subquery selector 3',
      expr: 'rate(my_metric{l1="l2"}[25d:6])',
      pos: 29,
      expectedStart: 29,
    },
    {
      title: 'start should be equal to the pos for the duration in a subquery selector 3',
      expr: 'rate(my_metric{l1="l2"}[25d:6d5])',
      pos: 31,
      expectedStart: 31,
    },
  ];
  testCases.forEach((value) => {
    it(value.title, () => {
      const state = createEditorState(value.expr);
      const node = syntaxTree(state).resolve(value.pos, -1);
      const result = computeStartCompletePosition(state, node, value.pos);
      expect(value.expectedStart).toEqual(result);
    });
  });
});

describe('autocomplete promQL test', () => {
  beforeEach(() => {
    mockPrometheusServer();
  });
  const testCases = [
    {
      title: 'offline empty expr',
      expr: '',
      pos: 0,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, numberTerms, snippets),
        from: 0,
        to: 0,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline simple function/aggregation/number autocompletion',
      expr: 'go_',
      pos: 3,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, numberTerms, snippets),
        from: 0,
        to: 3,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline function/aggregation autocompletion in aggregation',
      expr: 'sum()',
      pos: 4,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, snippets),
        from: 4,
        to: 4,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline function/aggregation autocompletion in aggregation 2',
      expr: 'sum(ra)',
      pos: 6,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, snippets),
        from: 4,
        to: 6,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline function/aggregation autocompletion in aggregation 3',
      expr: 'sum(rate())',
      pos: 9,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, snippets),
        from: 9,
        to: 9,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline function/aggregation autocompletion in aggregation 4',
      expr: 'sum by (instance, job) ( sum_over(scrape_series_added[1h])) / sum by (instance, job) (sum_over_time(scrape_samples_scraped[1h])) > 0.1 and sum by(instance, job) (scrape_samples_scraped{) > 100',
      pos: 33,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, snippets),
        from: 25,
        to: 33,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'autocomplete binOp modifier/metric/number',
      expr: 'metric_name / ignor',
      pos: 19,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, binOpModifierTerms, numberTerms, snippets),
        from: 14,
        to: 19,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'autocomplete binOp modifier/metric/number 2',
      expr: 'sum(http_requests_total{method="GET"} / o)',
      pos: 41,
      expectedResult: {
        options: ([] as Completion[]).concat(functionIdentifierTerms, aggregateOpTerms, binOpModifierTerms, numberTerms, snippets),
        from: 40,
        to: 41,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelName return nothing',
      expr: 'sum by ()',
      pos: 8, // cursor is between the bracket
      expectedResult: {
        options: [],
        from: 8,
        to: 8,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelName return nothing 2',
      expr: 'sum by (myL)',
      pos: 11, // cursor is between the bracket after the string myL
      expectedResult: {
        options: [],
        from: 8,
        to: 11,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelName return nothing 3',
      expr: 'sum by (myLabel1, myLab)',
      pos: 23, // cursor is between the bracket after the string myLab
      expectedResult: {
        options: [],
        from: 18,
        to: 23,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelName return nothing 4',
      expr: 'metric_name{}',
      pos: 12, // cursor is between the bracket
      expectedResult: {
        options: [],
        from: 12,
        to: 12,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelName return nothing 5',
      expr: '{}',
      pos: 1, // cursor is between the bracket
      expectedResult: {
        options: [],
        from: 1,
        to: 1,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelName return nothing 6',
      expr: 'metric_name{myL}',
      pos: 15, // cursor is between the bracket after the string myL
      expectedResult: {
        options: [],
        from: 12,
        to: 15,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelName return nothing 7',
      expr: '{myL}',
      pos: 4, // cursor is between the bracket after the string myL
      expectedResult: {
        options: [],
        from: 1,
        to: 4,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelValue return nothing',
      expr: 'metric_name{labelName=""}',
      pos: 23, // cursor is between the quotes
      expectedResult: {
        options: [],
        from: 23,
        to: 23,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete labelValue return nothing 2',
      expr: '{labelName=""}',
      pos: 12, // cursor is between the quotes
      expectedResult: {
        options: [],
        from: 12,
        to: 12,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete aggregate operation modifier or binary operator',
      expr: 'sum() b',
      pos: 7, // cursor is after 'b'
      expectedResult: {
        options: ([] as Completion[]).concat(aggregateOpModifierTerms, binOpTerms),
        from: 6,
        to: 7,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete aggregate operation modifier or binary operator 2',
      expr: 'sum(rate(foo[5m])) an',
      pos: 21, // cursor is after the string 'an'
      expectedResult: {
        options: ([] as Completion[]).concat(aggregateOpModifierTerms, binOpTerms),
        from: 19,
        to: 21,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete aggregate operation modifier or binary operator or offset',
      expr: 'sum b',
      pos: 5, // cursor is after 'b'
      expectedResult: {
        options: ([] as Completion[]).concat(aggregateOpModifierTerms, binOpTerms, [{ label: 'offset' }]),
        from: 4,
        to: 5,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete binOp',
      expr: 'metric_name !',
      pos: 13,
      expectedResult: {
        options: binOpTerms,
        from: 12,
        to: 13,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete binOp 2',
      expr: 'metric_name =',
      pos: 13,
      expectedResult: {
        options: binOpTerms,
        from: 12,
        to: 13,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete matchOp',
      expr: 'go{instance=""}',
      pos: 12, // cursor is after the 'equal'
      expectedResult: {
        options: matchOpTerms,
        from: 11,
        to: 12,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete matchOp 2',
      expr: 'metric_name{labelName!}',
      pos: 22, // cursor is after '!'
      expectedResult: {
        options: matchOpTerms,
        from: 21,
        to: 22,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete duration with offset',
      expr: 'http_requests_total offset 5',
      pos: 28,
      expectedResult: {
        options: durationTerms,
        from: 28,
        to: 28,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration with offset 2',
      expr: 'sum(http_requests_total{method="GET"} offset 4)',
      pos: 46,
      expectedResult: {
        options: durationTerms,
        from: 46,
        to: 46,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete offset or binOp',
      expr: 'http_requests_total off',
      pos: 23,
      expectedResult: {
        options: ([] as Completion[]).concat(binOpTerms, [{ label: 'offset' }]),
        from: 20,
        to: 23,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete offset or binOp 2',
      expr: 'metric_name unle',
      pos: 16,
      expectedResult: {
        options: ([] as Completion[]).concat(binOpTerms, [{ label: 'offset' }]),
        from: 12,
        to: 16,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete offset or binOp 3',
      expr: 'http_requests_total{method="GET"} off',
      pos: 37,
      expectedResult: {
        options: ([] as Completion[]).concat(binOpTerms, [{ label: 'offset' }]),
        from: 34,
        to: 37,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete offset or binOp 4',
      expr: 'rate(foo[5m]) un',
      pos: 16,
      expectedResult: {
        options: ([] as Completion[]).concat(binOpTerms, [{ label: 'offset' }]),
        from: 14,
        to: 16,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'autocomplete offset or binop 5',
      expr: 'sum(http_requests_total{method="GET"} off)',
      pos: 41,
      expectedResult: {
        options: ([] as Completion[]).concat(binOpTerms, [{ label: 'offset' }]),
        from: 38,
        to: 41,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline not autocompleting duration for a matrixSelector',
      expr: 'go[]',
      pos: 3,
      expectedResult: {
        options: [],
        from: 0,
        to: 3,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline not autocompleting duration for a matrixSelector 2',
      expr: 'go{l1="l2"}[]',
      pos: 12,
      expectedResult: {
        options: [],
        from: 0,
        to: 12,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'offline autocomplete duration for a matrixSelector',
      expr: 'go[5]',
      pos: 4,
      expectedResult: {
        options: durationTerms,
        from: 4,
        to: 4,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration for a matrixSelector 2',
      expr: 'go[5d1]',
      pos: 6,
      expectedResult: {
        options: durationTerms,
        from: 6,
        to: 6,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration for a matrixSelector 3',
      expr: 'rate(my_metric{l1="l2"}[25])',
      pos: 26,
      expectedResult: {
        options: durationTerms,
        from: 26,
        to: 26,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration for a matrixSelector 4',
      expr: 'rate(my_metric{l1="l2"}[25d5])',
      pos: 28,
      expectedResult: {
        options: durationTerms,
        from: 28,
        to: 28,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration for a subQuery',
      expr: 'go[5d:5]',
      pos: 7,
      expectedResult: {
        options: durationTerms,
        from: 7,
        to: 7,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration for a subQuery 2',
      expr: 'go[5d:5d4]',
      pos: 9,
      expectedResult: {
        options: durationTerms,
        from: 9,
        to: 9,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration for a subQuery 3',
      expr: 'rate(my_metric{l1="l2"}[25d:6])',
      pos: 29,
      expectedResult: {
        options: durationTerms,
        from: 29,
        to: 29,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete duration for a subQuery 4',
      expr: 'rate(my_metric{l1="l2"}[25d:6d5])',
      pos: 31,
      expectedResult: {
        options: durationTerms,
        from: 31,
        to: 31,
        validFor: undefined,
      },
    },
    {
      title: 'offline autocomplete at modifiers',
      expr: '1 @ s',
      pos: 5,
      expectedResult: {
        options: atModifierTerms,
        from: 4,
        to: 5,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'online autocomplete of metrics',
      expr: 'alert',
      pos: 5,
      conf: { remote: { url: 'http://localhost:8080' } },
      expectedResult: {
        options: ([] as Completion[]).concat(mockedMetricsTerms, functionIdentifierTerms, aggregateOpTerms, numberTerms, snippets),
        from: 0,
        to: 5,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'online autocomplete of label name corresponding to a metric',
      expr: 'alertmanager_alerts{}',
      pos: 20,
      conf: { remote: { url: 'http://localhost:8080' } },
      expectedResult: {
        options: [
          {
            label: 'env',
            type: 'constant',
          },
          {
            label: 'instance',
            type: 'constant',
          },
          {
            label: 'job',
            type: 'constant',
          },
          {
            label: 'state',
            type: 'constant',
          },
        ],
        from: 20,
        to: 20,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'online autocomplete of label value corresponding to a metric and a label name',
      expr: 'alertmanager_alerts{env=""}',
      pos: 25,
      conf: { remote: { url: 'http://localhost:8080' } },
      expectedResult: {
        options: [
          {
            label: 'demo',
            type: 'text',
          },
        ],
        from: 25,
        to: 25,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
    {
      title: 'online autocomplete with initial metric list',
      expr: 'rat',
      pos: 3,
      conf: { remote: { cache: { initialMetricList: ['metric1', 'metric2', 'rat'] } } },
      expectedResult: {
        options: ([] as Completion[]).concat(
          [
            {
              label: 'metric1',
              type: 'constant',
            },
            {
              label: 'metric2',
              type: 'constant',
            },
            {
              label: 'rat',
              type: 'constant',
            },
          ],
          functionIdentifierTerms,
          aggregateOpTerms,
          numberTerms,
          snippets
        ),
        from: 0,
        to: 3,
        validFor: /^[a-zA-Z0-9_:]+$/,
      },
    },
  ];
  testCases.forEach((value) => {
    it(value.title, async () => {
      const state = createEditorState(value.expr);
      const context = new CompletionContext(state, value.pos, true);
      const completion = newCompleteStrategy(value.conf);
      const result = await completion.promQL(context);
      expect(value.expectedResult).toEqual(result);
    });
  });
});
