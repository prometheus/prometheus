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

import {styleTags, tags} from "@lezer/highlight";

export const promQLHighLight = styleTags({
    LineComment: tags.comment,
    LabelName: tags.labelName,
    StringLiteral: tags.string,
    NumberDurationLiteral: tags.number,
    NumberDurationLiteralInDurationContext: tags.number,
    Identifier: tags.variableName,
    'Abs Absent AbsentOverTime Acos Acosh Asin Asinh Atan Atanh AvgOverTime Ceil Changes Clamp ClampMax ClampMin Cos Cosh CountOverTime DaysInMonth DayOfMonth DayOfWeek DayOfYear Deg Delta Deriv Exp Floor HistogramAvg HistogramCount HistogramFraction HistogramQuantile HistogramSum DoubleExponentialSmoothing Hour Idelta Increase Irate LabelReplace LabelJoin LastOverTime Ln Log10 Log2 MaxOverTime MinOverTime Minute Month Pi PredictLinear PresentOverTime QuantileOverTime Rad Rate Resets Round Scalar Sgn Sin Sinh Sort SortDesc SortByLabel SortByLabelDesc Sqrt StddevOverTime StdvarOverTime SumOverTime Tan Tanh Time Timestamp Vector Year':
        tags.function(tags.variableName),
    'Avg Bottomk Count Count_values Group LimitK LimitRatio Max Min Quantile Stddev Stdvar Sum Topk': tags.operatorKeyword,
    'By Without Bool On Ignoring GroupLeft GroupRight Offset Start End Smoothed Anchored': tags.modifier,
    'And Unless Or': tags.logicOperator,
    'Sub Add Mul Mod Div Atan2 Eql Neq Lte Lss Gte Gtr EqlRegex EqlSingle NeqRegex Pow At': tags.operator,
    UnaryOp: tags.arithmeticOperator,
    '( )': tags.paren,
    '[ ]': tags.squareBracket,
    '{ }': tags.brace,
    '⚠': tags.invalid,
})
