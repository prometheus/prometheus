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
    'Abs Absent AbsentOverTime Acos Acosh Asin Asinh Atan Atanh AvgOverTime Ceil Changes Clamp ClampMax ClampMin Cos Cosh CountOverTime DaysInMonth DayOfMonth DayOfWeek DayOfYear Deg Delta Deriv EndFn Exp FirstOverTime Floor HistogramAvg HistogramCount HistogramFraction HistogramQuantile HistogramQuantiles HistogramStdDev HistogramStdVar HistogramSum DoubleExponentialSmoothing Hour Idelta Increase Info Integral Irate LabelReplace LabelJoin LastOverTime Ln Log10 Log2 MadOverTime MaxOverTime MinOverTime Minute Month Pi PredictLinear PresentOverTime QuantileOverTime Rad Range Rate Resets Round Scalar Sgn Sin Sinh Sort SortDesc SortByLabel SortByLabelDesc Sqrt StartFn StartTimestamp Step StddevOverTime StdvarOverTime SumOverTime Tan Tanh Time Timestamp TsOfFirstOverTime TsOfMaxOverTime TsOfMinOverTime TsOfLastOverTime Vector Year':
        tags.function(tags.variableName),
    'Avg Bottomk Count CountValues Group LimitK LimitRatio Max Min Quantile Stddev Stdvar Sum Topk': tags.operatorKeyword,
    'AtModifierPreprocessors By Without Bool On Ignoring GroupLeft GroupRight Fill FillLeft FillRight Offset Smoothed Anchored': tags.modifier,
    'DurationExpr/DurationStep DurationExpr/DurationRange DurationExpr/DurationMinOf DurationExpr/DurationMaxOf OffsetDurationExpr/DurationStep OffsetDurationExpr/DurationRange OffsetDurationExpr/DurationMinOf OffsetDurationExpr/DurationMaxOf': tags.function(tags.variableName),
    'And Unless Or': tags.logicOperator,
    'Sub Add Mul Mod Div Atan2 BitAnd BitOr BitXor Eql Neq Lte Lss Gte Gtr TrimUpper TrimLower EqlRegex EqlSingle NeqRegex Pow At': tags.operator,
    UnaryOp: tags.arithmeticOperator,
    '( )': tags.paren,
    '[ ]': tags.squareBracket,
    '{ }': tags.brace,
    '⚠': tags.invalid,
})
