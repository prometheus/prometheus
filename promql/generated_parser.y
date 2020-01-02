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
        "math"
        "sort"
        "strconv"
        "time"

        "github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/value"
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
    series    []sequenceValue
    uint      uint64
    float     float64
    string    string
    duration  time.Duration
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
%token START_METRIC
%token START_GROUPING_LABELS
%token START_SERIES_DESCRIPTION
%token START_EXPRESSION
%token START_METRIC_SELECTOR
%token	startSymbolsEnd

%type <matchers> label_matchers label_match_list
%type <matcher> label_matcher

%type <item> match_op metric_identifier grouping_label maybe_label unary_op aggregate_op

%type <labels> label_set_list label_set metric
%type <label> label_set_item    
%type <strings> grouping_labels grouping_label_list maybe_grouping_labels
%type <series> series_values series_item
%type <uint> uint
%type <float> series_value signed_number number
%type <node>  paren_expr unary_expr binary_expr offset_expr number_literal string_literal vector_selector matrix_selector subquery_expr function_call aggregate_expr expr bin_modifier group_modifiers bool_modifier on_or_ignoring vector_selector matrix_selector subquery_expr function_call aggregate_expr function_call_args aggregate_modifier
%type <string> string
%type <duration> duration maybe_duration

%start start

%%

start           : START_LABELS label_matchers
                     {yylex.(*parser).generatedParserResult.(*VectorSelector).LabelMatchers = $2}
                | START_METRIC metric
                     { yylex.(*parser).generatedParserResult = $2 }
                | START_GROUPING_LABELS grouping_labels
                     { yylex.(*parser).generatedParserResult = $2 }
                | START_SERIES_DESCRIPTION series_description
                | START_EXPRESSION expr
                     { yylex.(*parser).generatedParserResult = $2 }
                | START_METRIC_SELECTOR vector_selector 
                     { yylex.(*parser).generatedParserResult = $2 }
                | start EOF
                | error /* If none of the more detailed error messages are triggered, we fall back to this. */
                        { yylex.(*parser).unexpected("","") }
                ;

expr            :
                /* empty */
                        { yylex.(*parser).errorf("No expression found in input"); $$ = nil }
                | paren_expr
                | unary_expr
                | binary_expr
                | offset_expr
                | number_literal
                | string_literal
                | vector_selector
                | matrix_selector
                | subquery_expr
                | function_call
                | aggregate_expr
                ;

unary_expr      :
                unary_op expr
                        {
                        if nl, ok := $2.(*NumberLiteral); ok {
                                if $1.Typ == SUB {
                                        nl.Val *= -1
                                }
                                $$ = nl
                        } else {
                                $$ = &UnaryExpr{Op: $1.Typ, Expr: $2.(Expr)}
                        }
                        }
                ;

unary_op        :
                ADD
                | SUB
                ;

binary_expr     :
                expr POW bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr MUL bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr DIV bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr MOD bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr ADD bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr SUB bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr EQL bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr NEQ bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr LTE bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr LSS bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr GTE bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr GTR bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr LAND bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr LUNLESS bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                | expr LOR bin_modifier expr
                        { $$ = yylex.(*parser).newBinaryExpression($1, $2, $3, $4)} 
                ;

// Using left recursion for the modifier rules, helps to keep the parser stack small and 
// reduces allocations

bin_modifier    :
                group_modifiers
                ;

bool_modifier   :
                /* empty */
                        { $$ = &BinaryExpr{
                        VectorMatching: &VectorMatching{Card: CardOneToOne},
                        }
                        }
                | BOOL
                        { $$ = &BinaryExpr{
                        VectorMatching: &VectorMatching{Card: CardOneToOne},
                        ReturnBool:     true,
                        }
                        }
                ;

on_or_ignoring  :
                bool_modifier /* empty */
                | bool_modifier IGNORING grouping_labels
                        {
                        $$ = $1
                        $$.(*BinaryExpr).VectorMatching.MatchingLabels = $3
                        } 
                | bool_modifier ON grouping_labels
                        {
                        $$ = $1
                        $$.(*BinaryExpr).VectorMatching.MatchingLabels = $3
                        $$.(*BinaryExpr).VectorMatching.On = true
                        } 
                ;

group_modifiers:
                on_or_ignoring /* empty */
                | on_or_ignoring GROUP_LEFT maybe_grouping_labels
                        {
                        $$ = $1
                        $$.(*BinaryExpr).VectorMatching.Card = CardManyToOne
                        $$.(*BinaryExpr).VectorMatching.Include = $3
                        }
                | on_or_ignoring GROUP_RIGHT maybe_grouping_labels
                        {
                        $$ = $1
                        $$.(*BinaryExpr).VectorMatching.Card = CardOneToMany
                        $$.(*BinaryExpr).VectorMatching.Include = $3
                        }
                ;

maybe_grouping_labels:
                /* empty */
                        { $$ = nil }
                | grouping_labels
                ;

paren_expr      : LEFT_PAREN expr RIGHT_PAREN
                        { $$ = &ParenExpr{Expr: $2.(Expr)} }
                ;


primary_expr    :
                number_literal
                ;

number_literal  :
                number
                        { $$ = &NumberLiteral{$1}}
                ;

string_literal  :
                string
                        { $$ = &StringLiteral{$1}}
                ;

string          :
                STRING
                        { $$ = yylex.(*parser).unquoteString($1.Val) }
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
                        { yylex.(*parser).unexpected("label matching", "\",\" or \"}\"") }
                ;

label_matcher   :
                IDENTIFIER match_op STRING
                        { $$ = yylex.(*parser).newLabelMatcher($1, $2, $3) }
                | IDENTIFIER match_op error
                        { yylex.(*parser).unexpected("label matching", "string")}
                | IDENTIFIER error 
                        { yylex.(*parser).unexpected("label matching", "label matching operator") } 
                | error
                        { yylex.(*parser).unexpected("label matching", "identifier or \"}\"")}
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
              /*| error
                        { yylex.(*parser).errorf("missing metric name or metric selector")} */
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
                        { yylex.(*parser).unexpected("label set", "\",\" or \"}\"", ) }
                
                ;

label_set_item  :
                IDENTIFIER EQL STRING
                        { $$ = labels.Label{Name: $1.Val, Value: yylex.(*parser).unquoteString($3.Val) } } 
                | IDENTIFIER EQL error
                        { yylex.(*parser).unexpected("label set", "string")}
                | IDENTIFIER error
                        { yylex.(*parser).unexpected("label set", "\"=\"")}
                | error
                        { yylex.(*parser).unexpected("label set", "identifier or \"}\"") }
                ;

grouping_labels :
                LEFT_PAREN grouping_label_list RIGHT_PAREN
                        { $$ = $2 }
                | LEFT_PAREN grouping_label_list COMMA RIGHT_PAREN
                        { $$ = $2 }
                | LEFT_PAREN RIGHT_PAREN
                        { $$ = []string{} }
                | error
                        { yylex.(*parser).unexpected("grouping opts", "\"(\"") }
                ;


grouping_label_list:
                grouping_label_list COMMA grouping_label
                        { $$ = append($1, $3.Val) }
                | grouping_label
                        { $$ = []string{$1.Val} }
                | grouping_label_list error
                        { yylex.(*parser).unexpected("grouping opts", "\",\" or \"}\"") }
                ;

grouping_label  :
                maybe_label
                        {
                        if !isLabel($1.Val) {
                                yylex.(*parser).unexpected("grouping opts", "label")
                        }
                        $$ = $1
                        }
                | error
                        { yylex.(*parser).unexpected("grouping opts", "label") }
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

offset_expr:
                expr OFFSET DURATION
                        {
                        offset, err := parseDuration($3.Val)
                        if err != nil {
                                yylex.(*parser).error(err)
                        }
                        yylex.(*parser).addOffset($1, offset)
                        $$ = $1
                        }
                ;

vector_selector:
                metric_identifier label_matchers
                        { $$ = &VectorSelector{Name: $1.Val, LabelMatchers:$2} }
                | metric_identifier 
                        { $$ = &VectorSelector{Name: $1.Val} }
                | label_matchers
                        { $$ = &VectorSelector{LabelMatchers:$1} }
                ;

matrix_selector :
                vector_selector LEFT_BRACKET duration RIGHT_BRACKET
                        {
                        $$ = $1
                        $$.(*MatrixSelector).Range = $3 
                        }
                ;

subquery_expr   :
                expr LEFT_BRACKET duration COLON maybe_duration RIGHT_BRACKET
                        {
                        $$ = &SubqueryExpr{
                                Expr:  $1.(Expr),
                                Range: $3,
                                Step:  $5,
                        }
                        }
                ;
                        

maybe_duration  :
                /* empty */
                        {$$ = 0}
                | duration
                ;

duration        :
                DURATION
                        {
                        var err error
                        $$, err = parseDuration($1.Val)
                        if err != nil {
                                yylex.(*parser).error(err)
                        }
                        }
                ;
                
function_call   :
                IDENTIFIER LEFT_PAREN function_call_args RIGHT_PAREN
                        {
                        fn, exist := getFunction($1.Val)
                        if !exist{
                                yylex.(*parser).errorf("unknown function with name %q", $1.Val)
                        } 
                        $$ = &Call{
                                Func: fn,
                                Args: $3.(Expressions),
                        }
                        }
                ;

function_call_args:
                function_call_args COMMA expr
                        { $$ = append($1.(Expressions), $3.(Expr)) }
                | expr
                        { $$ = Expressions{$1.(Expr)} }
                ;

// TODO param support
// Consider unifying calls and aggregate expressions
aggregate_expr  :
                aggregate_op aggregate_modifier expr
                        {
                        $$ = $2
                        $$.(*AggregateExpr).Op = $1.Typ
                        $$.(*AggregateExpr).Expr = $3.(Expr)
                        }
                | aggregate_op expr aggregate_modifier
                        {
                        $$ = $3
                        $$.(*AggregateExpr).Op = $1.Typ
                        $$.(*AggregateExpr).Expr = $2.(Expr)
                        }
                | aggregate_op expr
                        {
                        $$ = &AggregateExpr{
                                Op: $1.Typ,
                                Expr: $2.(Expr),
                        }
                        }
                ;

aggregate_op    :
                AVG
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
                ;

aggregate_modifier:
                BY grouping_labels
                        {
                        $$ = &AggregateExpr{
                                Grouping: $2,
                        }
                        }
                | WITHOUT grouping_labels
                        {
                        $$ = &AggregateExpr{
                                Grouping: $2,
                                Without:  true,
                        }
                        }
                ;

series_description:
                metric series_values
                        {
                        yylex.(*parser).generatedParserResult = &seriesDescription{
                                labels: $1,
                                values: $2,
                        }
                        }
                ;

series_values   :
                /*empty*/
                        { $$ = []sequenceValue{} }
                | series_values SPACE series_item
                        { $$ = append($1, $3...) }
                | series_values SPACE
                        { $$ = $1 }
                | error
                        { yylex.(*parser).unexpected("series values", "") }
                ;

series_item     :
                BLANK
                        { $$ = []sequenceValue{{omitted: true}}}
                | BLANK TIMES uint
                        {
                        $$ = []sequenceValue{}
                        for i:=uint64(0); i < $3; i++{
                                $$ = append($$, sequenceValue{omitted: true})
                        }
                        }
                | series_value
                        { $$ = []sequenceValue{{value: $1}}}
                | series_value TIMES uint
                        {
                        $$ = []sequenceValue{}
                        for i:=uint64(0); i <= $3; i++{
                                $$ = append($$, sequenceValue{value: $1})
                        }
                        }
                | series_value signed_number TIMES uint
                        {
                        $$ = []sequenceValue{}
                        for i:=uint64(0); i <= $4; i++{
                                $$ = append($$, sequenceValue{value: $1})
                                $1 += $2
                        }
                        }
uint            :
                NUMBER
                        {
                        var err error
                        $$, err = strconv.ParseUint($1.Val, 10, 64)
                        if err != nil {
                                yylex.(*parser).errorf("invalid repitition in series values: %s", err)
                        }
                        }
                ;

signed_number   :
                ADD number
                        { $$ = $2 }
                | SUB number
                        { $$ = -$2 }
                ;

series_value    :
                IDENTIFIER
                        {
                        if $1.Val != "stale" {
                                yylex.(*parser).unexpected("series values", "number or \"stale\"")
                        }
                        $$ = math.Float64frombits(value.StaleNaN)
                        }
                | number
                        { $$ = $1 }
                | signed_number
                        { $$ = $1 }
                ;



number          :
                NUMBER
                        {$$ = yylex.(*parser).number($1.Val) }
                ;

%%
