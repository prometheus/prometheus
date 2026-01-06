// Copyright The Prometheus Authors
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
    And,
    Avg,
    Atan2,
    Bool,
    Bottomk,
    By,
    Count,
    CountValues,
    End,
    Group,
    GroupLeft,
    GroupRight,
    Ignoring,
    inf,
    Max,
    Min,
    nan,
    Offset,
    On,
    Or,
    Quantile,
    LimitK,
    LimitRatio,
    Start,
    Stddev,
    Stdvar,
    Sum,
    Topk,
    Unless,
    Without,
    Smoothed,
    Anchored,
} from './parser.terms.js';

const keywordTokens = {
    inf: inf,
    nan: nan,
    bool: Bool,
    ignoring: Ignoring,
    on: On,
    group_left: GroupLeft,
    group_right: GroupRight,
    offset: Offset,
};

export const specializeIdentifier = (value, stack) => {
    return keywordTokens[value.toLowerCase()] || -1;
};

const contextualKeywordTokens = {
    avg: Avg,
    atan2: Atan2,
    bottomk: Bottomk,
    count: Count,
    count_values: CountValues,
    group: Group,
    max: Max,
    min: Min,
    quantile: Quantile,
    limitk: LimitK,
    limit_ratio: LimitRatio,
    stddev: Stddev,
    stdvar: Stdvar,
    sum: Sum,
    topk: Topk,
    by: By,
    without: Without,
    and: And,
    or: Or,
    unless: Unless,
    start: Start,
    end: End,
    smoothed: Smoothed,
    anchored: Anchored,
};

export const extendIdentifier = (value, stack) => {
    return contextualKeywordTokens[value.toLowerCase()] || -1;
};
