#pragma once

#include "selector.h"

namespace PromPP::Prometheus {

struct ReadHints {
  std::string func{};
  std::vector<std::string> grouping;
  int64_t step_ms{};
  int64_t start_ms{};
  int64_t end_ms{};
  int64_t range_ms{};
  bool by{};

  bool operator==(const ReadHints&) const noexcept = default;
};

struct Query {
  ReadHints hints{};
  LabelMatchers label_matchers;
  int64_t start_timestamp_ms{};
  int64_t end_timestamp_ms{};

  bool operator==(const Query&) const noexcept = default;
};

struct LabelValuesQuery {
  Query query{};
  std::string label_name;

  bool operator==(const LabelValuesQuery&) const noexcept = default;
};

}  // namespace PromPP::Prometheus