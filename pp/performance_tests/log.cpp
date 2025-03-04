#include "log.h"

Logger logger_instance() {
  return Logger();
}

// NOLINTNEXTLINE(cppcoreguidelines-avoid-non-const-global-variables): static variable for main logger.
static Logger global_logger = logger_instance();

Logger::Logger() : should_be_quiet_(false) {
  std::cout.sync_with_stdio(false);
}

void Logger::configure(const Config& config) {
  if (config.get_value_of("quiet") == "true") {
    global_logger.should_be_quiet_ = true;
  }
}

void Logger::init(const Config& config) {
  global_logger.configure(config);
}

Logger& log() {
  return global_logger;
}
