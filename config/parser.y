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
        package config

        import "fmt"
        import "github.com/matttproud/prometheus/model"
%}

%union {
        num model.SampleValue
        str string
        stringSlice []string
        labelSet model.LabelSet
}

%token <str> IDENTIFIER STRING
%token GLOBAL JOB
%token RULE_FILES 
%token LABELS TARGETS ENDPOINTS

%type <stringSlice> string_array string_list rule_files_stat endpoints_stat
%type <labelSet> labels_stat label_assign label_assign_list

%start config

%%
config             : /* empty */
                   | config config_stanza
                   ;

config_stanza      : GLOBAL '{' global_stat_list '}'
                   | JOB    '{' job_stat_list    '}'
                     { PopJob() }
                   ;

global_stat_list   : /* empty */
                   | global_stat_list global_stat
                   ;

global_stat        : IDENTIFIER '=' STRING
                     { parsedConfig.Global.SetOption($1, $3) }
                   | labels_stat
                     { parsedConfig.Global.SetLabels($1) }
                   | rule_files_stat
                     { parsedConfig.Global.AddRuleFiles($1) }
                   ;

labels_stat        : LABELS '{' label_assign_list '}'
                     { $$ = $3 }
                   | LABELS '{' '}'
                     { $$ = model.LabelSet{} }
                   ;

label_assign_list  : label_assign
                     { $$ = $1 }
                   | label_assign_list ',' label_assign
                     { for k, v := range $3 { $$[k] = v } }
                   ;

label_assign       : IDENTIFIER '=' STRING
                     { $$ = model.LabelSet{ model.LabelName($1): model.LabelValue($3) } }
                   ;

rule_files_stat    : RULE_FILES '=' string_array
                     { $$ = $3 }
                   ;

job_stat_list      : /* empty */
                   | job_stat_list job_stat
                   ;

job_stat           : IDENTIFIER '=' STRING
                     { PushJobOption($1, $3) }
                   | TARGETS '{' targets_stat_list '}'
                     { PushJobTargets() }
                   ;

targets_stat_list  : /* empty */
                   | targets_stat_list targets_stat
                   ;

targets_stat       : endpoints_stat
                     { PushTargetEndpoints($1) }
                   | labels_stat
                     { PushTargetLabels($1) }
                   ;

endpoints_stat     : ENDPOINTS '=' string_array
                     { $$ = $3 }
                   ;

string_array       : '[' string_list ']'
                     { $$ = $2 }
                   | '[' ']'
                     { $$ = []string{} }
                   ;

string_list        : STRING
                     { $$ = []string{$1} }
                   | string_list ',' STRING
                     { $$ = append($$, $3) }
                   ;
%%
