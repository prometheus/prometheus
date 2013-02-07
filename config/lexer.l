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
package config
%}

D                       [0-9]
L                       [a-zA-Z_]

%s S_GLOBAL S_GLOBAL_LABELS S_JOB S_TARGETS S_TARGET_LABELS
%x S_COMMENTS

%%
.                        { yypos++; REJECT }
\n                       { yyline++; yypos = 1; REJECT }

"/*"                     { BEGIN(S_COMMENTS); }
<S_COMMENTS>"*/"         { BEGIN(0) }
<S_COMMENTS>.            { /* ignore chars within multi-line comments */ }

\/\/[^\r\n]*\n           { /* gobble up one-line comments */ }

<0>global                { BEGIN(S_GLOBAL); return GLOBAL }
<S_GLOBAL>labels         { BEGIN(S_GLOBAL_LABELS); return LABELS }
<S_GLOBAL>rule_files     { return RULE_FILES }
<S_GLOBAL_LABELS>"}"     { BEGIN(S_GLOBAL); REJECT }
<S_GLOBAL>"}"            { BEGIN(0); REJECT }

<0>job                   { BEGIN(S_JOB); return JOB }
<S_JOB>targets           { BEGIN(S_TARGETS); return TARGETS }
<S_TARGETS>endpoints     { return ENDPOINTS }
<S_TARGETS>labels        { BEGIN(S_TARGET_LABELS); return LABELS }
<S_TARGET_LABELS>"}"     { BEGIN(S_TARGETS); REJECT }
<S_TARGETS>"}"           { BEGIN(S_JOB); REJECT }
<S_JOB>"}"               { BEGIN(0); REJECT }

{L}({L}|{D})+            { yylval.str = yytext; return IDENTIFIER }

\"(\\.|[^\\"])*\"        { yylval.str = yytext[1:len(yytext) - 1]; return STRING }
\'(\\.|[^\\'])*\'        { yylval.str = yytext[1:len(yytext) - 1]; return STRING }

[{}\[\]()=,]             { return int(yytext[0]) }
.                        { /* don't print any remaining chars (whitespace) */ }
\n                       { /* don't print any remaining chars (whitespace) */ }
%%
