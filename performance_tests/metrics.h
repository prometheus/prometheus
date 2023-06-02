#pragma once

#include <fstream>

#include "config.h"
#include "metric.h"

class Metrics {
  std::ofstream output_;
  bool should_be_quiet_;

 public:
  Metrics(const Config& config);
  Metrics(const Metrics&) = delete;
  Metrics& operator=(const Metrics&) = delete;
  Metrics(Metrics&&) = delete;
  Metrics& operator=(Metrics&&) = delete;

  Metrics& operator<<(const Metric& metric);
};
