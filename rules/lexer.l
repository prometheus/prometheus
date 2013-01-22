%{
package rules

import (
        "github.com/matttproud/prometheus/model"
        "strconv"
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
\<|>|AND|OR              { yylval.str = yytext; return CMP_OP }
==|!=|>=|<=              { yylval.str = yytext; return CMP_OP }
[+\-]                    { yylval.str = yytext; return ADDITIVE_OP }
[*/%]                    { yylval.str = yytext; return MULT_OP }

{D}+{U}                  { yylval.str = yytext; return DURATION }
{L}({L}|{D})+            { yylval.str = yytext; return IDENTIFIER }

\-?{D}+(\.{D}*)?         { num, err := strconv.ParseFloat(yytext, 32);
                           if (err != nil) { rulesError("Invalid float %v", yytext) }
                           yylval.num = model.SampleValue(num)
                           return NUMBER }

\"(\\.|[^\\"])*\"        { yylval.str = yytext[1:len(yytext) - 1]; return STRING }
\'(\\.|[^\\'])*\'        { yylval.str = yytext[1:len(yytext) - 1]; return STRING }

[{}\[\]()=,]             { return int(yytext[0]) }
.                        { /* don't print any remaining chars (whitespace) */ }
\n                       { /* don't print any remaining chars (whitespace) */ }
%%
