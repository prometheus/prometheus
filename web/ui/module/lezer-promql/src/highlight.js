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
    // Quoted names are highlighted like their unquoted counterparts: label names in groupings and matchers,
    // and metric names when the name is alone in the matchers.
    'GroupingLabels/QuotedLabelName/StringLiteral QuotedLabelMatcher/QuotedLabelName/StringLiteral': tags.labelName,
    'LabelMatchers/QuotedLabelName/StringLiteral': tags.variableName,
    NumberDurationLiteral: tags.number,
    NumberDurationLiteralInDurationContext: tags.number,
    Identifier: tags.variableName,
    'Abs Absent AbsentOverTime Acos Acosh Asin Asinh Atan Atanh AvgOverTime Ceil Changes Clamp ClampMax ClampMin Cos Cosh CountOverTime DaysInMonth DayOfMonth DayOfWeek DayOfYear Deg Delta Deriv EndFn Exp Floor HistogramAvg HistogramCount HistogramFraction HistogramQuantile HistogramSum DoubleExponentialSmoothing Hour Idelta Increase Integral Irate LabelReplace LabelJoin LastOverTime Ln Log10 Log2 MaxOverTime MinOverTime Minute Month Pi PredictLinear PresentOverTime QuantileOverTime Rad Range Rate Resets Round Scalar Sgn Sin Sinh Sort SortDesc SortByLabel SortByLabelDesc Sqrt StartFn Step StddevOverTime StdvarOverTime SumOverTime Tan Tanh Time Timestamp Vector Year':
        tags.function(tags.variableName),
    'Avg Bottomk Count Count_values Group LimitK LimitRatio Max Min Quantile Stddev Stdvar Sum Topk': tags.operatorKeyword,
    'AtModifierPreprocessors By Without Bool On Ignoring GroupLeft GroupRight Offset Smoothed Anchored': tags.modifier,
    'DurationExpr/DurationStep DurationExpr/DurationRange DurationExpr/DurationMinOf DurationExpr/DurationMaxOf OffsetDurationExpr/DurationStep OffsetDurationExpr/DurationRange OffsetDurationExpr/DurationMinOf OffsetDurationExpr/DurationMaxOf': tags.function(tags.variableName),
    'And Unless Or': tags.logicOperator,
    'Sub Add Mul Mod Div Atan2 Eql Neq Lte Lss Gte Gtr EqlRegex EqlSingle NeqRegex Pow At': tags.operator,
    UnaryOp: tags.arithmeticOperator,
    '( )': tags.paren,
    '[ ]': tags.squareBracket,
    '{ }': tags.brace,
    // The rest of a name that has to be quoted (e.g. the `bar` of `foo.bar`) ends up inside an error node.
    '⚠/Identifier ⚠/LabelName': tags.invalid,
    '⚠': tags.invalid,
})
