#include "tokenizer.h"

#include <cstring>

// NOLINTBEGIN
/*!conditions:re2c*/
// NOLINTEND

namespace PromPP::Prometheus::textparse {

Tokenizer::Tokenizer() : condition_{yycinit} {}

Tokenizer::Tokenizer(std::string_view str)
    : start_ptr_(str.data()), cursor_ptr_(start_ptr_), limit_ptr_(start_ptr_ + str.size()), marker_ptr_(start_ptr_), token_ptr_(start_ptr_),
      condition_{yycinit} {}

void Tokenizer::tokenize(std::string_view str) noexcept {
  start_ptr_ = str.data();
  cursor_ptr_ = start_ptr_;
  limit_ptr_ = start_ptr_ + str.size();
  marker_ptr_ = start_ptr_;
  token_ptr_ = start_ptr_;

  condition_ = yycinit;
}

Tokenizer::Token Tokenizer::consume_comment(Token token) noexcept {
  if (cursor_ptr_ = static_cast<const char*>(std::memchr(cursor_ptr_, '\n', limit_ptr_ - cursor_ptr_)); cursor_ptr_ != nullptr) [[likely]] {
    condition_ = yycinit;
    return token;
  }

  cursor_ptr_ = limit_ptr_;
  return Token::kEOF;
}

Tokenizer::Token Tokenizer::consume_escaped_string(Token token) noexcept {
  while (true) {
    if (cursor_ptr_ = static_cast<const char*>(std::memchr(cursor_ptr_, '"', limit_ptr_ - cursor_ptr_)); cursor_ptr_ == nullptr) [[unlikely]] {
      cursor_ptr_ = limit_ptr_;
      return Token::kInvalid;
    }

    if (cursor_ptr_[-1] == '\\') [[unlikely]] {
      ++cursor_ptr_;
      continue;
    }

    break;
  }

  ++cursor_ptr_;
  return token;
}

// NOLINTBEGIN
Tokenizer::Token Tokenizer::next_impl() noexcept {
  token_ptr_ = cursor_ptr_;

  /*!re2c
      re2c:api:style = free-form;
      re2c:define:YYCTYPE = "unsigned char";
      re2c:define:YYCURSOR = cursor_ptr_;
      re2c:define:YYLIMIT = limit_ptr_;
      re2c:define:YYMARKER = marker_ptr_;
      re2c:define:YYFILL = "{if (limit_ptr_ == cursor_ptr_) { return Token::kEOF; } }";
      re2c:define:YYGETCONDITION = "condition_";
      re2c:define:YYSETCONDITION = "condition_ = @@;";

      DIGIT = [0-9];
      LETTER = [a-zA-Z_];
      METRIC_NAME_CHAR = [a-zA-Z_:];
      METRIC_NAME = METRIC_NAME_CHAR(METRIC_NAME_CHAR|DIGIT)*;
      LABEL_NAME = LETTER(LETTER|DIGIT)*;
      SPACE = [ \t];

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
      <meta_name> ["] => meta_text_with_leading_spaces {
        return consume_escaped_string(Token::kMetricName);
      }
      <meta_text_with_leading_spaces> SPACE* => init {
        token_ptr_ = cursor_ptr_;
        return consume_comment(Token::kText);
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
      <labels> ["] {
        return consume_escaped_string(Token::kQuotedString);
      }
      <label_value> ["] => labels {
        return consume_escaped_string(Token::kLabelValue);
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
// NOLINTEND

} // namespace PromPP::Prometheus::textparse
