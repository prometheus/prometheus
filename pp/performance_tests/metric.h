#pragma once

#include <string>
#include <vector>

class Metric {
 public:
  struct Label;

 private:
  std::string name_;
  std::vector<Label> labels_;
  static std::vector<Label> global_labels_;  // NOLINT(cppcoreguidelines-avoid-non-const-global-variables)
  double value_ = 0.0;

  friend class Metrics;

 public:
  struct Label {
    std::string Name;
    std::string Value;
  };

  Metric() = default;
  Metric(const Metric&) = delete;
  Metric& operator=(const Metric&) = delete;
  Metric(Metric&&) = delete;
  Metric& operator=(Metric&&) = delete;
  ~Metric() = default;

  static void define_global_label(const Label& label);

  Metric& operator<<(const std::string& name);
  Metric& operator<<(const Label& label);
  Metric& operator<<(double value);
};
