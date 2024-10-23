#include <gtest/gtest.h>

#include "series_index/queryable_encoding_bimap.h"
#include "series_index/trie/cedarpp_tree.h"
#include "wal/decoder.h"
#include "wal/encoder.h"
#include "wal/wal.h"

namespace {
using TrieIndex = series_index::TrieIndex<series_index::trie::CedarTrie, series_index::trie::CedarMatchesList>;
using QueryableEncodingBimap = series_index::QueryableEncodingBimap<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimapFilament, TrieIndex>;
using BasicEncoder = PromPP::WAL::BasicEncoder<QueryableEncodingBimap&>;
using Encoder = PromPP::WAL::GenericEncoder<BasicEncoder>;
using LabelSet = PromPP::Primitives::LabelSet;
using Label = PromPP::Primitives::Label;
using Sample = PromPP::Primitives::Sample;
using InnerSeries = PromPP::Prometheus::Relabel::InnerSeries;
using InnerSeriesPtr = PromPP::Prometheus::Relabel::InnerSeries*;
using InnerSeriesPtrSlice = PromPP::Primitives::Go::Slice<InnerSeriesPtr>;
using Decoder = PromPP::WAL::GenericDecoder<QueryableEncodingBimap&>;

struct EncodingResult {
  int64_t earliest_timestamp;
  int64_t latest_timestamp;
  size_t allocated_memory;
  uint32_t series;
  uint32_t samples;
  uint32_t remainder_size;
  PromPP::Primitives::Go::Slice<char> error;
};

struct DecodingResult {};

Encoder create_encoder(Decoder& decoder, QueryableEncodingBimap& lss, uint16_t shard_id, uint8_t log_shards, size_t iteration) {
  if (iteration == 0) {
    return Encoder(shard_id, log_shards, decoder.gorilla(), lss, shard_id, log_shards);
  }

  return PromPP::WAL::create_encoder_from_decoder<Encoder, Decoder>(decoder);
}

void write_to_buffer(std::ostringstream& data, std::string& buffer) {
  auto output_str = data.str();
  auto output_str_size = std::to_string(output_str.size());
  std::cout << "segment size: " << output_str_size << std::endl;
  buffer.append(output_str_size.data(), output_str_size.size());
  buffer.append(output_str.data(), output_str.size());
  std::cout << "buffer size: " << buffer.size() << std::endl;
}

struct Codec : public testing::Test {};

void make_iteration(std::string& buffer, size_t iteration) {
  QueryableEncodingBimap lss{};

  Decoder decoder{lss, BasicEncoder::version};
  DecodingResult decoding_result;
  InnerSeries inner_series{};
  std::istringstream reader{buffer};
  uint32_t segment_id{0};
  while (!reader.eof()) {
    size_t size{0};
    reader >> size;
    std::cout << "reader >> size: " << size << std::endl;
    if (size == 0) {
      break;
    }
    std::cout << "segment id: " << segment_id << std::endl;
    std::string segment{};
    segment.resize(size);
    reader.read(segment.data(), segment.size());
    for (auto b : segment) {
      printf("%.2X ", static_cast<unsigned char>(b));
    }
    std::cout << std::endl;
    inner_series.clear();
    PromPP::Primitives::Go::SliceView<char> segment_view{};
    segment_view.reset_to(segment.data(), segment.size());
    decoder.decode_to_inner_series(segment_view, inner_series, &decoding_result);
    segment_id++;
  }
  inner_series.clear();

  uint16_t shard_id{0};
  uint8_t log_shards{2};
  Encoder encoder = create_encoder(decoder, lss, shard_id, log_shards, iteration);

  std::ostringstream output{};

  LabelSet ls;
  ls.add(Label{"__name__", "test_metric"});
  ls.add(Label{"lol", "kek"});
  uint32_t ls_id = lss.find_or_emplace(ls);
  InnerSeriesPtrSlice inner_series_slice{};
  BareBones::Vector<Sample> samples{};

  EncodingResult encoding_result;

  samples.emplace_back(0 + 3 * iteration, 1 + 3 * iteration);
  inner_series.emplace_back(samples, ls_id);
  inner_series_slice.emplace_back(&inner_series);
  encoder.add_inner_series(inner_series_slice, &encoding_result);

  ASSERT_EQ(encoding_result.samples, 1);
  ASSERT_EQ(encoding_result.series, 1);
  ASSERT_EQ(encoding_result.earliest_timestamp, 0 + 3 * iteration);
  ASSERT_EQ(encoding_result.latest_timestamp, 0 + 3 * iteration);

  encoder.finalize(&encoding_result, output);
  if (iteration == 1) {
    std::cout << "problem here" << std::endl;
  }
  for (auto b : output.str()) {
    printf("%.2X ", static_cast<unsigned char>(b));
  }
  std::cout << std::endl;
  write_to_buffer(output, buffer);
  output.str("");
  output.clear();

  samples.resize(0);
  inner_series.clear();
  inner_series_slice.clear();

  samples.emplace_back(1 + 3 * iteration, 2 + 3 * iteration);
  inner_series.emplace_back(samples, ls_id);
  inner_series_slice.emplace_back(&inner_series);
  encoder.add_inner_series(inner_series_slice, &encoding_result);

  ASSERT_EQ(encoding_result.samples, 1);
  ASSERT_EQ(encoding_result.series, 1);
  ASSERT_EQ(encoding_result.earliest_timestamp, 1 + 3 * iteration);
  ASSERT_EQ(encoding_result.latest_timestamp, 1 + 3 * iteration);

  encoder.finalize(&encoding_result, output);
  write_to_buffer(output, buffer);
  output.str("");
  output.clear();

  samples.resize(0);
  inner_series.clear();
  inner_series_slice.clear();

  samples.emplace_back(2 + 3 * iteration, 3 + 3 * iteration);
  inner_series.emplace_back(samples, ls_id);
  inner_series_slice.emplace_back(&inner_series);
  encoder.add_inner_series(inner_series_slice, &encoding_result);

  ASSERT_EQ(encoding_result.samples, 1);
  ASSERT_EQ(encoding_result.series, 1);
  ASSERT_EQ(encoding_result.earliest_timestamp, 2 + 3 * iteration);
  ASSERT_EQ(encoding_result.latest_timestamp, 2 + 3 * iteration);

  encoder.finalize(&encoding_result, output);
  write_to_buffer(output, buffer);
  output.str("");
  output.clear();
}

TEST_F(Codec, CODECTEST) {
  std::string buffer{};
  make_iteration(buffer, 0);
  std::cout << "iteration: " << 0 << ", buffer size: " << buffer.size() << std::endl;
  std::cout << "============================================" << std::endl;
  std::cout << "============================================" << std::endl;
  std::cout << "============================================" << std::endl;
  make_iteration(buffer, 1);
  std::cout << "iteration: " << 1 << ", buffer size: " << buffer.size() << std::endl;
  std::cout << "============================================" << std::endl;
  std::cout << "============================================" << std::endl;
  std::cout << "============================================" << std::endl;
  make_iteration(buffer, 2);
  std::cout << "iteration: " << 2 << ", buffer size: " << buffer.size() << std::endl;
}
}  // namespace