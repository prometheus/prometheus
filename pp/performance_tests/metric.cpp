#include "metric.h"

#include <stdexcept>

// NOLINTNEXTLINE(cppcoreguidelines-avoid-non-const-global-variables): static variable for all required metrics.
std::vector<Metric::Label> Metric::global_labels_;

void Metric::define_global_label(const Metric::Label& label) {
  global_labels_.push_back(label);
}

Metric& Metric::operator<<(const std::string& name) {
  name_ = name;

  return *this;
}

Metric& Metric::operator<<(const Label& label) {
  if (name_.empty()) {
    throw std::logic_error("metric name should be defined before labels");
  }

  labels_.push_back(label);

  return *this;
}

Metric& Metric::operator<<(double value) {
  if (name_.empty()) {
    throw std::logic_error("metric name and labels should be defined before value");
  }

  if (labels_.empty() && global_labels_.empty()) {
    throw std::logic_error("metric labels should be defined before values");
  }

  for (const auto& label : global_labels_) {
    labels_.push_back(label);
  }

  value_ = value;

  return *this;
}
