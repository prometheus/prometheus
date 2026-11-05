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

import { ExternalTokenizer } from "@lezer/lr";

import {
  Identifier,
  LabelName,
  And,
  Avg,
  Atan2,
  BitAnd,
  BitOr,
  BitXor,
  Bool,
  Bottomk,
  By,
  Count,
  CountValues,
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
  StartFn,
  EndFn,
  Stddev,
  Stdvar,
  Sum,
  Topk,
  Unless,
  Without,
  AtEnd,
  AtStart,
  Smoothed,
  Anchored,
  Fill,
  FillLeft,
  FillRight,
} from "./parser.terms.js";

// Name starts are Unicode letters or underscores; combining marks require quotes.
const nameStart = /[\p{L}_]/u;

// InputStream exposes UTF-16 code units; names may contain astral letters.
function readCodePoint(input) {
  const first = input.next;
  if (first >= 0xd800 && first <= 0xdbff) {
    const second = input.peek(1);
    if (second >= 0xdc00 && second <= 0xdfff) {
      return 0x10000 + ((first - 0xd800) << 10) + second - 0xdc00;
    }
  }
  return first;
}

function isNameStart(ch) {
  return ch >= 0 && nameStart.test(String.fromCodePoint(ch));
}

function isDigit(ch) {
  return ch >= 48 && ch <= 57;
}

// Keywords that can be directly followed by a number, matching the Go lexer.
const keywordsBeforeNumber = new Set(["bool", "offset", "and", "or", "unless", "atan2", "bitand", "bitor", "bitxor"]);
const longestKeywordBeforeNumber = Math.max(...[...keywordsBeforeNumber].map((kw) => kw.length));

export const names = new ExternalTokenizer(
  (input, stack) => {
    const label = stack.canShift(LabelName);
    let ch = readCodePoint(input);
    if (!isNameStart(ch) && (label || ch !== 58)) return;
    // The ASCII text read so far, or null once the name can't be one of keywordsBeforeNumber.
    let word = "";
    // Dots and ASCII digits can follow; only metric names allow colons.
    do {
      if (word !== null) {
        word = ch < 0x80 && word.length < longestKeywordBeforeNumber ? word + String.fromCharCode(ch) : null;
      }
      input.advance(ch > 0xffff ? 2 : 1);
      ch = readCodePoint(input);
      // Before unquoted names allowed dots, a dot ended a keyword, so a keyword could be
      // followed by a number such as .5, as in foo > bool.5. Keep lexing it that way.
      if (
        ch === 46 &&
        !label &&
        word !== null &&
        isDigit(input.peek(1)) &&
        keywordsBeforeNumber.has(word.toLowerCase())
      ) {
        break;
      }
    } while (isNameStart(ch) || isDigit(ch) || ch === 46 || (!label && ch === 58));
    input.acceptToken(label ? LabelName : Identifier);
  },
  { contextual: true }
);

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
  if (/[^\x00-\x7f]/.test(value)) return -1;
  return keywordTokens[value.toLowerCase()] || -1;
};

const contextualKeywordTokens = {
  avg: Avg,
  atan2: Atan2,
  bitand: BitAnd,
  bitor: BitOr,
  bitxor: BitXor,
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
  smoothed: Smoothed,
  anchored: Anchored,
  fill: Fill,
  fill_left: FillLeft,
  fill_right: FillRight,
};

export const extendIdentifier = (value, stack) => {
  // Unicode letters that lowercase to ASCII must remain names, not keywords.
  if (/[^\x00-\x7f]/.test(value)) return -1;
  if (value === "start" && stack.canShift(StartFn)) {
    return StartFn;
  }
  if (value === "end" && stack.canShift(EndFn)) {
    return EndFn;
  }
  if (value.toLowerCase() === "start" && stack.canShift(AtStart)) {
    return AtStart;
  }
  if (value.toLowerCase() === "end" && stack.canShift(AtEnd)) {
    return AtEnd;
  }

  const token = contextualKeywordTokens[value.toLowerCase()];
  if (token !== undefined && stack.canShift(token)) {
    return token;
  }

  return -1;
};
