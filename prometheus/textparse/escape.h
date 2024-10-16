#pragma once

#include <string_view>

#include "bare_bones/preprocess.h"

namespace PromPP::Prometheus::textparse {

PROMPP_ALWAYS_INLINE constexpr void unquote(std::string_view& string) noexcept {
  if (string.length() > 1 && string.front() == '"' && string.back() == '"') {
    string.remove_prefix(1);
    string.remove_suffix(1);
  }
}

template <class StringPieceHandler>
  requires std::is_invocable_v<StringPieceHandler, std::string_view>
constexpr void unescape_label_value(std::string_view label_value, StringPieceHandler handle_string_piece) noexcept {
  static constexpr auto is_escaped_char = [](char ch) PROMPP_LAMBDA_INLINE { return ch == '"' || ch == '\\' || ch == 'n'; };

  const auto handle_string_piece_with_result = [&handle_string_piece](std::string_view string_piece) PROMPP_LAMBDA_INLINE {
    if constexpr (std::is_void_v<decltype(handle_string_piece(string_piece))>) {
      handle_string_piece(string_piece);
      return true;
    } else {
      return handle_string_piece(string_piece);
    }
  };

  for (size_t pos = 0; (pos = label_value.find('\\', pos)) != std::string_view::npos;) {
    if (pos + 1 == label_value.size()) [[unlikely]] {
      break;
    }

    const auto next_char = label_value[pos + 1];
    if (!is_escaped_char(next_char)) {
      ++pos;
      continue;
    }

    if (pos > 0) [[likely]] {
      if (!handle_string_piece_with_result(label_value.substr(0, pos))) {
        return;
      }
    }

    if (next_char == 'n') {
      ++pos;
      if (!handle_string_piece_with_result("\n")) {
        return;
      }
    }

    label_value.remove_prefix(pos + 1);
    pos = next_char == '\\' ? 1 : 0;
  }

  if (!label_value.empty()) {
    handle_string_piece(label_value);
  }
}

}  // namespace PromPP::Prometheus::textparse