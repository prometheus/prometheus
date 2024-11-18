#pragma once

#include <fastfloat/fast_float.h>

#include "primitives/primitives.h"
#include "prometheus/textparse/open_metrics/tokenizer.h"
#include "prometheus/textparse/prometheus/tokenizer.h"

namespace PromPP::WAL::hashdex::scraper {

enum class Error : uint32_t {
  kNoError = 0,
  kUnexpectedToken,
  kNoMetricName,
  kInvalidUtf8,
  kInvalidValue,
  kInvalidTimestamp,
};

template <class Value>
[[nodiscard]] bool parse_numeric_value(std::string_view str, Value& value) noexcept {
  if (str.front() == '+') {
    str.remove_prefix(1);
  }

  if (const auto [ptr, ec] = fast_float::from_chars(str.begin(), str.end(), value); ec != std::errc{} || ptr != str.end()) [[unlikely]] {
    return false;
  }

  return true;
}

template <class Parser>
concept ParserInterface = requires(Parser& parser, const Parser& const_parser, Primitives::Timestamp& timestamp) {
  { parser.tokenizer() };
  { Prometheus::textparse::TokenizerInterface<decltype(parser.tokenizer())> };
  { Prometheus::textparse::TokenizerInterface<decltype(const_parser.tokenizer())> };

  { const_parser.is_value_token() } -> std::same_as<bool>;
  { parser.parse_timestamp(timestamp) } -> std::same_as<Error>;
  { const_parser.validate_parse_result() } -> std::same_as<Error>;
  { const_parser.validate_parse_sample_result() } -> std::same_as<Error>;
};

class PrometheusParser {
 public:
  using Tokenizer = Prometheus::textparse::Prometheus::Tokenizer;

  [[nodiscard]] PROMPP_ALWAYS_INLINE Tokenizer& tokenizer() noexcept { return tokenizer_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Tokenizer& tokenizer() const noexcept { return tokenizer_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_value_token() const noexcept {
    return tokenizer_.token() == Token::kValue || (tokenizer_.token() == Token::kEOF && !tokenizer_.token_str().empty());
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Error validate_parse_result() const noexcept {
    return tokenizer_.token_str().empty() ? Error::kNoError : Error::kUnexpectedToken;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Error validate_parse_sample_result() const noexcept {
    return (tokenizer_.token() == Token::kLinebreak || tokenizer_.token() == Token::kEOF) ? Error::kNoError : Error::kUnexpectedToken;
  }

  [[nodiscard]] Error parse_timestamp(Primitives::Timestamp& timestamp) noexcept {
    if (is_timestamp_token()) {
      if (!parse_numeric_value(tokenizer_.token_str(), timestamp)) [[unlikely]] {
        return Error::kInvalidTimestamp;
      }

      tokenizer_.next_non_whitespace();
    }

    return Error::kNoError;
  }

 private:
  using Token = Prometheus::textparse::Token;

  Tokenizer tokenizer_;

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_timestamp_token() const noexcept {
    return tokenizer_.token() == Token::kTimestamp || (tokenizer_.token() == Token::kEOF && !tokenizer_.token_str().empty());
  }
};

class OpenMetricsParser {
 public:
  using Tokenizer = Prometheus::textparse::OpenMetrics::Tokenizer;

  [[nodiscard]] PROMPP_ALWAYS_INLINE Tokenizer& tokenizer() noexcept { return tokenizer_; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE const Tokenizer& tokenizer() const noexcept { return tokenizer_; }

  [[nodiscard]] PROMPP_ALWAYS_INLINE bool is_value_token() const noexcept { return tokenizer_.token() == Token::kValue; }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Error validate_parse_result() const noexcept {
    return tokenizer_.token() == Token::kEOFWord ? Error::kNoError : Error::kUnexpectedToken;
  }
  [[nodiscard]] PROMPP_ALWAYS_INLINE Error validate_parse_sample_result() const noexcept {
    return (tokenizer_.token() == Token::kLinebreak || tokenizer_.token() == Token::kExemplar) ? Error::kNoError : Error::kUnexpectedToken;
  }

  [[nodiscard]] Error parse_timestamp(Primitives::Timestamp& timestamp) noexcept {
    if (tokenizer_.token() == Token::kTimestamp) {
      if (double float_timestamp; parse_timestamp_as_float(float_timestamp)) [[likely]] {
        timestamp = static_cast<int64_t>(float_timestamp * 1000.0);
      } else {
        return Error::kInvalidTimestamp;
      }

      tokenizer_.next_non_whitespace();
    }

    return Error::kNoError;
  }

 private:
  using Token = Prometheus::textparse::Token;

  Tokenizer tokenizer_;

  [[nodiscard]] bool parse_timestamp_as_float(double& float_timestamp) const noexcept {
    if (!parse_numeric_value(tokenizer_.token_str(), float_timestamp)) [[unlikely]] {
      return false;
    }

    if (std::isnan(float_timestamp) || std::isinf(float_timestamp)) [[unlikely]] {
      return false;
    }

    return true;
  }
};

static_assert(ParserInterface<PrometheusParser>);
static_assert(ParserInterface<OpenMetricsParser>);

}  // namespace PromPP::WAL::hashdex::scraper