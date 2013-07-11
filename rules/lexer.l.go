package rules

import (
        "fmt"
        "strconv"
        "strings"

        clientmodel "github.com/prometheus/client_golang/model"
)

func (lexer *RulesLexer) Lex(lval *yySymType) int {
  // Internal lexer states.
  const (
    S_INITIAL = iota
    S_COMMENTS
  )

  // We simulate multiple start symbols for closely-related grammars via dummy tokens. See
  // http://www.gnu.org/software/bison/manual/html_node/Multiple-start_002dsymbols.html
  // Reason: we want to be able to parse lists of named rules as well as single expressions.
  if lexer.startToken != 0 {
    startToken := lexer.startToken
    lexer.startToken = 0
    return startToken
  }

  c := lexer.current
  currentState := 0

  if lexer.empty {
    c, lexer.empty = lexer.getChar(), false
  }



yystate0:

  lexer.buf = lexer.buf[:0]   // The code before the first rule executed before every scan cycle (rule #0 / state 0 action)

switch yyt := currentState; yyt {
default:
panic(fmt.Errorf(`invalid start condition %d`, yyt))
case 0: // start condition: INITIAL
goto yystart1
case 1: // start condition: S_COMMENTS
goto yystart97
}

goto yystate1 // silence unused label error
yystate1:
c = lexer.getChar()
yystart1:
switch {
default:
goto yyabort
case c == '!':
goto yystate3
case c == '"':
goto yystate5
case c == '%' || c == '*':
goto yystate8
case c == '(' || c == ')' || c == ',' || c == '[' || c == ']' || c == '{' || c == '}':
goto yystate12
case c == '+':
goto yystate13
case c == '-':
goto yystate14
case c == '/':
goto yystate17
case c == ':' || c == 'D' || c == 'E' || c == 'G' || c == 'H' || c >= 'J' && c <= 'L' || c == 'N' || c == 'Q' || c == 'R' || c >= 'T' && c <= 'V' || c >= 'X' && c <= 'Z' || c == '_' || c == 'd' || c == 'e' || c == 'g' || c == 'h' || c >= 'j' && c <= 'l' || c == 'n' || c == 'q' || c == 'r' || c >= 't' && c <= 'v' || c >= 'x' && c <= 'z':
goto yystate23
case c == '<' || c == '>':
goto yystate24
case c == '=':
goto yystate25
case c == 'A':
goto yystate26
case c == 'B':
goto yystate35
case c == 'C':
goto yystate37
case c == 'F':
goto yystate41
case c == 'I':
goto yystate44
case c == 'M':
goto yystate46
case c == 'O':
goto yystate49
case c == 'P':
goto yystate50
case c == 'S':
goto yystate59
case c == 'W':
goto yystate61
case c == '\'':
goto yystate9
case c == '\t' || c == '\n' || c == '\r' || c == ' ':
goto yystate2
case c == 'a':
goto yystate65
case c == 'b':
goto yystate72
case c == 'c':
goto yystate73
case c == 'f':
goto yystate77
case c == 'i':
goto yystate79
case c == 'm':
goto yystate80
case c == 'o':
goto yystate83
case c == 'p':
goto yystate84
case c == 's':
goto yystate92
case c == 'w':
goto yystate94
case c >= '0' && c <= '9':
goto yystate21
}

yystate2:
c = lexer.getChar()
goto yyrule23

yystate3:
c = lexer.getChar()
switch {
default:
goto yyabort
case c == '=':
goto yystate4
}

yystate4:
c = lexer.getChar()
goto yyrule14

yystate5:
c = lexer.getChar()
switch {
default:
goto yyabort
case c == '"':
goto yystate6
case c == '\\':
goto yystate7
case c >= '\x01' && c <= '!' || c >= '#' && c <= '[' || c >= ']' && c <= 'ÿ':
goto yystate5
}

yystate6:
c = lexer.getChar()
goto yyrule20

yystate7:
c = lexer.getChar()
switch {
default:
goto yyabort
case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
goto yystate5
}

yystate8:
c = lexer.getChar()
goto yyrule16

yystate9:
c = lexer.getChar()
switch {
default:
goto yyabort
case c == '\'':
goto yystate10
case c == '\\':
goto yystate11
case c >= '\x01' && c <= '&' || c >= '(' && c <= '[' || c >= ']' && c <= 'ÿ':
goto yystate9
}

yystate10:
c = lexer.getChar()
goto yyrule21

yystate11:
c = lexer.getChar()
switch {
default:
goto yyabort
case c >= '\x01' && c <= '\t' || c >= '\v' && c <= 'ÿ':
goto yystate9
}

yystate12:
c = lexer.getChar()
goto yyrule22

yystate13:
c = lexer.getChar()
goto yyrule15

yystate14:
c = lexer.getChar()
switch {
default:
goto yyrule15
case c >= '0' && c <= '9':
goto yystate15
}

yystate15:
c = lexer.getChar()
switch {
default:
goto yyrule19
case c == '.':
goto yystate16
case c >= '0' && c <= '9':
goto yystate15
}

yystate16:
c = lexer.getChar()
switch {
default:
goto yyrule19
case c >= '0' && c <= '9':
goto yystate16
}

yystate17:
c = lexer.getChar()
switch {
default:
goto yyrule16
case c == '*':
goto yystate18
case c == '/':
goto yystate19
}

yystate18:
c = lexer.getChar()
goto yyrule1

yystate19:
c = lexer.getChar()
switch {
default:
goto yyabort
case c == '\n':
goto yystate20
case c >= '\x01' && c <= '\t' || c == '\v' || c == '\f' || c >= '\x0e' && c <= 'ÿ':
goto yystate19
}

yystate20:
c = lexer.getChar()
goto yyrule4

yystate21:
c = lexer.getChar()
switch {
default:
goto yyrule19
case c == '.':
goto yystate16
case c == 'd' || c == 'h' || c == 'm' || c == 's' || c == 'w' || c == 'y':
goto yystate22
case c >= '0' && c <= '9':
goto yystate21
}

yystate22:
c = lexer.getChar()
goto yyrule17

yystate23:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate24:
c = lexer.getChar()
switch {
default:
goto yyrule13
case c == '=':
goto yystate4
}

yystate25:
c = lexer.getChar()
switch {
default:
goto yyrule22
case c == '=':
goto yystate4
}

yystate26:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'L':
goto yystate27
case c == 'N':
goto yystate31
case c == 'V':
goto yystate33
case c >= '0' && c <= ':' || c >= 'A' && c <= 'K' || c == 'M' || c >= 'O' && c <= 'U' || c >= 'W' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate27:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'E':
goto yystate28
case c >= '0' && c <= ':' || c >= 'A' && c <= 'D' || c >= 'F' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate28:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'R':
goto yystate29
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Q' || c >= 'S' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate29:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'T':
goto yystate30
case c >= '0' && c <= ':' || c >= 'A' && c <= 'S' || c >= 'U' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate30:
c = lexer.getChar()
switch {
default:
goto yyrule5
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate31:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'D':
goto yystate32
case c >= '0' && c <= ':' || c >= 'A' && c <= 'C' || c >= 'E' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate32:
c = lexer.getChar()
switch {
default:
goto yyrule13
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate33:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'G':
goto yystate34
case c >= '0' && c <= ':' || c >= 'A' && c <= 'F' || c >= 'H' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate34:
c = lexer.getChar()
switch {
default:
goto yyrule11
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate35:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'Y':
goto yystate36
case c >= '0' && c <= ':' || c >= 'A' && c <= 'X' || c == 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate36:
c = lexer.getChar()
switch {
default:
goto yyrule10
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate37:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'O':
goto yystate38
case c >= '0' && c <= ':' || c >= 'A' && c <= 'N' || c >= 'P' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate38:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'U':
goto yystate39
case c >= '0' && c <= ':' || c >= 'A' && c <= 'T' || c >= 'V' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate39:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'N':
goto yystate40
case c >= '0' && c <= ':' || c >= 'A' && c <= 'M' || c >= 'O' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate40:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'T':
goto yystate34
case c >= '0' && c <= ':' || c >= 'A' && c <= 'S' || c >= 'U' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate41:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'O':
goto yystate42
case c >= '0' && c <= ':' || c >= 'A' && c <= 'N' || c >= 'P' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate42:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'R':
goto yystate43
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Q' || c >= 'S' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate43:
c = lexer.getChar()
switch {
default:
goto yyrule7
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate44:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'F':
goto yystate45
case c >= '0' && c <= ':' || c >= 'A' && c <= 'E' || c >= 'G' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate45:
c = lexer.getChar()
switch {
default:
goto yyrule6
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate46:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'A':
goto yystate47
case c == 'I':
goto yystate48
case c >= '0' && c <= ':' || c >= 'B' && c <= 'H' || c >= 'J' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate47:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'X':
goto yystate34
case c >= '0' && c <= ':' || c >= 'A' && c <= 'W' || c == 'Y' || c == 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate48:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'N':
goto yystate34
case c >= '0' && c <= ':' || c >= 'A' && c <= 'M' || c >= 'O' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate49:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'R':
goto yystate32
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Q' || c >= 'S' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate50:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'E':
goto yystate51
case c >= '0' && c <= ':' || c >= 'A' && c <= 'D' || c >= 'F' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate51:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'R':
goto yystate52
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Q' || c >= 'S' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate52:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'M':
goto yystate53
case c >= '0' && c <= ':' || c >= 'A' && c <= 'L' || c >= 'N' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate53:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'A':
goto yystate54
case c >= '0' && c <= ':' || c >= 'B' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate54:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'N':
goto yystate55
case c >= '0' && c <= ':' || c >= 'A' && c <= 'M' || c >= 'O' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate55:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'E':
goto yystate56
case c >= '0' && c <= ':' || c >= 'A' && c <= 'D' || c >= 'F' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate56:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'N':
goto yystate57
case c >= '0' && c <= ':' || c >= 'A' && c <= 'M' || c >= 'O' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate57:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'T':
goto yystate58
case c >= '0' && c <= ':' || c >= 'A' && c <= 'S' || c >= 'U' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate58:
c = lexer.getChar()
switch {
default:
goto yyrule9
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate59:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'U':
goto yystate60
case c >= '0' && c <= ':' || c >= 'A' && c <= 'T' || c >= 'V' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate60:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'M':
goto yystate34
case c >= '0' && c <= ':' || c >= 'A' && c <= 'L' || c >= 'N' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate61:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'I':
goto yystate62
case c >= '0' && c <= ':' || c >= 'A' && c <= 'H' || c >= 'J' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate62:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'T':
goto yystate63
case c >= '0' && c <= ':' || c >= 'A' && c <= 'S' || c >= 'U' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate63:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'H':
goto yystate64
case c >= '0' && c <= ':' || c >= 'A' && c <= 'G' || c >= 'I' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate64:
c = lexer.getChar()
switch {
default:
goto yyrule8
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate65:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'l':
goto yystate66
case c == 'n':
goto yystate69
case c == 'v':
goto yystate70
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'k' || c == 'm' || c >= 'o' && c <= 'u' || c >= 'w' && c <= 'z':
goto yystate23
}

yystate66:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'e':
goto yystate67
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'd' || c >= 'f' && c <= 'z':
goto yystate23
}

yystate67:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'r':
goto yystate68
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'q' || c >= 's' && c <= 'z':
goto yystate23
}

yystate68:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 't':
goto yystate30
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 's' || c >= 'u' && c <= 'z':
goto yystate23
}

yystate69:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'd':
goto yystate32
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'c' || c >= 'e' && c <= 'z':
goto yystate23
}

yystate70:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'g':
goto yystate71
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'f' || c >= 'h' && c <= 'z':
goto yystate23
}

yystate71:
c = lexer.getChar()
switch {
default:
goto yyrule12
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'z':
goto yystate23
}

yystate72:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'y':
goto yystate36
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'x' || c == 'z':
goto yystate23
}

yystate73:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'o':
goto yystate74
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'n' || c >= 'p' && c <= 'z':
goto yystate23
}

yystate74:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'u':
goto yystate75
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 't' || c >= 'v' && c <= 'z':
goto yystate23
}

yystate75:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'n':
goto yystate76
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'm' || c >= 'o' && c <= 'z':
goto yystate23
}

yystate76:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 't':
goto yystate71
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 's' || c >= 'u' && c <= 'z':
goto yystate23
}

yystate77:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'o':
goto yystate78
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'n' || c >= 'p' && c <= 'z':
goto yystate23
}

yystate78:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'r':
goto yystate43
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'q' || c >= 's' && c <= 'z':
goto yystate23
}

yystate79:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'f':
goto yystate45
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'e' || c >= 'g' && c <= 'z':
goto yystate23
}

yystate80:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'a':
goto yystate81
case c == 'i':
goto yystate82
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'b' && c <= 'h' || c >= 'j' && c <= 'z':
goto yystate23
}

yystate81:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'x':
goto yystate71
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'w' || c == 'y' || c == 'z':
goto yystate23
}

yystate82:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'n':
goto yystate71
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'm' || c >= 'o' && c <= 'z':
goto yystate23
}

yystate83:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'r':
goto yystate32
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'q' || c >= 's' && c <= 'z':
goto yystate23
}

yystate84:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'e':
goto yystate85
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'd' || c >= 'f' && c <= 'z':
goto yystate23
}

yystate85:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'r':
goto yystate86
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'q' || c >= 's' && c <= 'z':
goto yystate23
}

yystate86:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'm':
goto yystate87
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'l' || c >= 'n' && c <= 'z':
goto yystate23
}

yystate87:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'a':
goto yystate88
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'b' && c <= 'z':
goto yystate23
}

yystate88:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'n':
goto yystate89
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'm' || c >= 'o' && c <= 'z':
goto yystate23
}

yystate89:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'e':
goto yystate90
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'd' || c >= 'f' && c <= 'z':
goto yystate23
}

yystate90:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'n':
goto yystate91
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'm' || c >= 'o' && c <= 'z':
goto yystate23
}

yystate91:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 't':
goto yystate58
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 's' || c >= 'u' && c <= 'z':
goto yystate23
}

yystate92:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'u':
goto yystate93
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 't' || c >= 'v' && c <= 'z':
goto yystate23
}

yystate93:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'm':
goto yystate71
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'l' || c >= 'n' && c <= 'z':
goto yystate23
}

yystate94:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'i':
goto yystate95
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'h' || c >= 'j' && c <= 'z':
goto yystate23
}

yystate95:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 't':
goto yystate96
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 's' || c >= 'u' && c <= 'z':
goto yystate23
}

yystate96:
c = lexer.getChar()
switch {
default:
goto yyrule18
case c == 'h':
goto yystate64
case c >= '0' && c <= ':' || c >= 'A' && c <= 'Z' || c == '_' || c >= 'a' && c <= 'g' || c >= 'i' && c <= 'z':
goto yystate23
}

goto yystate97 // silence unused label error
yystate97:
c = lexer.getChar()
yystart97:
switch {
default:
goto yyabort
case c == '*':
goto yystate99
case c >= '\x01' && c <= ')' || c >= '+' && c <= 'ÿ':
goto yystate98
}

yystate98:
c = lexer.getChar()
goto yyrule3

yystate99:
c = lexer.getChar()
switch {
default:
goto yyrule3
case c == '/':
goto yystate100
}

yystate100:
c = lexer.getChar()
goto yyrule2

yyrule1: // "/*"
{
                    currentState = S_COMMENTS
goto yystate0
}
yyrule2: // "*/"
{
        currentState = S_INITIAL
goto yystate0
}
yyrule3: // .|\n
{
        /* ignore chars within multi-line comments */
goto yystate0
}
yyrule4: // \/\/[^\r\n]*\n
{
          /* gobble up one-line comments */
goto yystate0
}
yyrule5: // ALERT|alert
{
             return ALERT
}
yyrule6: // IF|if
{
                   return IF
}
yyrule7: // FOR|for
{
                 return FOR
}
yyrule8: // WITH|with
{
               return WITH
}
yyrule9: // PERMANENT|permanent
{
     return PERMANENT
}
yyrule10: // BY|by
{
                   return GROUP_OP
}
yyrule11: // AVG|SUM|MAX|MIN|COUNT
{
   lval.str = lexer.token(); return AGGR_OP
goto yystate0
}
yyrule12: // avg|sum|max|min|count
{
   lval.str = strings.ToUpper(lexer.token()); return AGGR_OP
goto yystate0
}
yyrule13: // \<|>|AND|OR|and|or
{
      lval.str = strings.ToUpper(lexer.token()); return CMP_OP
goto yystate0
}
yyrule14: // ==|!=|>=|<=
{
             lval.str = lexer.token(); return CMP_OP
goto yystate0
}
yyrule15: // [+\-]
{
                   lval.str = lexer.token(); return ADDITIVE_OP
goto yystate0
}
yyrule16: // [*/%]
{
                   lval.str = lexer.token(); return MULT_OP
goto yystate0
}
yyrule17: // {D}+{U}
{
                 lval.str = lexer.token(); return DURATION
goto yystate0
}
yyrule18: // {L}({L}|{D})*
{
           lval.str = lexer.token(); return IDENTIFIER
goto yystate0
}
yyrule19: // \-?{D}+(\.{D}*)?
{
        num, err := strconv.ParseFloat(lexer.token(), 64);
                         if (err != nil && err.(*strconv.NumError).Err == strconv.ErrSyntax) {
                           panic("Invalid float")
                         }
                         lval.num = clientmodel.SampleValue(num)
                         return NUMBER
}
yyrule20: // \"(\\.|[^\\"])*\"
{
       lval.str = lexer.token()[1:len(lexer.token()) - 1]; return STRING
goto yystate0
}
yyrule21: // \'(\\.|[^\\'])*\'
{
       lval.str = lexer.token()[1:len(lexer.token()) - 1]; return STRING
goto yystate0
}
yyrule22: // [{}\[\]()=,]
{
            return int(lexer.buf[0])
}
yyrule23: // [\t\n\r ]
{
               /* gobble up any whitespace */
goto yystate0
}
panic("unreachable")

goto yyabort // silence unused label error

yyabort: // no lexem recognized

  lexer.empty = true
  return int(c)
}
