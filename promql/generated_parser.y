// Copyright 2019 The Prometheus Authors
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

%{
    package promql

    import (
        "github.com/prometheus/prometheus/pkg/labels"
    )
%}

%union {
    node Node
    item Item
    matchers []*labels.Matcher
    matcher  *labels.Matcher
}


%token <item> ERROR 
%token <item> EOF
%token <item> COMMENT
%token <item> IDENTIFIER
%token <item> METRIC_IDENTIFIER
%token <item> LEFT_PAREN
%token <item> RIGHT_PAREN
%token <item> LEFT_BRACE
%token <item> RIGHT_BRACE
%token <item> LEFT_BRACKET
%token <item> RIGHT_BRACKET
%token <item> COMMA
%token <item> ASSIGN
%token <item> COLON
%token <item> SEMICOLON
%token <item> STRING
%token <item> NUMBER
%token <item> DURATION
%token <item> BLANK
%token <item> TIMES
%token <item> SPACE

%token	operatorsStart
// Operators.
%token <item> SUB
%token <item> ADD
%token <item> MUL
%token <item> MOD
%token <item> DIV
%token <item> LAND
%token <item> LOR
%token <item> LUNLESS
%token <item> EQL
%token <item> NEQ
%token <item> LTE
%token <item> LSS
%token <item> GTE
%token <item> GTR
%token <item> EQL_REGEX
%token <item> NEQ_REGEX
%token <item> POW
%token	operatorsEnd

%token	aggregatorsStart
// Aggregators.
%token <item> AVG
%token <item> COUNT
%token <item> SUM
%token <item> MIN
%token <item> MAX
%token <item> STDDEV
%token <item> STDVAR
%token <item> TOPK
%token <item> BOTTOMK
%token <item> COUNT_VALUES
%token <item> QUANTILE
%token	aggregatorsEnd

%token	keywordsStart
// Keywords.
%token <item> OFFSET
%token <item> BY
%token <item> WITHOUT
%token <item> ON
%token <item> IGNORING
%token <item> GROUP_LEFT
%token <item> GROUP_RIGHT
%token <item> BOOL

%token keywordsEnd


%token	startSymbolsStart
// Start symbols for the generated parser.
%token START_LABELS
%token	startSymbolsEnd

%type <matchers> label_matchers label_match_list
%type <matcher> label_matcher

%type <item> match_op

%start start

%%

start           : START_LABELS label_matchers
                     {yylex.(*parser).generatedParserResult.(*VectorSelector).LabelMatchers = $2}
                | error 
                        { yylex.(*parser).errorf("unknown syntax error after parsing %v", yylex.(*parser).token.desc()) }
                ;


label_match_list:
                label_match_list COMMA label_matcher
                        { $$ = append($1, $3)}
                | label_matcher
                        { $$ = []*labels.Matcher{$1}}
                ;

label_matchers  : 
                LEFT_BRACE label_match_list RIGHT_BRACE
                        { $$ = $2 }
                | LEFT_BRACE RIGHT_BRACE
                        { $$ = []*labels.Matcher{} }

                ;

label_matcher   :
                IDENTIFIER match_op STRING
                        { $$ = yylex.(*parser).newLabelMatcher($1, $2, $3) }
                | IDENTIFIER match_op error
                        { yylex.(*parser).errorf("unexpected %v in label matching, expected string", yylex.(*parser).token.desc())}
                ;

match_op        :
                EQL {$$ =$1}
                | NEQ {$$=$1}
                | EQL_REGEX {$$=$1}
                | NEQ_REGEX {$$=$1}
                | error 
                        { yylex.(*parser).errorf("expected label matching operator but got %s", yylex.(*parser).token.Val) } 
                ;


%%
