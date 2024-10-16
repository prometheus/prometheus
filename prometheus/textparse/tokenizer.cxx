#include "tokenizer.h"

/*!conditions:re2c*/

namespace PromPP::Prometheus::textparse {

Tokenizer::Tokenizer() : condition_{yycinit} {}

Tokenizer::Tokenizer(std::string_view str)
    : cursor_ptr_(str.data()), limit_ptr_(str.data() + str.size()), marker_ptr_(str.data()), token_ptr_(str.data()),
      condition_{yycinit} {}

void Tokenizer::tokenize(std::string_view str) noexcept {
  cursor_ptr_ = str.data();
  limit_ptr_ = str.data() + str.size();
  marker_ptr_ = str.data();
  token_ptr_ = str.data();

  condition_ = yycinit;
}

Tokenizer::Token Tokenizer::consume_comment() noexcept {
  do {
    if (*cursor_ptr_ == '\n') {
      condition_ = yycinit;
      return Token::kComment;
    }
  } while (++cursor_ptr_ != limit_ptr_);

  return Token::kEOF;
}

Tokenizer::Token Tokenizer::next_impl() noexcept {
  token_ptr_ = cursor_ptr_;

  /*!re2c
      re2c:api:style = free-form;
      re2c:define:YYCTYPE = char;
      re2c:define:YYCURSOR = cursor_ptr_;
      re2c:define:YYLIMIT = limit_ptr_;
      re2c:define:YYMARKER = marker_ptr_;
      re2c:define:YYFILL = "{if (limit_ptr_ >= cursor_ptr_) { return Token::kEOF; } }";
      re2c:define:YYGETCONDITION = "condition_";
      re2c:define:YYSETCONDITION = "condition_ = @@;";

      DIGIT = [0-9];
      LETTER = [a-zA-Z_];
      METRIC_NAME_CHAR = [a-zA-Z_:];
      METRIC_NAME = METRIC_NAME_CHAR(METRIC_NAME_CHAR|DIGIT)*;
      LABEL_NAME = LETTER(LETTER|DIGIT)*;
      CHAR = .;
      SPACE = [ \t];

      QUOTED_STRING = ["] ([\\]. | [^\\"])* ["];

      <init> [\x00] {
        return Token::kEOF;
      }
      <init> [\n] => init {
        return Token::kLinebreak;
      }
      <*> SPACE+ {
        return Token::kWhitespace;
      }
      <init> [#] {
        token_ptr_ = cursor_ptr_;
        return consume_comment();
      }
      <init> [{] => labels {
        return Token::kBraceOpen;
      }

      // meta
      <init> [#]SPACE+ => comment {
        token_ptr_ = cursor_ptr_;
      }
      <comment>"HELP" SPACE+ => meta_name {
        return Token::kHelp;
      }
      <comment>"TYPE" SPACE+ => meta_name {
        return Token::kType;
      }
      <comment>"" => init {
        return consume_comment();
      }
      <meta_name> METRIC_NAME => meta_text_with_leading_spaces {
        return Token::kMetricName;
      }
      <meta_name> QUOTED_STRING => meta_text_with_leading_spaces {
        return Token::kMetricName;
      }
      <meta_text_with_leading_spaces> SPACE* => meta_text {
        token_ptr_ = cursor_ptr_;
      }
      <meta_text> CHAR* => init {
        return Token::kText;
      }

      // metrics
      <init> METRIC_NAME => value {
        return Token::kMetricName;
      }
      <labels> LABEL_NAME {
        return Token::kLabelName;
      }
      <labels> [=] => label_value {
        return Token::kEqual;
      }
      <labels> [,] {
        return Token::kComma;
      }
      <labels> QUOTED_STRING {
        return Token::kQuotedString;
      }
      <label_value> QUOTED_STRING => labels {
        return Token::kLabelValue;
      }
      <labels> [}] => value {
        return Token::kBraceClose;
      }

      // value
      <value> [{] => labels {
        return Token::kBraceOpen;
      }
      <value> [^{ \t\n]+ => timestamp {
        return Token::kValue;
      }
      <timestamp> DIGIT+ {
        return Token::kTimestamp;
      }
      <timestamp> [\n] => init {
        return Token::kLinebreak;
      }

      <label_value, value, timestamp, labels, init, meta_name, meta_text_with_leading_spaces>"" {
        return Token::kInvalid;
      }
  */
}

} // namespace PromPP::Prometheus::textparse
