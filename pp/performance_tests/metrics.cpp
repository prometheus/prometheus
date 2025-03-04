#include "metrics.h"

Metrics::Metrics(const Config& config) : should_be_quiet_(config.get_value_of("quiet") == "true") {
  if (!should_be_quiet_) {
    output_.open(config.get_value_of("metrics_file_full_name"), std::ios::app);
    if (!output_.is_open()) {
      throw std::runtime_error("failed to open file '" + config.get_value_of("metrics_file_full_name") + "' with metrics");
    }
  }
}

Metrics& Metrics::operator<<(const Metric& metric) {
  if (!should_be_quiet_) {
    output_ << metric.name_ << "{";

    std::string labels_separator;
    for (const auto& label : metric.labels_) {
      output_ << labels_separator;
      output_ << label.Name << "=\"" << label.Value;
      labels_separator = "\", ";
    }
    output_ << "\"} " << metric.value_ << std::endl;
  }
  return *this;
}
