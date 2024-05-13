#include "gorilla_prometheus_stream_encoder_test.h"

#include <chrono>

#include "bare_bones/allocator.h"
#include "bare_bones/gorilla.h"
#include "performance_tests/dummy_wal.h"
#include "primitives/snug_composites.h"
#include "series_data/encoder/encoder.h"

namespace performance_tests {

using BareBones::Encoding::Gorilla::PrometheusStreamEncoder;
using BareBones::Encoding::Gorilla::StreamEncoder;
using BareBones::Encoding::Gorilla::TimestampDecoder;
using BareBones::Encoding::Gorilla::TimestampEncoder;
using BareBones::Encoding::Gorilla::ValuesEncoder;

void GorillaPrometheusStreamEncoder::execute(const Config& config, Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap label_set_bitmap;
  series_data::encoder::Encoder encoder;
  std::chrono::nanoseconds encode_time{};
  size_t samples_count = 0;
  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      auto ls_id = label_set_bitmap.find_or_emplace(tmsr.label_set());
      auto& sample = tmsr.samples()[0];

      auto start_tm = std::chrono::steady_clock::now();
      encoder.encode(ls_id, sample.timestamp(), sample.value());
      encode_time += std::chrono::steady_clock::now() - start_tm;

      ++samples_count;
    }
  }

  size_t constants_count = 0;
  size_t double_constants_count = 0;
  size_t two_double_constants_count = 0;
  size_t gorilla_count = 0;
  for (auto& gg : encoder.encoders_data()) {
    if (gg.values_encoding_type == series_data::encoder::value::EncodingType::kUint32Constant) {
      ++constants_count;
    } else if (gg.values_encoding_type == series_data::encoder::value::EncodingType::kDoubleConstant) {
      ++double_constants_count;
    } else if (gg.values_encoding_type == series_data::encoder::value::EncodingType::kTwoDoubleConstant) {
      ++two_double_constants_count;
    } else {
      ++gorilla_count;
    }
  }

  auto allocated_memory = encoder.allocated_memory();
  std::cout << "allocated_memory: " << allocated_memory << ", constants_count: " << constants_count << ", double_constants_count: " << double_constants_count
            << ", two_double_constants_count: " << two_double_constants_count << ", gorilla_count: " << gorilla_count << std::endl;

  metrics << (Metric() << "gorilla_prometheus_stream_encoder_allocated_memory" << allocated_memory);
  metrics << (Metric() << "gorilla_prometheus_stream_encoder_nanoseconds" << (encode_time.count() / (samples_count)));

  std::cout << "gorilla_prometheus_stream_encoder_nanoseconds: " << (encode_time.count() / (samples_count)) << std::endl;
}

}  // namespace performance_tests