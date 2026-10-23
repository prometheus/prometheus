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

import { CompleteStrategy } from './index';
import { SyntaxNode } from '@lezer/common';
import { PrometheusClient } from '../client';
import {
  Add,
  AggregateExpr,
  AggregateModifier,
  And,
  BinaryExpr,
  BoolModifier,
  Div,
  Eql,
  EqlRegex,
  EqlSingle,
  FunctionCallBody,
  GroupingLabels,
  Gte,
  Gtr,
  LabelMatchers,
  LabelName,
  Lss,
  Lte,
  MatchOp,
  MatrixSelector,
  Identifier,
  Mod,
  Mul,
  Neq,
  NeqRegex,
  OffsetExpr,
  Or,
  Pow,
  PromQL,
  StepInvariantExpr,
  StringLiteral,
  Sub,
  SubqueryExpr,
  Unless,
  VectorSelector,
  UnquotedLabelMatcher,
  QuotedLabelMatcher,
  QuotedLabelName,
  NumberDurationLiteralInDurationContext,
  NumberDurationLiteral,
  DurationExpr,
  AggregateOp,
  Topk,
  Bottomk,
  LimitK,
  LimitRatio,
  CountValues,
  TrimLower,
  TrimUpper,
} from '@prometheus-io/lezer-promql';
import { Completion, CompletionContext, CompletionResult } from '@codemirror/autocomplete';
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import {
  buildLabelMatchers,
  containsAtLeastOneChild,
  containsChild,
  escapePromQLString,
  findUnquotedUtf8Name,
  isTerminatedStringLiteral,
  labelNameNeedsQuoting,
  metricNameNeedsQuoting,
  quotePromQLString,
  unquotePromQLString,
  walkBackward,
} from '../parser';
import {
  aggregateOpModifierTerms,
  aggregateOpTerms,
  atModifierTerms,
  binOpModifierTerms,
  binOpTerms,
  durationTerms,
  durationExprTerms,
  durationExprOperatorTerms,
  functionIdentifierTerms,
  matchOpTerms,
  numberTerms,
  snippets,
} from './promql.terms';
import { Matcher } from '../types';
import { syntaxTree } from '@codemirror/language';

const autocompleteNodes: { [key: string]: Completion[] } = {
  matchOp: matchOpTerms,
  binOp: binOpTerms,
  duration: durationTerms,
  durationExpr: durationExprTerms,
  durationExprOperator: durationExprOperatorTerms,
  binOpModifier: binOpModifierTerms,
  atModifier: atModifierTerms,
  functionIdentifier: functionIdentifierTerms,
  aggregateOp: aggregateOpTerms,
  aggregateOpModifier: aggregateOpModifierTerms,
  number: numberTerms,
};

// ContextKind is the different possible value determinate by the autocompletion
export enum ContextKind {
  // dynamic autocompletion (required a distant server)
  MetricName,
  LabelName,
  LabelValue,
  // static autocompletion
  Function,
  Aggregation,
  BinOpModifier,
  BinOp,
  MatchOp,
  AggregateOpModifier,
  Duration,
  DurationExpr,
  DurationExprOperator,
  Offset,
  Bool,
  AtModifiers,
  Number,
}

export interface Context {
  kind: ContextKind;
  metricName?: string;
  labelName?: string;
  matchers?: Matcher[];
}

function getMetricNameInGroupBy(tree: SyntaxNode, state: EditorState): string {
  // There should be an AggregateExpr as parent of the GroupingLabels.
  // Then we should find the VectorSelector child to be able to find the metric name.
  const currentNode: SyntaxNode | null = walkBackward(tree, AggregateExpr);
  if (!currentNode) {
    return '';
  }
  let metricName = '';
  currentNode.cursor().iterate((node) => {
    // Continue until we find the VectorSelector, then look up the metric name.
    if (node.type.id === VectorSelector) {
      metricName = getMetricNameInVectorSelector(node.node, state);
      if (metricName) {
        return false;
      }
    }
  });
  return metricName;
}

function getMetricNameInVectorSelector(tree: SyntaxNode, state: EditorState): string {
  // Find if there is a defined metric name. Should be used to autocomplete a labelValue or a labelName
  // First find the parent "VectorSelector" to be able to find then the subChild "Identifier" if it exists.
  const currentNode: SyntaxNode | null = walkBackward(tree, VectorSelector);
  if (!currentNode) {
    // Weird case that shouldn't happen, because "VectorSelector" is by definition the parent of the LabelMatchers.
    return '';
  }
  const identifier = currentNode.getChild(Identifier);
  if (identifier) {
    // A name that has to be quoted but is typed without quotes only has its first characters in the Identifier.
    const line = state.doc.lineAt(identifier.from);
    const unquotedName = findUnquotedUtf8Name(line.text, identifier.from - line.from);
    if (unquotedName && line.from + unquotedName.from === identifier.from) {
      return line.text.slice(unquotedName.from, unquotedName.to);
    }
    return state.sliceDoc(identifier.from, identifier.to);
  }
  // The metric name can also be quoted inside the braces: `{"metric.name", foo="bar"}`.
  // The quoted name being currently completed is not a metric name to rely on.
  for (let child = currentNode.getChild(LabelMatchers)?.firstChild ?? null; child; child = child.nextSibling) {
    if (child.type.id === QuotedLabelName && !(child.from <= tree.from && tree.to <= child.to)) {
      return unquotePromQLString(state.sliceDoc(child.from, child.to));
    }
  }
  return '';
}

function arrayToCompletionResult(data: Completion[], from: number, to: number, includeSnippet = false, span = true): CompletionResult {
  const options = dedupeCompletions(data);
  if (includeSnippet) {
    // Snippets are appended after deduplication; if a snippet label ever matched a
    // deduped option, both could appear until dedupe is extended to cover snippets.
    options.push(...snippets);
  }
  return {
    from: from,
    to: to,
    options: options,
    validFor: span ? /^[a-zA-Z0-9_:]+$/ : undefined,
  } as CompletionResult;
}

// Replaces the string literal surrounding the completion range [from, to) with the given quoted name.
function replaceQuotedString(quotedName: string): (view: EditorView, completion: Completion, from: number, to: number) => void {
  return (view, _completion, from, to) => {
    const delimiter = view.state.sliceDoc(from - 1, from);
    const end = view.state.sliceDoc(to, to + 1) === delimiter ? to + 1 : to;
    view.dispatch({
      changes: { from: from - 1, to: end, insert: quotedName },
      selection: { anchor: from - 1 + quotedName.length },
      userEvent: 'input.complete',
    });
  };
}

// Returns how the label name has to be inserted, or undefined when inserting its label is enough.
// `inString` tells whether the name is inserted inside a quoted string, in which case the completion range
// is the content of the string, and the quotes around it are replaced by the completion.
function applyLabelName(name: string, inString: boolean): Completion['apply'] {
  if (inString) {
    return replaceQuotedString(quotePromQLString(name));
  }
  return labelNameNeedsQuoting(name) ? quotePromQLString(name) : undefined;
}

// Returns how the metric name has to be inserted, or undefined when inserting its label is enough.
// A metric name that requires quotes has to be written inside the braces of the selector, next to the other matchers:
// `foo{a="b"}` becomes `{"foo.bar", a="b"}`.
// The document is inspected when the completion is applied, because it can have changed since the completion was computed.
function applyMetricName(name: string, inString: boolean): Completion['apply'] {
  const quotedName = quotePromQLString(name);
  if (inString) {
    return replaceQuotedString(quotedName);
  }
  if (!metricNameNeedsQuoting(name)) {
    return undefined;
  }
  return (view, _completion, from, to) => {
    // Matchers right after the name, or after the position of the completion when there is no name yet.
    const braces = /^\s*\{(\s*)(\}?)/.exec(view.state.sliceDoc(to, to + 1000));
    if (!braces) {
      const selector = `{${quotedName}}`;
      view.dispatch({
        changes: { from, to, insert: selector },
        selection: { anchor: from + selector.length - 1 },
        userEvent: 'input.complete',
      });
      return;
    }
    const open = to + braces[0].indexOf('{');
    const hasMatchers = braces[2] === '' && view.state.sliceDoc(open + 1 + braces[1].length, open + 2 + braces[1].length) !== '';
    view.dispatch({
      changes: [
        { from, to, insert: '' },
        { from: open + 1, insert: hasMatchers ? `${quotedName},${braces[1] === '' ? ' ' : ''}` : quotedName },
      ],
      selection: { anchor: open + 1 - (to - from) + quotedName.length },
      userEvent: 'input.complete',
    });
  };
}

// Returns the contexts to complete a name that starts with the given error node, which is a name that is not
// a legacy name, and that starts with a non-ASCII character.
function analyzeErrorNodeAsName(state: EditorState, node: SyntaxNode): Context[] {
  for (let ancestor = node.parent; ancestor; ancestor = ancestor.parent) {
    switch (ancestor.type.id) {
      case GroupingLabels:
        return [{ kind: ContextKind.LabelName, metricName: getMetricNameInGroupBy(ancestor, state) }];
      case LabelMatchers:
        return [{ kind: ContextKind.LabelName, metricName: getMetricNameInVectorSelector(ancestor, state) }];
      case MatrixSelector:
      case SubqueryExpr:
      case OffsetExpr:
      case DurationExpr:
        // A duration is expected here, not a name.
        return [];
    }
  }
  return [{ kind: ContextKind.MetricName, metricName: '' }];
}

function isAfterClosedFunctionCallBody(state: EditorState, node: SyntaxNode, pos: number): boolean {
  return node.type.id === FunctionCallBody && pos >= node.to && node.from < node.to && state.sliceDoc(node.to - 1, node.to) === ')';
}

function dedupeCompletions(data: Completion[]): Completion[] {
  const seen = new Set<string>();
  const deduped: Completion[] = [];
  for (const completion of data) {
    const infoKey = typeof completion.info === 'string' ? completion.info : '';
    // Include `apply` in the key when it is a string (e.g. snippet insert text) so that two
    // completions sharing the same label/type/info but inserting different text are not merged.
    // Function `apply` values cannot be compared meaningfully, so they fall back to an empty key.
    const applyKey = typeof completion.apply === 'string' ? completion.apply : '';
    const key = `${completion.label}|${completion.type ?? ''}|${infoKey}|${applyKey}`;
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    deduped.push(completion);
  }
  return deduped;
}

// computeEndCompletePosition calculates the end position for autocompletion replacement.
// When the cursor is in the middle of a token, this ensures the entire token is replaced,
// not just the portion before the cursor. This fixes issue #15839.
// Note: this method is exported only for testing purpose.
export function computeEndCompletePosition(state: EditorState, node: SyntaxNode, pos: number): number {
  // For error nodes, use the cursor position as the end position
  if (node.type.id === 0) {
    return pos;
  }

  if (isAfterClosedFunctionCallBody(state, node, pos)) {
    return pos;
  }

  if (
    node.type.id === LabelMatchers ||
    node.type.id === GroupingLabels ||
    node.type.id === FunctionCallBody ||
    node.type.id === MatrixSelector ||
    node.type.id === SubqueryExpr
  ) {
    // When we're inside empty brackets, we want to replace up to just before the closing bracket.
    return node.to - 1;
  }

  if (node.type.id === StringLiteral && (node.parent?.type.id === UnquotedLabelMatcher || node.parent?.type.id === QuotedLabelMatcher)) {
    // For label values, we want to replace all content inside the quotes.
    return node.parent.to - 1;
  }

  if (node.type.id === StringLiteral && node.parent?.type.id === QuotedLabelName) {
    // For quoted names, we want to replace all content inside the quotes.
    // A string that is missing its closing quote runs until the end of the line, which is not part of the name:
    // only the content before the cursor is replaced.
    return isTerminatedStringLiteral(state.sliceDoc(node.from, node.to)) ? node.to - 1 : pos;
  }

  // For all other nodes, extend the end position to include the entire token.
  return node.to;
}

// Matches complete PromQL durations, including compound units (e.g., 5m, 1d2h, 1h30m, etc.).
// Duration units are a fixed, safe set (no regex metacharacters), so no escaping is needed.
export const durationWithUnitRegexp = new RegExp(`^(\\d+(${durationTerms.map((term) => term.label).join('|')}))+$`);

// Determines if a duration already has a complete time unit to prevent autocomplete insertion (issue #15452)
function hasCompleteDurationUnit(state: EditorState, node: SyntaxNode): boolean {
  if (node.from >= node.to) {
    return false;
  }
  const nodeContent = state.sliceDoc(node.from, node.to);
  return durationWithUnitRegexp.test(nodeContent);
}

// computeStartCompleteLabelPositionInLabelMatcherOrInGroupingLabel calculates the start position only when the node is a LabelMatchers or a GroupingLabels
function computeStartCompleteLabelPositionInLabelMatcherOrInGroupingLabel(node: SyntaxNode, pos: number): number {
  // Here we can have two different situations:
  // 1. `metric{}` or `sum by()` with the cursor between the bracket
  // and so we have increment the starting position to avoid to consider the open bracket when filtering the autocompletion list.
  // 2. `metric{foo="bar",} or `sum by(foo,)  with the cursor after the comma.
  // Then the start number should be the current position to avoid to consider the previous labelMatcher/groupingLabel when filtering the autocompletion list.
  let start = node.from + 1;
  if (node.firstChild !== null) {
    // here that means the LabelMatchers / GroupingLabels has a child, which is not possible if we have the expression `metric{}`. So we are likely trying to autocomplete the label list after a comma
    start = pos;
  }
  return start;
}

// computeStartCompletePosition calculates the start position of the autocompletion.
// It is an important step because the start position will be used by CMN to find the string and then to use it to filter the CompletionResult.
// A wrong `start` position will lead to have the completion not working.
// Note: this method is exported only for testing purpose.
export function computeStartCompletePosition(state: EditorState, node: SyntaxNode, pos: number): number {
  const currentText = state.doc.slice(node.from, pos).toString();
  let start = node.from;
  if (isAfterClosedFunctionCallBody(state, node, pos)) {
    start = pos;
  } else if (node.type.id === LabelMatchers || node.type.id === GroupingLabels) {
    start = computeStartCompleteLabelPositionInLabelMatcherOrInGroupingLabel(node, pos);
  } else if (
    (node.type.id === FunctionCallBody && node.firstChild === null) ||
    (node.type.id === StringLiteral &&
      (node.parent?.type.id === UnquotedLabelMatcher || node.parent?.type.id === QuotedLabelMatcher || node.parent?.type.id === QuotedLabelName))
  ) {
    // When the cursor is between bracket, quote, we need to increment the starting position to avoid to consider the open bracket/ first string.
    start++;
  } else if (
    // MatrixSelector/SubqueryExpr are safe here: this branch is only reached when
    // `resolve()` returns those bracket nodes directly, i.e. cursor is in duration
    // slots where replacing from `pos` avoids clobbering the selector expression.
    node.type.id === MatrixSelector ||
    node.type.id === SubqueryExpr ||
    node.type.id === OffsetExpr ||
    // Since duration and number are equivalent, writing go[5] or go[5d] is syntactically accurate.
    // Before we were able to guess when we had to autocomplete the duration later based on the error node,
    // which is not possible anymore.
    // So we have to analyze the string about the current node to see if the duration unit is already present or not.
    (node.type.id === NumberDurationLiteralInDurationContext && !durationTerms.map((v) => v.label).includes(currentText[currentText.length - 1])) ||
    (node.type.id === NumberDurationLiteral && node.parent?.type.id === 0 && node.parent.parent?.type.id === SubqueryExpr) ||
    (node.type.id === FunctionCallBody && isAggregatorWithParam(node) && node.firstChild !== null) ||
    (node.type.id === 0 &&
      (node.parent?.type.id === OffsetExpr ||
        node.parent?.type.id === MatrixSelector ||
        (node.parent?.type.id === SubqueryExpr && node.parent.getChild(DurationExpr) !== null)))
  ) {
    start = pos;
  }
  return start;
}

function isAggregatorWithParam(functionCallBody: SyntaxNode): boolean {
  const parent = functionCallBody.parent;
  if (parent !== null && parent.firstChild?.type.id === AggregateOp) {
    const aggregationOpType = parent.firstChild.firstChild;
    if (aggregationOpType !== null && [Topk, Bottomk, LimitK, LimitRatio, CountValues].includes(aggregationOpType.type.id)) {
      return true;
    }
  }
  return false;
}

// analyzeCompletion is going to determinate what should be autocompleted.
// The value of the autocompletion is then calculate by the function buildCompletion.
// `explicit` should reflect whether the completion was explicitly requested by the user (e.g. via Ctrl+Space)
// rather than triggered automatically while typing; some contexts are only relevant in the former case.
// Note: this method is exported for testing purpose only. Do not use it directly.
export function analyzeCompletion(state: EditorState, node: SyntaxNode, pos: number, explicit = true): Context[] {
  const result: Context[] = [];
  switch (node.type.id) {
    case 0: {
      // 0 is the id of the error node
      if (
        node.parent?.type.id === OffsetExpr ||
        node.parent?.type.id === MatrixSelector ||
        (node.parent?.type.id === SubqueryExpr && node.parent.getChild(DurationExpr) !== null)
      ) {
        // We are in a duration slot. Two situations land here with an error node:
        //   1. `go[]`  -> the error node text is empty: nothing typed yet, so offer
        //      no suggestions (units appear once the user starts a duration, and
        //      functions appear once the user types a letter via the Identifier/LabelName handler).
        //   2. `go[5d1]` or `go[5d:5d4]` -> a dangling digit follows a complete duration,
        //      so the error node text is a non-empty number. The user is building a
        //      compound duration (e.g. `5d1h`), so we keep offering duration units.
        const errorText = state.sliceDoc(node.from, node.to);
        if (errorText.length > 0) {
          // TODO: Ideally we should restrict the offered units to those strictly smaller than
          // the last unit already typed, because compound durations must be written in strictly
          // descending unit order (y w d h m s ms). For example, after `5d` only `h/m/s/ms` are
          // valid, so suggesting `5d1d` or `5d1y` is wrong. For now we over-offer the full set.
          result.push({ kind: ContextKind.Duration });
        }
        break;
      }
      if (node.parent?.type.id === UnquotedLabelMatcher || node.parent?.type.id === QuotedLabelMatcher) {
        // In this case the current token is not itself a valid match op yet:
        //      metric_name{labelName!}
        result.push({ kind: ContextKind.MatchOp });
        break;
      }
      // when we are in the situation 'metric_name !', we have the following tree
      // VectorSelector(Identifier,⚠)
      // We should try to know if the char '!' is part of a binOp.
      // Note: as it is quite experimental, maybe it requires more condition and to check the current tree (parent, other child at the same level ..etc.).
      const operator = state.sliceDoc(node.from, node.to);
      if (binOpTerms.filter((term) => term.label.includes(operator)).length > 0) {
        result.push({ kind: ContextKind.BinOp });
      }
      break;
    }
    case Identifier: {
      // sometimes an Identifier has an error has parent. This should be treated in priority
      if (node.parent?.type.id === 0) {
        const errorNodeParent = node.parent.parent;
        if (errorNodeParent?.type.id === StepInvariantExpr) {
          // we are likely in the given situation:
          //   `expr @ s`
          // we can autocomplete start / end
          result.push({ kind: ContextKind.AtModifiers });
          break;
        }
        if (errorNodeParent?.type.id === AggregateExpr) {
          // it matches 'sum() b'. So here we can autocomplete:
          // - the aggregate operation modifier
          // - the binary operation (since it's not mandatory to have an aggregate operation modifier)
          result.push({ kind: ContextKind.AggregateOpModifier }, { kind: ContextKind.BinOp });
          break;
        }
        if (errorNodeParent?.type.id === VectorSelector) {
          // it matches 'sum b'. So here we also have to autocomplete the aggregate operation modifier only
          // if the associated identifier is matching an aggregation operation.
          // Note: here is the corresponding tree in order to understand the situation:
          // VectorSelector(
          //   Identifier,
          //   ⚠(Identifier)
          // )
          const operator = getMetricNameInVectorSelector(node, state);
          if (aggregateOpTerms.filter((term) => term.label === operator).length > 0) {
            result.push({ kind: ContextKind.AggregateOpModifier });
          }
          // It's possible it also match the expr 'metric_name unle'.
          // It's also possible that the operator is also a metric even if it matches the list of aggregation function.
          // So we also have to autocomplete the binary operator.
          //
          // The expr `metric_name off` leads to the same tree. So we have to provide the offset keyword too here.
          result.push({ kind: ContextKind.BinOp }, { kind: ContextKind.Offset });
          break;
        }

        if (
          errorNodeParent?.type.id === MatrixSelector ||
          errorNodeParent?.type.id === OffsetExpr ||
          errorNodeParent?.type.id === SubqueryExpr ||
          errorNodeParent?.type.id === DurationExpr
        ) {
          // Identifier typed in a duration slot or inside a DurationExpr arithmetic expression
          // (e.g. `foo[ste]`, `foo offset ste`, `go[5d:ste]`, `foo[5m+2ms+m`).
          // Offer duration-expression functions so the user can complete `step()`, `range()`, etc.
          result.push({ kind: ContextKind.DurationExpr });
          break;
        }

        if (errorNodeParent && containsChild(errorNodeParent, 'Expr')) {
          // this last case can appear with the following expression:
          // 1. http_requests_total{method="GET"} off
          // 2. rate(foo[5m]) un
          // 3. sum(http_requests_total{method="GET"} off)
          // For these different cases we have this kind of tree:
          // Parent (
          //    ⚠(Identifier)
          // )
          // We don't really care about the parent, here we are more interested if in the siblings of the error node, there is the node 'Expr'
          // If it is the case, then likely we should autocomplete the BinOp or the offset.
          result.push({ kind: ContextKind.BinOp }, { kind: ContextKind.Offset });
          break;
        }
      }
      // As the leaf Identifier is coming for different cases, we have to take a bit time to analyze the tree
      // in order to know what we have to autocomplete exactly.
      // Here is some cases:
      // 1. metric_name / ignor --> we should autocomplete the BinOpModifier + metric/function/aggregation
      // 2. sum(http_requests_total{method="GET"} / o) --> BinOpModifier + metric/function/aggregation
      // Examples above give a different tree each time and ends up to be treated in this case.
      // But they all have the following common tree pattern:
      // Parent( ...,
      //         ... ,
      //         VectorSelector(Identifier)
      //       )
      //
      // So the first things to do is to get the `Parent` and to determinate if we are in this configuration.
      // Otherwise we would just have to autocomplete the metric / function / aggregation.

      const parent = node.parent?.parent;
      if (!parent) {
        // this case can be possible if the topNode is not anymore PromQL but MetricName.
        // In this particular case, then we just want to autocomplete the metric
        result.push({ kind: ContextKind.MetricName, metricName: state.sliceDoc(node.from, node.to) });
        break;
      }
      // now we have to know if we have two Expr in the direct children of the `parent`
      const containExprTwice = containsChild(parent, 'Expr', 'Expr');
      if (containExprTwice && parent.type.id !== FunctionCallBody) {
        if (parent.type.id === BinaryExpr && !containsAtLeastOneChild(parent, 0)) {
          // We are likely in the case 1 or 5
          result.push(
            { kind: ContextKind.MetricName, metricName: state.sliceDoc(node.from, node.to) },
            { kind: ContextKind.Function },
            { kind: ContextKind.Aggregation },
            { kind: ContextKind.BinOpModifier },
            { kind: ContextKind.Number }
          );
          // in  case the BinaryExpr is a comparison, we should autocomplete the `bool` keyword. But only if it is not present.
          // When the `bool` keyword is NOT present, then the expression looks like this:
          // 			BinaryExpr( ..., Gtr , ... )
          // When the `bool` keyword is present, then the expression looks like this:
          //      BinaryExpr( ..., Gtr , BoolModifier(...), ... )
          if (containsAtLeastOneChild(parent, Eql, Gte, Gtr, Lte, Lss, Neq) && !containsAtLeastOneChild(parent, BoolModifier)) {
            result.push({ kind: ContextKind.Bool });
          }
        }
      } else {
        result.push(
          { kind: ContextKind.MetricName, metricName: state.sliceDoc(node.from, node.to) },
          { kind: ContextKind.Function },
          { kind: ContextKind.Aggregation }
        );
        if (parent.type.id !== FunctionCallBody && parent.type.id !== MatrixSelector) {
          // it's to avoid to autocomplete a number in situation where it shouldn't.
          // Like with `sum by(rat)`
          result.push({ kind: ContextKind.Number });
        }
      }
      break;
    }
    case PromQL:
      if (node.firstChild !== null && node.firstChild.type.id === 0) {
        // this situation can happen when there is nothing in the text area and the user is explicitly triggering the autocompletion (with ctrl + space)
        result.push(
          { kind: ContextKind.MetricName, metricName: '' },
          { kind: ContextKind.Function },
          { kind: ContextKind.Aggregation },
          { kind: ContextKind.Number }
        );
      }
      break;
    case GroupingLabels:
      // In this case we are in the given situation:
      //      sum by () or sum (metric_name) by ()
      // so we have or to autocomplete any kind of labelName or to autocomplete only the labelName associated to the metric
      result.push({ kind: ContextKind.LabelName, metricName: getMetricNameInGroupBy(node, state) });
      break;
    case LabelMatchers: {
      if (pos >= node.to) {
        // Cursor is outside of the label matcher block (e.g. right after `}`),
        // so don't offer label-related completions anymore.
        break;
      }
      // In that case we are in the given situation:
      //       metric_name{} or {}
      // so we have or to autocomplete any kind of labelName or to autocomplete only the labelName associated to the metric
      result.push({ kind: ContextKind.LabelName, metricName: getMetricNameInVectorSelector(node, state) });
      break;
    }
    case LabelName:
      if (node.parent?.type.id === GroupingLabels) {
        // In this case we are in the given situation:
        //      sum by (myL)
        // So we have to continue to autocomplete any kind of labelName
        result.push({ kind: ContextKind.LabelName });
      } else if (node.parent?.type.id === UnquotedLabelMatcher) {
        // In that case we are in the given situation:
        //       metric_name{myL} or {myL}
        // so we have or to continue to autocomplete any kind of labelName or
        // to continue to autocomplete only the labelName associated to the metric
        result.push({ kind: ContextKind.LabelName, metricName: getMetricNameInVectorSelector(node, state) });
      } else if (node.parent?.type.id === 0) {
        const grandParent = node.parent.parent;
        if (grandParent?.type.id === MatrixSelector || grandParent?.type.id === OffsetExpr || grandParent?.type.id === SubqueryExpr) {
          // LabelName typed after a complete duration in a duration slot (e.g. `foo[5mss]`).
          // Offer duration-expression functions so the user can complete `step()`, `range()`, etc.
          result.push({ kind: ContextKind.DurationExpr });
        }
      }
      break;
    case StringLiteral:
      if (node.parent?.type.id === UnquotedLabelMatcher || node.parent?.type.id === QuotedLabelMatcher) {
        // In this case we are in the given situation:
        //      metric_name{labelName=""} or metric_name{"labelName"=""}
        // So we can autocomplete the labelValue

        // Get the labelName.
        // By definition it's the firstChild: https://github.com/promlabs/lezer-promql/blob/0ef65e196a8db6a989ff3877d57fd0447d70e971/src/promql.grammar#L250
        let labelName = '';
        if (node.parent.firstChild?.type.id === LabelName) {
          labelName = state.sliceDoc(node.parent.firstChild.from, node.parent.firstChild.to);
        } else if (node.parent.firstChild?.type.id === QuotedLabelName) {
          labelName = unquotePromQLString(state.sliceDoc(node.parent.firstChild.from, node.parent.firstChild.to));
        }
        // then find the metricName if it exists
        const metricName = getMetricNameInVectorSelector(node, state);
        // finally get the full matcher available
        const matcherNode = walkBackward(node, LabelMatchers);
        const labelMatcherOpts = [QuotedLabelName, QuotedLabelMatcher, UnquotedLabelMatcher];
        let labelMatchers: Matcher[] = [];
        for (const labelMatcherOpt of labelMatcherOpts) {
          labelMatchers = labelMatchers.concat(buildLabelMatchers(matcherNode ? matcherNode.getChildren(labelMatcherOpt) : [], state));
        }
        result.push({
          kind: ContextKind.LabelValue,
          metricName: metricName,
          labelName: labelName,
          matchers: labelMatchers,
        });
      } else if (node.parent?.parent?.type.id === GroupingLabels) {
        // In this case we are in the given situation:
        //      sum by ("myL")
        // So we have to continue to autocomplete any kind of labelName
        result.push({ kind: ContextKind.LabelName });
      } else if (node.parent?.parent?.type.id === LabelMatchers) {
        // In that case we are in the given situation:
        //       {""} or {"metric_"}
        // The quoted string is either a QuotedMetricName or the name of a label that is going to be matched,
        // so we need to continue to autocomplete for the metric names and the label names.
        // A metric name is only possible when the selector doesn't already have one.
        const selector = walkBackward(node, VectorSelector);
        const hasMetricName =
          selector?.getChild(Identifier) != null ||
          selector
            ?.getChild(LabelMatchers)
            ?.getChildren(QuotedLabelName)
            .some((n) => n.from !== node.parent?.from);
        if (!hasMetricName) {
          result.push({ kind: ContextKind.MetricName, metricName: unquotePromQLString(state.sliceDoc(node.from, node.to)) });
        }
        result.push({ kind: ContextKind.LabelName, metricName: getMetricNameInVectorSelector(node, state) });
      } else if (node.parent?.parent?.type.id === QuotedLabelMatcher) {
        // In that case the cursor is in the name of a quoted matcher, like `{"labelNa"="value"}`.
        result.push({ kind: ContextKind.LabelName, metricName: getMetricNameInVectorSelector(node, state) });
      }
      break;
    case NumberDurationLiteral:
      if (node.parent?.type.id === 0 && node.parent.parent?.type.id === SubqueryExpr) {
        // Here we are likely in this situation:
        //     `go[5d:4]`
        // and we have the given tree:
        // SubqueryExpr(
        //   VectorSelector(Identifier),
        //   Duration, Duration, ⚠(NumberLiteral)
        // )
        // So we should continue to autocomplete a duration
        if (!hasCompleteDurationUnit(state, node)) {
          result.push({ kind: ContextKind.Duration });
        }
      } else {
        result.push({ kind: ContextKind.Number });
      }
      break;
    case NumberDurationLiteralInDurationContext:
      if (!hasCompleteDurationUnit(state, node)) {
        result.push({ kind: ContextKind.Duration });
      }
      break;
    case MatrixSelector:
    case SubqueryExpr:
    case OffsetExpr:
      // Duration slot: nothing typed yet, so offer no suggestions.
      // Units appear once the user types a digit (NumberDurationLiteralInDurationContext).
      // Functions appear once the user types a letter (Identifier/LabelName handler).
      break;
    case FunctionCallBody:
      if (isAfterClosedFunctionCallBody(state, node, pos)) {
        if (!explicit) {
          // Only offer binary operators/modifiers here when explicitly requested, otherwise the
          // dropdown would pop open as soon as the closing parenthesis is typed.
          break;
        }
        if (node.parent?.type.id === AggregateExpr && !containsAtLeastOneChild(node.parent, AggregateModifier)) {
          result.push({ kind: ContextKind.AggregateOpModifier });
        }
        result.push({ kind: ContextKind.BinOp });
        break;
      }
      // For aggregation function such as Topk, the first parameter is a number.
      // The second one is an expression.
      // When moving to the second parameter, the node is an error node.
      // Unfortunately, as a current node, codemirror doesn't give us the error node but instead the FunctionCallBody
      // The tree looks like that: PromQL(AggregateExpr(AggregateOp(Topk),FunctionCallBody(NumberDurationLiteral,⚠)))
      // So, we need to figure out if the cursor is on the first parameter or in the second.
      if (isAggregatorWithParam(node)) {
        if (node.firstChild === null || (node.firstChild.from <= pos && node.firstChild.to >= pos)) {
          // it means the FunctionCallBody has no child, which means we are autocompleting the first parameter
          result.push({ kind: ContextKind.Number });
          break;
        }
        // at this point we are necessary autocompleting the second parameter
        result.push({ kind: ContextKind.MetricName, metricName: '' }, { kind: ContextKind.Function }, { kind: ContextKind.Aggregation });
        break;
      }
      // In all other cases, we are in the given situation:
      //       sum() or in rate()
      // with the cursor between the bracket. So we can autocomplete the metric, the function and the aggregation.
      result.push({ kind: ContextKind.MetricName, metricName: '' }, { kind: ContextKind.Function }, { kind: ContextKind.Aggregation });
      break;
    case Neq:
      if (node.parent?.type.id === MatchOp) {
        result.push({ kind: ContextKind.MatchOp });
      } else if (node.parent?.type.id === BinaryExpr) {
        result.push({ kind: ContextKind.BinOp });
      }
      break;
    case EqlSingle:
    case EqlRegex:
    case NeqRegex:
    case MatchOp:
      result.push({ kind: ContextKind.MatchOp });
      break;
    case Pow:
    case Mul:
    case Div:
    case Mod:
    case Add:
    case Sub:
      if (node.parent?.type.id === DurationExpr) {
        result.push({ kind: ContextKind.DurationExprOperator });
      } else {
        result.push({ kind: ContextKind.BinOp });
      }
      break;
    case Eql:
    case Gte:
    case Gtr:
    case TrimLower:
    case TrimUpper:
    case Lte:
    case Lss:
    case And:
    case Unless:
    case Or:
    case BinaryExpr:
      result.push({ kind: ContextKind.BinOp });
      break;
  }
  return result;
}

// HybridComplete provides a full completion result with or without a remote prometheus.
export class HybridComplete implements CompleteStrategy {
  private readonly prometheusClient: PrometheusClient | undefined;
  private readonly maxMetricsMetadata: number;

  constructor(prometheusClient?: PrometheusClient, maxMetricsMetadata = 10000) {
    this.prometheusClient = prometheusClient;
    this.maxMetricsMetadata = maxMetricsMetadata;
  }

  getPrometheusClient(): PrometheusClient | undefined {
    return this.prometheusClient;
  }

  destroy(): void {
    this.prometheusClient?.destroy?.();
  }

  promQL(context: CompletionContext): Promise<CompletionResult | null> | CompletionResult | null {
    const { state, pos } = context;
    let tree = syntaxTree(state).resolve(pos, -1);
    // The lines above can help you to print the current lezer tree.
    // It's useful when you are trying to understand why it doesn't autocomplete.
    // console.log(syntaxTree(state).topNode.toString());
    // console.log(`current node: ${tree.type.name}`);
    let contexts = analyzeCompletion(state, tree, pos, context.explicit);
    let from = computeStartCompletePosition(state, tree, pos);
    let to = computeEndCompletePosition(state, tree, pos);
    // A name that has to be quoted but is typed without quotes (e.g. `http.requests`) is not parsed as a single node.
    // In that case the whole word is completed, and the completion adds the missing quotes.
    const unquotedName = this.analyzeUnquotedName(state, pos, context.explicit);
    if (unquotedName) {
      ({ tree, contexts, from, to } = unquotedName);
    }
    const inString = tree.type.id === StringLiteral;
    let asyncResult: Promise<Completion[]> = Promise.resolve([]);
    let completeSnippet = false;
    let span = true;
    for (const context of contexts) {
      switch (context.kind) {
        case ContextKind.Aggregation:
          completeSnippet = true;
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.aggregateOp);
          });
          break;
        case ContextKind.Function:
          completeSnippet = true;
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.functionIdentifier);
          });
          break;
        case ContextKind.BinOpModifier:
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.binOpModifier);
          });
          break;
        case ContextKind.BinOp:
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.binOp);
          });
          break;
        case ContextKind.MatchOp:
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.matchOp);
          });
          break;
        case ContextKind.AggregateOpModifier:
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.aggregateOpModifier);
          });
          break;
        case ContextKind.Duration:
          span = false;
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.duration);
          });
          break;
        case ContextKind.DurationExpr:
          span = false;
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.durationExpr);
          });
          break;
        case ContextKind.DurationExprOperator:
          span = false;
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.durationExprOperator);
          });
          break;
        case ContextKind.Offset:
          asyncResult = asyncResult.then((result) => {
            return result.concat([{ label: 'offset' }]);
          });
          break;
        case ContextKind.Bool:
          asyncResult = asyncResult.then((result) => {
            return result.concat([{ label: 'bool' }]);
          });
          break;
        case ContextKind.AtModifiers:
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.atModifier);
          });
          break;
        case ContextKind.Number:
          asyncResult = asyncResult.then((result) => {
            return result.concat(autocompleteNodes.number);
          });
          break;
        case ContextKind.MetricName:
          asyncResult = asyncResult.then((result) => {
            return this.autocompleteMetricName(result, context, inString);
          });
          break;
        case ContextKind.LabelName:
          asyncResult = asyncResult.then((result) => {
            return this.autocompleteLabelName(result, context, inString);
          });
          break;
        case ContextKind.LabelValue:
          asyncResult = asyncResult.then((result) => {
            return this.autocompleteLabelValue(result, context);
          });
      }
    }
    return asyncResult.then((result) => {
      return arrayToCompletionResult(result, from, to, completeSnippet, span);
    });
  }

  // Handles the completion of a name that requires quotes but is written without them.
  // The lezer grammar only knows about legacy names, so such a name leads to an Identifier or a LabelName
  // followed by error nodes, or to error nodes only when it starts with a non-ASCII character.
  // It returns null when the cursor is not on such a name.
  private analyzeUnquotedName(
    state: EditorState,
    pos: number,
    explicit: boolean
  ): { tree: SyntaxNode; contexts: Context[]; from: number; to: number } | null {
    const line = state.doc.lineAt(pos);
    const name = findUnquotedUtf8Name(line.text, pos - line.from);
    if (!name) {
      return null;
    }
    const from = line.from + name.from;
    const to = line.from + name.to;
    const tree = syntaxTree(state).resolve(from, 1);
    let contexts: Context[] = [];
    if (tree.type.id === Identifier || tree.type.id === LabelName) {
      contexts = analyzeCompletion(state, tree, pos, explicit).filter((c) => c.kind === ContextKind.MetricName || c.kind === ContextKind.LabelName);
    } else if (tree.type.id === 0) {
      // The name starts with an error node: the context only depends on where the error node is.
      contexts = analyzeErrorNodeAsName(state, tree);
    }
    contexts = contexts.map((c) => (c.kind === ContextKind.MetricName ? { ...c, metricName: state.sliceDoc(from, to) } : c));
    return contexts.length > 0 ? { tree, contexts, from, to } : null;
  }

  private autocompleteMetricName(result: Completion[], context: Context, inString: boolean): Completion[] | Promise<Completion[]> {
    if (!this.prometheusClient) {
      return result;
    }
    const metricCompletion = new Map<string, Completion>();
    return this.prometheusClient
      .metricNames(context.metricName)
      .then((metricNames: string[]) => {
        for (const metricName of metricNames) {
          metricCompletion.set(metricName, { label: metricName, type: 'constant', apply: applyMetricName(metricName, inString) });
        }

        // avoid to get all metric metadata if the prometheus server is too big
        if (metricNames.length <= this.maxMetricsMetadata) {
          // in order to enrich the completion list of the metric,
          // we are trying to find the associated metadata
          return this.prometheusClient?.metricMetadata();
        }
      })
      .then((metricMetadata) => {
        if (metricMetadata) {
          for (const [metricName, node] of metricCompletion) {
            // First check if the full metric name has metadata (even if it has one of the histogram/summary/openmetrics suffixes
            // it may be a metric that is not following naming conventions)
            // Then fall back to the base metric name if full metadata doesn't exist
            const metadata = metricMetadata[metricName] ?? metricMetadata[metricName.replace(/(_count|_sum|_bucket|_total)$/, '')];
            if (metadata) {
              if (metadata.length > 1) {
                // it means the metricName has different possible helper and type
                for (const m of metadata) {
                  if (node.detail === '') {
                    node.detail = m.type;
                  } else if (node.detail !== m.type) {
                    node.detail = 'unknown';
                    node.info = 'multiple different definitions for this metric';
                  }

                  if (node.info === '') {
                    node.info = m.help;
                  } else if (node.info !== m.help) {
                    node.info = 'multiple different definitions for this metric';
                  }
                }
              } else if (metadata.length === 1) {
                let { type, help } = metadata[0];
                if (type === 'histogram' || type === 'summary') {
                  if (metricName.endsWith('_count')) {
                    type = 'counter';
                    help = `The total number of observations for: ${help}`;
                  }
                  if (metricName.endsWith('_sum')) {
                    type = 'counter';
                    help = `The total sum of observations for: ${help}`;
                  }
                  if (metricName.endsWith('_bucket')) {
                    type = 'counter';
                    help = `The total count of observations for a bucket in the histogram: ${help}`;
                  }
                }
                node.detail = type;
                node.info = help;
              }
            }
          }
        }
        return result.concat(Array.from(metricCompletion.values()));
      });
  }

  private autocompleteLabelName(result: Completion[], context: Context, inString: boolean): Completion[] | Promise<Completion[]> {
    if (!this.prometheusClient) {
      return result;
    }
    return this.prometheusClient.labelNames(context.metricName).then((labelNames: string[]) => {
      return result.concat(labelNames.map((value) => ({ label: value, type: 'constant', apply: applyLabelName(value, inString) })));
    });
  }

  private autocompleteLabelValue(result: Completion[], context: Context): Completion[] | Promise<Completion[]> {
    if (!this.prometheusClient || !context.labelName) {
      return result;
    }
    return this.prometheusClient.labelValues(context.labelName, context.metricName, context.matchers).then((labelValues: string[]) => {
      return result.concat(labelValues.map((value) => ({ label: value, apply: escapePromQLString(value), type: 'text' })));
    });
  }
}
