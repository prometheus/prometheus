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
        "sort"

        "github.com/prometheus/prometheus/pkg/labels"
    )
%}

%union {
    node      Node
    item      Item
    matchers  []*labels.Matcher
    matcher   *labels.Matcher
    label     labels.Label
    labels    labels.Labels
    strings   []string
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
%token START_LABEL_SET
%token START_METRIC
%token START_GROUPING_LABELS
%token	startSymbolsEnd

%type <matchers> label_matchers label_match_list
%type <matcher> label_matcher

%type <item> match_op metric_identifier grouping_label maybe_label

%type <labels> label_set_list label_set metric
%type <label> label_set_item    
%type <strings> grouping_labels  grouping_label_list

%start start

%%

start           : START_LABELS label_matchers
                     {yylex.(*parser).generatedParserResult.(*VectorSelector).LabelMatchers = $2}
                | START_LABEL_SET label_set 
                     { yylex.(*parser).generatedParserResult = $2 }
                | START_METRIC metric
                     { yylex.(*parser).generatedParserResult = $2 }
                | START_GROUPING_LABELS grouping_labels
                     { yylex.(*parser).generatedParserResult = $2 }
                | error /* If none of the more detailed error messages are triggered, we fall back to this. */
                        { yylex.(*parser).errorf("unexpected %v", yylex.(*parser).token.desc()) }
                ;


label_matchers  : 
                LEFT_BRACE label_match_list RIGHT_BRACE
                        { $$ = $2 }
                | LEFT_BRACE label_match_list COMMA RIGHT_BRACE
                        { $$ = $2 }
                | LEFT_BRACE RIGHT_BRACE
                        { $$ = []*labels.Matcher{} }

                ;

label_match_list:
                label_match_list COMMA label_matcher
                        { $$ = append($1, $3)}
                | label_matcher
                        { $$ = []*labels.Matcher{$1}}
                | label_match_list error
                        { yylex.(*parser).errorf("unexpected %v in label matching, expected \",\" or \"}\"", yylex.(*parser).token.desc()) }
                ;

label_matcher   :
                IDENTIFIER match_op STRING
                        { $$ = yylex.(*parser).newLabelMatcher($1, $2, $3) }
                | IDENTIFIER match_op error
                        { yylex.(*parser).errorf("unexpected %v in label matching, expected string", yylex.(*parser).token.desc())}
                | IDENTIFIER error 
                        { yylex.(*parser).errorf("unexpected %v in label matching, expected label matching operator", yylex.(*parser).token.Val) } 
                | error
                        { yylex.(*parser).errorf("unexpected %v in label matching, expected identifier or \"}\"", yylex.(*parser).token.desc()) }
                ;

match_op        :
                EQL {$$ =$1}
                | NEQ {$$=$1}
                | EQL_REGEX {$$=$1}
                | NEQ_REGEX {$$=$1}
                ;


metric          :
                metric_identifier label_set
                        { $$ = append($2, labels.Label{Name: labels.MetricName, Value: $1.Val}); sort.Sort($$) }
                | label_set 
                        {$$ = $1}
                | error
                        { yylex.(*parser).errorf("missing metric name or metric selector")}
                ;

metric_identifier
                :
                METRIC_IDENTIFIER {$$=$1}
                | IDENTIFIER      {$$=$1}

label_set       :
                LEFT_BRACE label_set_list RIGHT_BRACE
                        { $$ = labels.New($2...) }
                | LEFT_BRACE label_set_list COMMA RIGHT_BRACE
                        { $$ = labels.New($2...) }
                | LEFT_BRACE RIGHT_BRACE
                        { $$ = labels.New() }
                | /* empty */
                        { $$ = labels.New() }
                ;

label_set_list  :
                label_set_list COMMA label_set_item
                        { $$ = append($1, $3) }
                | label_set_item
                        { $$ = []labels.Label{$1} }
                | label_set_list error
                        { yylex.(*parser).errorf("unexpected %v in label set, expected \",\" or \"}\"", yylex.(*parser).token.desc()) }
                
                ;

label_set_item  :
                IDENTIFIER EQL STRING
                        { $$ = labels.Label{Name: $1.Val, Value: yylex.(*parser).unquoteString($3.Val) } } 
                | IDENTIFIER EQL error
                        { yylex.(*parser).errorf("unexpected %v in label set, expected string", yylex.(*parser).token.desc())}
                | IDENTIFIER error
                        { yylex.(*parser).errorf("unexpected %v in label set, expected \"=\"", yylex.(*parser).token.desc())}
                | error
                        { yylex.(*parser).errorf("unexpected %v in label set, expected identifier or \"}\"", yylex.(*parser).token.desc()) }
                ;

grouping_labels :
                LEFT_PAREN grouping_label_list RIGHT_PAREN
                        { $$ = $2 }
                | LEFT_PAREN RIGHT_PAREN
                        { $$ = []string{} }
                | error
                        { yylex.(*parser).errorf("unexpected %v in grouping opts, expected \"(\"", yylex.(*parser).token.desc()) }
                ;


grouping_label_list:
                grouping_label_list COMMA grouping_label
                        { $$ = append($1, $3.Val) }
                | grouping_label
                        { $$ = []string{$1.Val} }
                | grouping_label_list error
                        { yylex.(*parser).errorf("unexpected %v in grouping opts, expected \",\" or \"}\"", yylex.(*parser).token.desc()) }
                ;

grouping_label  :
                maybe_label
                        {
                        if !isLabel($1.Val) {
                                yylex.(*parser).errorf("unexpected %s in grouping opts, expected label", $1.desc())
                        }
                        $$ = $1
                        }
                | error
                        { yylex.(*parser).errorf("unexpected %s in grouping opts, expected label", yylex.(*parser).token.desc()) }
                ;


/* inside of grouping options label names can be recognized as keywords by the lexer */
maybe_label     :
                IDENTIFIER
                | METRIC_IDENTIFIER
                | LAND
                | LOR
                | LUNLESS
                | AVG
                | COUNT
                | SUM
                | MIN
                | MAX
                | STDDEV
                | STDVAR
                | TOPK
                | BOTTOMK
                | COUNT_VALUES
                | QUANTILE
                | OFFSET
                | BY
                | ON
                | IGNORING
                | GROUP_LEFT
                | GROUP_RIGHT
                | BOOL
                ;

%%
