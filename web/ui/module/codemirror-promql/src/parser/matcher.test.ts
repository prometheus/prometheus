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

import { EqlRegex, EqlSingle, Neq, NeqRegex } from '../grammar/parser.terms';
import { labelMatchersToString } from './matcher';
import { Matcher } from '../types';
import chai from 'chai';

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
      chai.expect(labelMatchersToString(value.metricName, value.matchers, value.labelName)).to.equal(value.result);
    });
  });
});
