#pragma once

#include "bare_bones/preprocess.h"
#include "remote_write.h"
#include "third_party/protozero/pbf_writer.hpp"

namespace PromPP::Prometheus::RemoteWrite {

template <class OutputBuffer>
class ProtozeroEncoder {
 public:
  explicit ProtozeroEncoder(OutputBuffer& output_buffer) : pb_message_(output_buffer) {}

  void PROMPP_ALWAYS_INLINE operator()(auto& timeseries) { PromPP::Prometheus::RemoteWrite::write_timeseries(pb_message_, timeseries); }

 private:
  protozero::basic_pbf_writer<OutputBuffer> pb_message_;
};

}  // namespace PromPP::Prometheus::RemoteWrite
