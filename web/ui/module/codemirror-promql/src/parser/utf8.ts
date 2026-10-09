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

// Helpers to deal with UTF-8 metric and label names, which have to be quoted in PromQL
// as soon as they are not valid legacy names.

const legacyMetricNameRegexp = /^[a-zA-Z_:][a-zA-Z0-9_:]*$/;
const legacyLabelNameRegexp = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

// Characters that can be part of an unquoted name candidate: the legacy name characters, dots, and any non-ASCII
// letter, mark, number, format character (e.g. joiners of emoji sequences) or symbol such as an emoji. Everything else (whitespace, quotes, brackets, operators, other punctuation) ends the candidate.
// The `u` flag is passed as a string to keep the ES5 compatible build working.
const nameCharRegexp = new RegExp('^[a-zA-Z0-9_:.\\p{L}\\p{M}\\p{N}\\p{Cf}\\p{So}]$', 'u');
// An unquoted name can only start with a letter, `_`, `:`, a mark or a symbol (a leading digit or dot means a number).
const nameStartRegexp = new RegExp('^[a-zA-Z_:\\p{L}\\p{M}\\p{So}]', 'u');
const nonLegacyCharRegexp = /[^a-zA-Z0-9_:]/;

// Reports whether the metric name cannot be written without quotes.
export function metricNameNeedsQuoting(name: string): boolean {
  return !legacyMetricNameRegexp.test(name);
}

// Reports whether the label name cannot be written without quotes.
export function labelNameNeedsQuoting(name: string): boolean {
  return !legacyLabelNameRegexp.test(name);
}

// Escapes the characters that cannot appear verbatim in a double-quoted PromQL string.
export function escapePromQLString(str: string): string {
  return str.replace(/[\\"\n\r\t]/g, (c) => {
    switch (c) {
      case '\n':
        return '\\n';
      case '\r':
        return '\\r';
      case '\t':
        return '\\t';
      default:
        return `\\${c}`;
    }
  });
}

// Returns the double-quoted PromQL string literal for str.
export function quotePromQLString(str: string): string {
  return `"${escapePromQLString(str)}"`;
}

const singleCharEscapes: Record<string, string> = {
  a: '\x07',
  b: '\b',
  f: '\f',
  n: '\n',
  r: '\r',
  t: '\t',
  v: '\v',
  '\\': '\\',
  "'": "'",
  '"': '"',
};

// Reports whether the string literal, given with its delimiters, has its closing delimiter.
// The lezer grammar also accepts an unterminated literal while the user is typing.
export function isTerminatedStringLiteral(literal: string): boolean {
  if (literal.length < 2 || !literal.endsWith(literal[0])) {
    return false;
  }
  if (literal[0] === '`') {
    return true;
  }
  // The last delimiter is escaped when it is preceded by an odd number of backslashes.
  let backslashes = 0;
  for (let i = literal.length - 2; i > 0 && literal[i] === '\\'; i--) {
    backslashes++;
  }
  return backslashes % 2 === 0;
}

// Matches an escape sequence, or a run of `\xHH` and octal escapes that denote the bytes of an UTF-8 sequence.
const escapeSequenceRegexp = /\\(?:([abfnrtv\\'"])|u([0-9a-fA-F]{4})|U([0-9a-fA-F]{8}))|((?:\\(?:x[0-9a-fA-F]{2}|[0-7]{3}))+)/g;
const byteEscapeRegexp = /\\(?:x([0-9a-fA-F]{2})|([0-7]{3}))/g;

// Returns the value of a PromQL string literal given with its delimiters.
// An unterminated literal is handled too. Unknown escape sequences are kept as they are.
export function unquotePromQLString(literal: string): string {
  if (literal.length === 0) {
    return '';
  }
  const body = isTerminatedStringLiteral(literal) ? literal.slice(1, -1) : literal.slice(1);
  if (literal[0] === '`') {
    return body;
  }
  return body.replace(escapeSequenceRegexp, (match, single, u, bigU, bytes) => {
    if (single) {
      return singleCharEscapes[single];
    }
    if (bytes) {
      const values = Array.from(bytes.matchAll(byteEscapeRegexp) as Iterable<RegExpMatchArray>, (m) => parseInt(m[1] ?? m[2], m[1] ? 16 : 8));
      if (values.some((v) => v > 0xff)) {
        return match;
      }
      return new TextDecoder().decode(Uint8Array.from(values));
    }
    const codePoint = parseInt(u ?? bigU, 16);
    return codePoint <= 0x10ffff ? String.fromCodePoint(codePoint) : match;
  });
}

// Encodes a label name for the path of the Prometheus HTTP API (`/api/v1/label/<name>/values`)
// using the values escaping scheme. Legacy names are returned untouched.
export function escapeLabelNameForAPI(name: string): string {
  if (!metricNameNeedsQuoting(name)) {
    return name;
  }
  let escaped = 'U__';
  for (const c of name) {
    if (/[a-zA-Z0-9:]/.test(c)) {
      escaped += c;
    } else if (c === '_') {
      escaped += '__';
    } else {
      escaped += `_${c.codePointAt(0)!.toString(16)}_`;
    }
  }
  return escaped;
}

// Returns the length in UTF-16 code units of the character that ends right before `index`.
function lengthBefore(text: string, index: number): number {
  const isLowSurrogate = index >= 2 && text.charCodeAt(index - 1) >= 0xdc00 && text.charCodeAt(index - 1) <= 0xdfff;
  const isHighSurrogate = isLowSurrogate && text.charCodeAt(index - 2) >= 0xd800 && text.charCodeAt(index - 2) <= 0xdbff;
  return isHighSurrogate ? 2 : 1;
}

// Looks for a name written without quotes around `offset` in `text` that is not a valid legacy name,
// like `http.requests` or `métrique`. These names only parse partially, and have to be quoted.
// It returns the range of the whole name, or null if there is none.
export function findUnquotedUtf8Name(text: string, offset: number): { from: number; to: number } | null {
  let from = offset;
  while (from > 0) {
    const length = lengthBefore(text, from);
    if (!nameCharRegexp.test(text.slice(from - length, from))) {
      break;
    }
    from -= length;
  }
  let to = offset;
  while (to < text.length) {
    const char = String.fromCodePoint(text.codePointAt(to) as number);
    if (!nameCharRegexp.test(char)) {
      break;
    }
    to += char.length;
  }
  const word = text.slice(from, to);
  if (!nameStartRegexp.test(word) || !nonLegacyCharRegexp.test(word)) {
    return null;
  }
  return { from, to };
}
