/* Copyright 2013 Prometheus Team
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http: *www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License. */

%{
package rules

import (
        "github.com/prometheus/prometheus/model"
        "strconv"
        "strings"
)
%}

D                       [0-9]
L                       [a-zA-Z_:]
U                       [smhdwy]

%x S_COMMENTS

%%
.                        { yypos++; REJECT }
\n                       { yyline++; yypos = 1; REJECT }

"/*"                     { BEGIN(S_COMMENTS) }
<S_COMMENTS>"*/"         { BEGIN(0) }
<S_COMMENTS>.            { /* ignore chars within multi-line comments */ }

\/\/[^\r\n]*\n           { /* gobble up one-line comments */ }

permanent                { return PERMANENT }
BY                       { return GROUP_OP }
AVG|SUM|MAX|MIN          { yylval.str = yytext; return AGGR_OP }
avg|sum|max|min          { yylval.str = strings.ToUpper(yytext); return AGGR_OP }
\<|>|AND|OR              { yylval.str = yytext; return CMP_OP }
==|!=|>=|<=              { yylval.str = yytext; return CMP_OP }
[+\-]                    { yylval.str = yytext; return ADDITIVE_OP }
[*/%]                    { yylval.str = yytext; return MULT_OP }

{D}+{U}                  { yylval.str = yytext; return DURATION }
{L}({L}|{D})*            { yylval.str = yytext; return IDENTIFIER }

\-?{D}+(\.{D}*)?         { num, err := strconv.ParseFloat(yytext, 32);
                           if (err != nil && err.(*strconv.NumError).Err == strconv.ErrSyntax) {
                             panic("Invalid float")
                           }
                           yylval.num = model.SampleValue(num)
                           return NUMBER }

\"(\\.|[^\\"])*\"        { yylval.str = yytext[1:len(yytext) - 1]; return STRING }
\'(\\.|[^\\'])*\'        { yylval.str = yytext[1:len(yytext) - 1]; return STRING }

[{}\[\]()=,]             { return int(yytext[0]) }
.                        { /* don't print any remaining chars (whitespace) */ }
\n                       { /* don't print any remaining chars (whitespace) */ }
%%
