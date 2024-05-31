#include "gorilla_prometheus_stream_encoder_test.h"

#include <chrono>
#include <unordered_set>

#include "bare_bones/allocator.h"
#include "bare_bones/gorilla.h"
#include "performance_tests/dummy_wal.h"
#include "primitives/snug_composites.h"
#include "series_data/encoder.h"

namespace performance_tests {

using BareBones::Encoding::Gorilla::PrometheusStreamEncoder;
using BareBones::Encoding::Gorilla::StreamEncoder;
using BareBones::Encoding::Gorilla::TimestampDecoder;
using BareBones::Encoding::Gorilla::TimestampEncoder;
using BareBones::Encoding::Gorilla::ValuesEncoder;

struct Sample {
  int64_t timestamp;
  double value;

  Sample(int64_t ts, double v) : timestamp(ts), value(v) {}

  bool operator==(const Sample& other) const noexcept {
    return timestamp == other.timestamp && std::bit_cast<uint64_t>(value) == std::bit_cast<uint64_t>(other.value);
  }
};

struct PROMPP_ATTRIBUTE_PACKED OutdatedSample {
  int64_t timestamp;
  double value;
  uint32_t ls_id;

  OutdatedSample(int64_t _timestamp, double _value, uint32_t _ls_id) : timestamp(_timestamp), value(_value), ls_id(_ls_id) {}
};

void GorillaPrometheusStreamEncoder::execute(const Config& config, Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap label_set_bitmap;
  series_data::Encoder encoder;
  std::chrono::nanoseconds encode_time{};
  size_t samples_count = 0;
  uint32_t outdated_samples_count = 0;
  [[maybe_unused]] std::unordered_map<uint32_t, std::vector<Sample>> source_samples;
  BareBones::Vector<OutdatedSample> outdated_samples;
#ifdef DUMP_LABELS
  std::unordered_set<std::string> names_set;
#endif
  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      auto ls_id = label_set_bitmap.find_or_emplace(tmsr.label_set());
      auto& sample = tmsr.samples()[0];

#if 0
      source_samples[ls_id].emplace_back(sample.timestamp(), sample.value());
#endif

#ifdef DUMP_LABELS
      auto lss = label_set_bitmap[ls_id];
      for (auto it = lss.begin(); it != lss.end(); ++it) {
        names_set.emplace((*it).second);
      }
#endif

#if 0
      double ddd = std::numeric_limits<uint32_t>::max();
      std::cout << ddd << std::endl;

      uint64_t ggg = std::bit_cast<uint64_t>(ddd);
      std::cout << "ok: " << (ggg <= std::numeric_limits<uint32_t>::max()) << std::endl;

      uint8_t* bytes = reinterpret_cast<uint8_t*>(&ggg);
      for (int i = 0; i < 8; ++i) {
        printf("0x%.2X, ", static_cast<unsigned int>(bytes[i]));
      }
      printf("\n");

      int64_t ggg2 = -1;
      bytes = reinterpret_cast<uint8_t*>(&ggg2);
      for (int i = 0; i < 8; ++i) {
        printf("0x%.2X, ", static_cast<unsigned int>(bytes[i]));
      }
      printf("\n");

      std::exit(0);
#endif

      if (ls_id < 12000 && ++outdated_samples_count % 10 == 0 && false) {
        outdated_samples.emplace_back(sample.timestamp(), sample.value(), ls_id);
      } else {
        auto start_tm = std::chrono::steady_clock::now();
        encoder.encode(ls_id, sample.timestamp(), sample.value());
        encode_time += std::chrono::steady_clock::now() - start_tm;
      }

      ++samples_count;
    }

    //    for (auto& sample : outdated_samples) {
    //      encoder.encode(sample.ls_id, sample.timestamp, sample.value);
    //    }
  }

#ifdef DUMP_LABELS
  for (auto n : names_set) {
    std::cout << n << std::endl;
  }
  std::exit(0);
#endif

  struct ChunkInfo {
    series_data::chunk::DataChunk::EncodingType type;
    std::string_view name;
    uint32_t count{};
  };
  std::array chunks_info = {
      ChunkInfo{.type = series_data::chunk::DataChunk::EncodingType::kUnknown, .name = "unknown"},
      ChunkInfo{.type = series_data::chunk::DataChunk::EncodingType::kUint32Constant, .name = "uint32_constants"},
      ChunkInfo{.type = series_data::chunk::DataChunk::EncodingType::kDoubleConstant, .name = "double_constants"},
      ChunkInfo{.type = series_data::chunk::DataChunk::EncodingType::kTwoDoubleConstant, .name = "two_double_constants"},
      ChunkInfo{.type = series_data::chunk::DataChunk::EncodingType::kAscIntegerValuesGorilla, .name = "asc_integer_values_gorilla"},
      ChunkInfo{.type = series_data::chunk::DataChunk::EncodingType::kValuesGorilla, .name = "values_gorilla"},
      ChunkInfo{.type = series_data::chunk::DataChunk::EncodingType::kGorilla, .name = "gorilla"},
  };
  auto finalized_chunks_info = chunks_info;

  [[maybe_unused]] uint32_t ls_id = 0;
  for (auto& chunk : encoder.storage().open_chunks) {
    ++chunks_info[static_cast<size_t>(chunk.encoding_type)].count;
    ++ls_id;
  }

  for (auto& [chunk_ls_id, chunks] : encoder.storage().finalized_chunks) {
    for (auto& chunk : chunks) {
      ++finalized_chunks_info[static_cast<size_t>(chunk.encoding_type)].count;
    }
  }

  const auto print_chunks_info = [&encoder](std::string_view message, const auto& chunks_info) {
    std::cout << "==========================" << std::endl;
    std::cout << message << ":" << std::endl;
    for (auto& info : chunks_info) {
      if (info.type != series_data::chunk::DataChunk::EncodingType::kUnknown) {
        std::cout << info.name << "_count: " << info.count << ", allocated_memory: " << encoder.allocated_memory(info.type) << std::endl;
      }
    }
    std::cout << "==========================" << std::endl << std::endl;
  };

  auto allocated_memory = encoder.allocated_memory();
  std::cout << "allocated_memory: " << allocated_memory << std::endl;

  print_chunks_info("opened chunks", chunks_info);
  print_chunks_info("finalized chunks", finalized_chunks_info);

  metrics << (Metric() << "gorilla_prometheus_stream_encoder_allocated_memory" << allocated_memory);
  metrics << (Metric() << "gorilla_prometheus_stream_encoder_nanoseconds" << (encode_time.count() / (samples_count)));

  std::cout << "gorilla_prometheus_stream_encoder_nanoseconds: " << (encode_time.count() / (samples_count)) << std::endl;
}

}  // namespace performance_tests