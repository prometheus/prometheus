// The MIT License (MIT)
//
// Copyright (c) 2020 The Prometheus Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import { EqlRegex, EqlSingle, Neq, NeqRegex } from 'lezer-promql';
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
