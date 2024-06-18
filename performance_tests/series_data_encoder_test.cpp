#include "series_data_encoder_test.h"

#include <chrono>
#include <unordered_set>

#include "bare_bones/allocator.h"
#include "bare_bones/gorilla.h"
#include "performance_tests/dummy_wal.h"
#include "primitives/snug_composites.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"

namespace performance_tests {

using BareBones::Encoding::Gorilla::PrometheusStreamEncoder;
using BareBones::Encoding::Gorilla::StreamEncoder;
using BareBones::Encoding::Gorilla::TimestampDecoder;
using BareBones::Encoding::Gorilla::TimestampEncoder;
using BareBones::Encoding::Gorilla::ValuesEncoder;
using series_data::Decoder;
using series_data::encoder::Sample;
using series_data::encoder::SampleList;

struct PROMPP_ATTRIBUTE_PACKED OutdatedSample {
  int64_t timestamp;
  double value;
  uint32_t ls_id;

  OutdatedSample(int64_t _timestamp, double _value, uint32_t _ls_id) : timestamp(_timestamp), value(_value), ls_id(_ls_id) {}
};

SampleList get_encoded_samples(const series_data::DataStorage& data_storage, uint32_t ls_id) {
  SampleList result;
  if (auto it = data_storage.finalized_chunks.find(ls_id); it != data_storage.finalized_chunks.end()) {
    auto decoded_samples = Decoder::decode_chunks(data_storage, it->second, data_storage.open_chunks[ls_id]);
    for (auto& samples : decoded_samples) {
      result.reserve(result.size() + samples.size());
      std::copy(samples.begin(), samples.end(), std::back_inserter(result));
    }
  } else {
    result = Decoder::decode_chunk<series_data::chunk::DataChunk::Type::kOpen>(data_storage, data_storage.open_chunks[ls_id]);
  }
  return result;
}

void validate_encoded_chunks(const std::unordered_map<uint32_t, SampleList>& source_samples, const series_data::DataStorage& data_storage) {
  for (auto& [ls_id, expected_samples] : source_samples) {
    auto actual_samples = get_encoded_samples(data_storage, ls_id);
    if (!std::ranges::equal(expected_samples, actual_samples)) {
      std::cout << "Encoded samples for " << ls_id << " is not valid! type: " << static_cast<int>(data_storage.open_chunks[ls_id].encoding_type)
                << ", value: " << data_storage.open_chunks[ls_id].encoder.uint32_constant.value() << ", expected samples size: " << expected_samples.size()
                << ", actual samples size: " << actual_samples.size() << std::endl;

      auto samples_for_print = std::max(expected_samples.size(), actual_samples.size());
      for (size_t i = 0; i < samples_for_print; ++i) {
        auto& expected = expected_samples[i];
        auto& actual = actual_samples[i];
        std::cout << expected.timestamp << ":" << actual.timestamp << ", " << expected.value << ":" << actual.value << std::endl;
      }
    }
  }
}

void SeriesDataEncoder::execute(const Config& config, Metrics& metrics) const {
  DummyWal::Timeseries tmsr;
  DummyWal dummy_wal(input_file_full_name(config));

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap label_set_bitmap;
  series_data::DataStorage storage;
  series_data::OutdatedSampleEncoder outdated_sample_encoder{storage};
  series_data::Encoder encoder{storage, outdated_sample_encoder};
  std::chrono::nanoseconds encode_time{};
  size_t samples_count = 0;
  uint32_t outdated_samples_count = 0;
  std::unordered_map<uint32_t, SampleList> source_samples;
  BareBones::Vector<OutdatedSample> outdated_samples;

  const auto encode_outdated_samples = [&outdated_samples, &encoder]() {
    for (auto& sample : outdated_samples) {
      encoder.encode(sample.ls_id, sample.timestamp, sample.value);
    }

    outdated_samples.clear();
  };

  while (dummy_wal.read_next_segment()) {
    while (dummy_wal.read_next(tmsr)) {
      auto ls_id = label_set_bitmap.find_or_emplace(tmsr.label_set());
      auto& sample = tmsr.samples()[0];

      if (ls_id < 12000 && ++outdated_samples_count % 10 == 0 && false) {
        outdated_samples.emplace_back(sample.timestamp(), sample.value(), ls_id);
      } else {
        auto start_tm = std::chrono::steady_clock::now();
        encoder.encode(ls_id, sample.timestamp(), sample.value());
        encode_time += std::chrono::steady_clock::now() - start_tm;
      }

      source_samples[ls_id].emplace_back(Sample{.timestamp = sample.timestamp(), .value = sample.value()});

      ++samples_count;
    }

    if (outdated_samples.size() >= 10000) {
      encode_outdated_samples();
    }
  }

  encode_outdated_samples();

  series_data::OutdatedChunkMerger merger{storage, encoder};
  merger.merge();

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
  for (auto& chunk : storage.open_chunks) {
    ++chunks_info[static_cast<size_t>(chunk.encoding_type)].count;
    ++ls_id;
  }

  for (auto& [chunk_ls_id, chunks] : storage.finalized_chunks) {
    for (auto& chunk : chunks) {
      ++finalized_chunks_info[static_cast<size_t>(chunk.encoding_type)].count;
    }
  }

  const auto print_chunks_info = [&storage](std::string_view message, const auto& chunks_info) {
    std::cout << "==========================" << std::endl;
    std::cout << message << ":" << std::endl;
    for (auto& info : chunks_info) {
      if (info.type != series_data::chunk::DataChunk::EncodingType::kUnknown) {
        std::cout << info.name << "_count: " << info.count << ", allocated_memory: " << storage.allocated_memory(info.type) << std::endl;
      }
    }
    std::cout << "==========================" << std::endl << std::endl;
  };

  auto allocated_memory = storage.allocated_memory();
  std::cout << "allocated_memory: " << allocated_memory << std::endl;

  print_chunks_info("opened chunks", chunks_info);
  print_chunks_info("finalized chunks", finalized_chunks_info);

  metrics << (Metric() << "gorilla_prometheus_stream_encoder_allocated_memory" << allocated_memory);
  metrics << (Metric() << "gorilla_prometheus_stream_encoder_nanoseconds" << (encode_time.count() / (samples_count)));

  std::cout << "gorilla_prometheus_stream_encoder_nanoseconds: " << (encode_time.count() / (samples_count)) << std::endl;

  validate_encoded_chunks(source_samples, storage);
}

}  // namespace performance_tests