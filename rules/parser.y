// Copyright 2013 Prometheus Team
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
        package rules

        import "github.com/prometheus/prometheus/model"
        import "github.com/prometheus/prometheus/rules/ast"
%}

%union {
        num model.SampleValue
        str string
        ruleNode ast.Node
        ruleNodeSlice []ast.Node
        boolean bool
        labelNameSlice model.LabelNames
        labelSet model.LabelSet
}

/* We simulate multiple start symbols for closely-related grammars via dummy tokens. See
   http://www.gnu.org/software/bison/manual/html_node/Multiple-start_002dsymbols.html
   Reason: we want to be able to parse lists of named rules as well as single expressions.
   */
%token START_RULES START_EXPRESSION

%token <str> IDENTIFIER STRING DURATION
%token <num> NUMBER
%token PERMANENT GROUP_OP
%token <str> AGGR_OP CMP_OP ADDITIVE_OP MULT_OP
%token ALERT IF FOR WITH

%type <ruleNodeSlice> func_arg_list
%type <labelNameSlice> label_list grouping_opts
%type <labelSet> label_assign label_assign_list rule_labels
%type <ruleNode> rule_expr func_arg
%type <boolean> qualifier
%type <str> for_duration

%right '='
%left CMP_OP
%left ADDITIVE_OP
%left MULT_OP
%start start

%%
start              : START_RULES rules_stat_list
                   | START_EXPRESSION saved_rule_expr
                   ;

rules_stat_list    : /* empty */
                   | rules_stat_list rules_stat
                   ;

saved_rule_expr    : rule_expr
                     { yylex.(*RulesLexer).parsedExpr = $1 }
                   ;

rules_stat         : qualifier IDENTIFIER rule_labels '=' rule_expr
                     {
                       rule, err := CreateRecordingRule($2, $3, $5, $1)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
                     }
                   | ALERT IDENTIFIER IF rule_expr for_duration WITH rule_labels
                     {
                       rule, err := CreateAlertingRule($2, $4, $5, $7)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
                     }
                   ;

for_duration       : /* empty */
                     { $$ = "0s" }
                   | FOR DURATION
                     { $$ = $2 }
                   ;

qualifier          : /* empty */
                     { $$ = false }
                   | PERMANENT
                     { $$ = true }
                   ;

rule_labels        : /* empty */
                     { $$ = model.LabelSet{} }
                   | '{' label_assign_list '}'
                     { $$ = $2  }
                   | '{' '}'
                     { $$ = model.LabelSet{} }

label_assign_list  : label_assign
                     { $$ = $1 }
                   | label_assign_list ',' label_assign
                     { for k, v := range $3 { $$[k] = v } }
                   ;

label_assign       : IDENTIFIER '=' STRING
                     { $$ = model.LabelSet{ model.LabelName($1): model.LabelValue($3) } }
                   ;


rule_expr          : '(' rule_expr ')'
                     { $$ = $2 }
                   | IDENTIFIER rule_labels
                     { $2[model.MetricNameLabel] = model.LabelValue($1); $$ = ast.NewVectorLiteral($2) }
                   | IDENTIFIER '(' func_arg_list ')'
                     {
                       var err error
                       $$, err = NewFunctionCall($1, $3)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                     }
                   | IDENTIFIER '(' ')'
                     {
                       var err error
                       $$, err = NewFunctionCall($1, []ast.Node{})
                       if err != nil { yylex.Error(err.Error()); return 1 }
                     }
                   | rule_expr '[' DURATION ']'
                     {
                       var err error
                       $$, err = NewMatrix($1, $3)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                     }
                   | AGGR_OP '(' rule_expr ')' grouping_opts
                     {
                       var err error
                       $$, err = NewVectorAggregation($1, $3, $5)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                     }
                   /* Yacc can only attach associativity to terminals, so we
                    * have to list all operators here. */
                   | rule_expr ADDITIVE_OP rule_expr
                     {
                       var err error
                       $$, err = NewArithExpr($2, $1, $3)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                     }
                   | rule_expr MULT_OP rule_expr
                     {
                       var err error
                       $$, err = NewArithExpr($2, $1, $3)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                     }
                   | rule_expr CMP_OP rule_expr
                     {
                       var err error
                       $$, err = NewArithExpr($2, $1, $3)
                       if err != nil { yylex.Error(err.Error()); return 1 }
                     }
                   | NUMBER
                     { $$ = ast.NewScalarLiteral($1)}
                   ;

grouping_opts      :
                     { $$ = model.LabelNames{} }
                   | GROUP_OP '(' label_list ')'
                     { $$ = $3 }
                   ;

label_list         : IDENTIFIER
                     { $$ = model.LabelNames{model.LabelName($1)} }
                   | label_list ',' IDENTIFIER
                     { $$ = append($$, model.LabelName($3)) }
                   ;

func_arg_list      : func_arg
                     { $$ = []ast.Node{$1} }
                   | func_arg_list ',' func_arg
                     { $$ = append($$, $3) }
                   ;

func_arg           : rule_expr
                     { $$ = $1 }
                   | STRING
                     { $$ = ast.NewStringLiteral($1) }
                   ;
%%
