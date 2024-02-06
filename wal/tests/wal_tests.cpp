#include <gtest/gtest.h>

#include "wal/wal.h"

#include "gtest/gtest.h"

namespace {

class SampleForTest {
  PromPP::Primitives::Timestamp timestamp_;
  double value_;

 public:
  PromPP::Primitives::Timestamp timestamp() const { return timestamp_; }
  PromPP::Primitives::Timestamp timestamp() { return timestamp_; }

  double value() const { return value_; }
  double& value() { return value_; }

  SampleForTest(const SampleForTest& s) : timestamp_(s.timestamp()), value_(s.value()) {}
  SampleForTest(const PromPP::Primitives::Timestamp ts, const double val) : timestamp_(ts), value_(val) {}
  SampleForTest() = default;
};

class NamesSetForTest : public std::vector<std::string> {
  using Base = std::vector<std::string>;

 public:
  using Base::Base;

  friend size_t hash_value(const NamesSetForTest& lns) {
    size_t res = 0;
    for (const auto& label_name : lns) {
      res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res);
    }
    return res;
  }
};

class LabelSetForTest : public std::vector<std::pair<std::string, std::string>> {
  using Base = std::vector<std::pair<std::string, std::string>>;

 public:
  using Base::Base;

  NamesSetForTest names() const {
    NamesSetForTest tns;

    for (auto [label_name, _] : *this) {
      tns.push_back(label_name);
    }

    return tns;
  }
};

class TimeSeriesForTest {
 public:
  using PrimaryLS =
      PromPP::Primitives::SnugComposites::Filaments::LabelSet<BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol>,
                                                            BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::LabelNameSet<
                                                                BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol>>>>;
  using PrimaryCompositeType = PrimaryLS::composite_type;

 private:
  PrimaryCompositeType label_set_;
  std::vector<SampleForTest> samples_;
  PrimaryLS::data_type data_;

 public:
  TimeSeriesForTest() = default;
  TimeSeriesForTest(const LabelSetForTest& label_set, const std::vector<SampleForTest>& samples) : samples_(samples) {
    label_set_ = PrimaryLS(data_, label_set).composite(data_);
  }

  const auto& label_set() const { return label_set_; }
  auto& label_set() { return label_set_; }

  const auto& samples() const { return samples_; }
  auto& samples() { return samples_; }

  const auto& data() const { return data_; }
  auto& data() { return data_; }
};

using samples_sequence_type = std::vector<SampleForTest>;
const size_t NUM_VALUES = 10;
const uint64_t START_TS = 1660828400000;
const char SYMBOLS_DATA[89] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-+=/|.,\\?<>!@#$%^&*()\"':;";

samples_sequence_type generate_samples(uint64_t ts_step, double v) {
  samples_sequence_type samples;

  for (size_t i = 0; i < NUM_VALUES; ++i) {
    samples.push_back({
        static_cast<PromPP::Primitives::Timestamp>(START_TS + (ts_step * (i + 1))),
        v * (i + 1),
    });
  }

  return samples;
};

std::vector<std::pair<uint32_t, samples_sequence_type>> generate_ids_and_samples() {
  std::vector<std::pair<uint32_t, samples_sequence_type>> idssmpls;

  for (size_t i = 0; i < NUM_VALUES; ++i) {
    idssmpls.push_back({
        i + 1,
        generate_samples((i + 1) * 1000, (i + 1) * 1.123),
    });
  }

  return idssmpls;
};

std::string generate_str(int seed) {
  std::mt19937 gen32(seed);
  std::string word;
  int maxlen = 4 + (gen32() % 28);

  for (int i = 0; i < maxlen; i++) {
    word += SYMBOLS_DATA[gen32() % 89];
  }

  return word;
};

LabelSetForTest generate_label_set() {
  LabelSetForTest lst;

  for (size_t i = 0; i < NUM_VALUES; ++i) {
    lst.push_back({generate_str(i), generate_str(i + NUM_VALUES)});
  }

  return lst;
};

struct Wal : public testing::Test {};

TEST_F(Wal, Buffer) {
  PromPP::WAL::BasicEncoder<PromPP::Primitives::SnugComposites::Symbol::EncodingBimap>::Buffer buf;

  std::vector<std::pair<uint32_t, std::pair<uint64_t, double>>> etalons;
  std::vector<std::pair<uint32_t, std::pair<uint64_t, double>>> outcomes;
  uint64_t latest_sample = 0;

  for (const auto& [series_id, samples] : generate_ids_and_samples()) {
    for (const auto& sample : samples) {
      buf.add(series_id, PromPP::Primitives::Sample(sample.timestamp(), sample.value()));
      etalons.push_back({series_id, {sample.timestamp(), sample.value()}});
      latest_sample = sample.timestamp();
    }
  }

  EXPECT_EQ(buf.latest_sample(), latest_sample);
  EXPECT_EQ(buf.earliest_sample(), START_TS + 1000);
  EXPECT_EQ(buf.samples_count(), NUM_VALUES * NUM_VALUES);
  EXPECT_EQ(buf.series_count(), NUM_VALUES);

  buf.for_each([&](uint32_t ls_id, uint64_t ts, double v) { outcomes.push_back({ls_id, {ts, v}}); });

  auto etalon = etalons.begin();
  auto outcome = outcomes.begin();

  while (etalon != etalons.end() && outcome != outcomes.end()) {
    EXPECT_EQ((*outcome).first, (*etalon).first);
    EXPECT_EQ((*outcome).second.first, (*etalon).second.first);
    EXPECT_EQ((*outcome).second.second, (*etalon).second.second);
    ++etalon;
    ++outcome;
  }

  EXPECT_EQ(outcome == outcomes.end(), etalon == etalons.end());
  EXPECT_EQ(outcomes.size(), etalons.size());

  buf.clear();
  EXPECT_EQ(buf.samples_count(), 0);
}

TEST_F(Wal, BasicEncoderBasicDecoder) {
  PromPP::WAL::BasicEncoder<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap> writer;

  const LabelSetForTest etalons_label_set = generate_label_set();
  const samples_sequence_type etalons_samples = generate_samples(1000, 1.123);
  const TimeSeriesForTest etalons_timeseries = TimeSeriesForTest(etalons_label_set, etalons_samples);

  writer.add(etalons_timeseries);

  EXPECT_EQ(writer.buffer().latest_sample(), (*(etalons_samples.end() - 1)).timestamp());

  // check writer label_set
  auto outcomes_writer = writer.label_sets().items()[0].composite(etalons_timeseries.data());
  auto outcome_label_set_writer = outcomes_writer.begin();
  auto etalon_label_set_writer = etalons_label_set.begin();

  while (etalon_label_set_writer != etalons_label_set.end() && outcome_label_set_writer != outcomes_writer.end()) {
    EXPECT_EQ((*outcome_label_set_writer).first, (*etalon_label_set_writer).first);
    EXPECT_EQ((*outcome_label_set_writer).second, (*etalon_label_set_writer).second);
    ++etalon_label_set_writer;
    ++outcome_label_set_writer;
  }

  EXPECT_EQ(writer.samples(), 0);

  PromPP::WAL::BasicDecoder<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap> reader{PromPP::WAL::BasicEncoderVersion::kV3};
  std::stringstream stream_buffer;

  // save wal
  stream_buffer << writer;
  EXPECT_EQ(writer.samples(), NUM_VALUES);

  // read wal
  stream_buffer >> reader;

  // check reader label_set
  auto outcomes_reader = reader.label_sets().items()[0].composite(etalons_timeseries.data());
  auto outcome_label_set_reader = outcomes_reader.begin();
  etalon_label_set_writer = etalons_label_set.begin();
  while (etalon_label_set_writer != etalons_label_set.end() && outcome_label_set_reader != outcomes_reader.end()) {
    EXPECT_EQ((*outcome_label_set_reader).first, (*etalon_label_set_writer).first);
    EXPECT_EQ((*outcome_label_set_reader).second, (*etalon_label_set_writer).second);
    ++etalon_label_set_writer;
    ++outcome_label_set_reader;
  }

  // check reader samples
  samples_sequence_type outcomes_reader_samples;
  reader.process_segment([&](uint32_t, uint64_t ts, double v) { outcomes_reader_samples.push_back(SampleForTest(ts, v)); });

  auto etalon_sample_reader = etalons_samples.begin();
  auto outcome_sample_reader = outcomes_reader_samples.begin();
  while (etalon_sample_reader != etalons_samples.end() && outcome_sample_reader != outcomes_reader_samples.end()) {
    EXPECT_EQ((*outcome_sample_reader).timestamp(), (*etalon_sample_reader).timestamp());
    EXPECT_EQ((*outcome_sample_reader).value(), (*etalon_sample_reader).value());
    ++etalon_sample_reader;
    ++outcome_sample_reader;
  }

  EXPECT_EQ(outcome_sample_reader == outcomes_reader_samples.end(), etalon_sample_reader == etalons_samples.end());
  EXPECT_EQ(outcomes_reader_samples.size(), etalons_samples.size());

  EXPECT_EQ(reader.samples(), NUM_VALUES);
}

TEST_F(Wal, Snapshots) {
  using WALEncoder = PromPP::WAL::BasicEncoder<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap>;
  using WALDecoder = PromPP::WAL::BasicDecoder<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap>;
  WALEncoder encoder(2, 3);
  WALDecoder decoder{PromPP::WAL::BasicEncoderVersion::kV3};
  std::stringstream writer_stream;
  std::vector<std::unique_ptr<WALEncoder::Redundant>> redundants;
  std::ofstream devnull("/dev/null");
  for (int i = 0; i < 10; ++i) {     // segments
    for (int j = 0; j < 100; ++j) {  // samples
      const LabelSetForTest label_set = generate_label_set();
      const samples_sequence_type samples = generate_samples(1000, 1.123);
      const TimeSeriesForTest timeseries = TimeSeriesForTest(label_set, samples);
      encoder.add(timeseries);
    }
    if (i < 5) {
      writer_stream << encoder;
      writer_stream >> decoder;
      decoder.process_segment([](uint32_t, uint64_t, double) {});
    } else {
      redundants.emplace_back(encoder.write(devnull));
    }
  }

  std::stringstream stream_buffer;

  // save wal
  encoder.snapshot(redundants, stream_buffer);

  EXPECT_GT(stream_buffer.tellp(), 0);

  // read wal from snapshot
  WALDecoder decoder2{PromPP::WAL::BasicEncoderVersion::kV3};
  decoder2.load_snapshot(stream_buffer);

  EXPECT_GT(stream_buffer.tellg(), 0);

  // check label sets
  std::stringstream reader_sbuf1, reader_sbuf2;
  reader_sbuf1 << decoder.label_sets().checkpoint();
  reader_sbuf2 << decoder2.label_sets().checkpoint();
  EXPECT_EQ(reader_sbuf1.view(), reader_sbuf2.view());
  EXPECT_EQ(decoder.decoders(), decoder2.decoders());
}
}  // namespace

TEST_F(Wal, BasicEncoderMany) {
  using WALEncoder = PromPP::WAL::BasicEncoder<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap>;
  using AddManyCallbackType = WALEncoder::add_many_generator_callback_type;
  using enum AddManyCallbackType;
  WALEncoder encoder(2, 3);

  const LabelSetForTest ls1{{"LS", "1"}};
  const LabelSetForTest ls2{{"LS", "2"}};
  const LabelSetForTest ls3{{"LS", "3"}};
  const LabelSetForTest ls4{{"LS", "4"}};

  const PromPP::Primitives::Timestamp ts[]{1000, 2000};

  auto generator = [&](auto add_cb) {
    add_cb(TimeSeriesForTest(ls1, {{1000, 1}}));
    add_cb(TimeSeriesForTest(ls2, {{1000, 0}}));
    add_cb(TimeSeriesForTest(ls3, {{1000, 2}}));
  };

  auto generator_2 = [&](auto add_cb) {
    add_cb(TimeSeriesForTest(ls1, {{2000, 2}}));
    add_cb(TimeSeriesForTest(ls3, {{2000, 2}}));
    add_cb(TimeSeriesForTest(ls4, {{2000, 2}}));
  };

  //
  decltype(encoder)::SourceState state = encoder.add_many<without_hash_value, TimeSeriesForTest>(0, ts[0], generator);
  state = encoder.add_many<without_hash_value, TimeSeriesForTest>(state, ts[1], generator_2);
  decltype(encoder)::DestroySourceState(state);

  struct Item {
    uint32_t ls_id;
    SampleForTest sample;
    Item(uint32_t id, PromPP::Primitives::Timestamp ts, double v) : ls_id(id), sample(ts, v) {}
  };

  // clang-format off
  Item expected_items[]{
      {0, 1000, 1.0},
      {0, 2000, 2.0},
      {1, 1000, 0},
      {1, 2000, BareBones::Encoding::Gorilla::STALE_NAN},
      {2, 1000, 2},
      {2, 2000, 2},
      {3, 2000, 2},
  };
  // clang-format on

  size_t ind = 0;

  encoder.buffer().for_each([&](auto ls_id, auto timestamp, double v) {
    EXPECT_TRUE(ind < std::size(expected_items));
    EXPECT_EQ(expected_items[ind].ls_id, ls_id);
    EXPECT_EQ(expected_items[ind].sample.timestamp(), timestamp);
    EXPECT_EQ(std::bit_cast<uint64_t>(expected_items[ind].sample.value()), std::bit_cast<uint64_t>(v));
    ++ind;
  });
}
