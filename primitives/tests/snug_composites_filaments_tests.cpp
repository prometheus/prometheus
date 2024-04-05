#include <random>

#include "gtest/gtest.h"

#include "primitives/snug_composites_filaments.h"

namespace {

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

  friend size_t hash_value(const LabelSetForTest& tls) {
    size_t res = 0;
    for (const auto& [label_name, label_value] : tls) {
      res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), res);
    }
    return res;
  }
};

const char SYMBOLS_DATA[89] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-+=/|.,\\?<>!@#$%^&*()\"':;";
const int NUM_VALUES = 1000;

std::string generate_str(int seed) {
  std::mt19937 gen32(seed);
  std::string b;
  int maxlen = 4 + (gen32() % 28);
  for (int i = 0; i < maxlen; i++) {
    b += SYMBOLS_DATA[gen32() % 89];
  }

  return b;
};

NamesSetForTest generate_names_set() {
  NamesSetForTest nst;

  for (int i = 0; i < NUM_VALUES; i++) {
    nst.push_back(generate_str(i));
  }

  return nst;
};

LabelSetForTest generate_label_set() {
  LabelSetForTest lst;

  for (auto i = 0; i < NUM_VALUES; ++i) {
    lst.push_back({generate_str(i), generate_str(i + NUM_VALUES)});
  }

  return lst;
};

struct SnugCompositesFilaments : public testing::Test {};

TEST_F(SnugCompositesFilaments, Symbol) {
  PromPP::Primitives::SnugComposites::Filaments::Symbol::data_type data;

  std::vector<std::pair<std::string, PromPP::Primitives::SnugComposites::Filaments::Symbol>> etalons_and_outcomes;

  for (int i = 0; i < NUM_VALUES; i++) {
    const auto etalon = generate_str(i);
    auto outcome = PromPP::Primitives::SnugComposites::Filaments::Symbol(data, etalon);
    EXPECT_NO_THROW(outcome.validate(data));

    etalons_and_outcomes.push_back({etalon, outcome});
  }

  for (const auto& [etalon, outcome] : etalons_and_outcomes) {
    EXPECT_EQ(outcome.composite(data), etalon);
  }
}

TEST_F(SnugCompositesFilaments, LabelNamesSet) {
  using FillamentLabelNameSet =
      PromPP::Primitives::SnugComposites::Filaments::LabelNameSet<BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol>>;

  FillamentLabelNameSet::data_type data;
  const NamesSetForTest etalons = generate_names_set();

  auto lns = FillamentLabelNameSet(data, etalons);
  EXPECT_NO_THROW(lns.validate(data));
  auto outcomes = lns.composite(data);

  auto etalon = etalons.begin();
  auto outcome = outcomes.begin();
  while (etalon != etalons.end() && outcome != outcomes.end()) {
    EXPECT_EQ(*outcome++, *etalon++);
  }

  EXPECT_EQ(outcome == outcomes.end(), etalon == etalons.end());
  EXPECT_EQ(outcomes.size(), etalons.size());
  EXPECT_EQ(hash_value(outcomes), hash_value(etalons));
}

TEST_F(SnugCompositesFilaments, LabelSet) {
  using FillamentLabelSet =
      PromPP::Primitives::SnugComposites::Filaments::LabelSet<BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol>,
                                                            BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::LabelNameSet<
                                                                BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol>>>>;

  FillamentLabelSet::data_type data;
  const LabelSetForTest etalons = generate_label_set();

  auto ls = FillamentLabelSet(data, etalons);
  EXPECT_NO_THROW(ls.validate(data));
  auto outcomes = ls.composite(data);

  auto etalon = etalons.begin();
  auto outcome = outcomes.begin();
  while (etalon != etalons.end() && outcome != outcomes.end()) {
    EXPECT_EQ((*outcome).first, (*etalon).first);
    EXPECT_EQ((*outcome).second, (*etalon).second);
    ++etalon;
    ++outcome;
  }

  EXPECT_EQ(outcome == outcomes.end(), etalon == etalons.end());
  EXPECT_EQ(outcomes.size(), etalons.size());

  EXPECT_EQ(hash_value(outcomes), hash_value(etalons));
}

class EncodingTableLabelSetFixture : public testing::Test {
 protected:
  using FillamentLabelSet =
      PromPP::Primitives::SnugComposites::Filaments::LabelSet<BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol>,
                                                            BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::LabelNameSet<
                                                                BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol>>>>;

  BareBones::SnugComposite::EncodingTable<FillamentLabelSet> encoding_table_;
  BareBones::SnugComposite::DecodingTable<FillamentLabelSet> decoding_table_;
  std::array<LabelSetForTest, 3> ls_;

  void SetUp() override {
    ls_[0].emplace_back("1", "1");
    ls_[0].emplace_back("2", "2");

    ls_[1].emplace_back("3", "3");

    ls_[2].emplace_back("4", "4");
  }

  auto create_and_load_checkpoint(BareBones::SnugComposite::EncodingTable<FillamentLabelSet>::checkpoint_type* from) {
    auto checkpoint = encoding_table_.checkpoint();
    std::stringstream ss;
    checkpoint.save(ss, from);
    decoding_table_.load(ss);

    return checkpoint;
  }

  void check_decoding_table() {
    ASSERT_EQ(3U, decoding_table_.size());
    {
      auto composite = decoding_table_.items()[0].composite(decoding_table_.data());
      ASSERT_EQ(2U, composite.size());
      auto it = composite.begin();
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("1", "1")), *it++);
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("2", "2")), *it);
    }
    {
      auto composite = decoding_table_.items()[1].composite(decoding_table_.data());
      ASSERT_EQ(1U, composite.size());
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("3", "3")), *composite.begin());
    }
    {
      auto composite = decoding_table_.items()[2].composite(decoding_table_.data());
      ASSERT_EQ(1U, composite.size());
      EXPECT_EQ((std::pair<std::string_view, std::string_view>("4", "4")), *composite.begin());
    }
  }
};

TEST_F(EncodingTableLabelSetFixture, ShrinkAndLoad) {
  // Arrange

  // Act
  {
    encoding_table_.emplace(ls_[0]);
    encoding_table_.emplace(ls_[1]);
    auto checkpoint = create_and_load_checkpoint(nullptr);
    encoding_table_.shrink_to_checkpoint_size(checkpoint);
  }
  {
    auto empty_checkpoint = encoding_table_.checkpoint();
    encoding_table_.emplace(ls_[2]);
    auto checkpoint = create_and_load_checkpoint(&empty_checkpoint);
    encoding_table_.shrink_to_checkpoint_size(checkpoint);
  }

  // Assert
  check_decoding_table();
}

TEST_F(EncodingTableLabelSetFixture, LoadWithoutShrink) {
  // Arrange

  // Act
  {
    encoding_table_.emplace(ls_[0]);
    encoding_table_.emplace(ls_[1]);
    create_and_load_checkpoint(nullptr);
  }
  {
    auto empty_checkpoint = encoding_table_.checkpoint();
    encoding_table_.emplace(ls_[2]);
    create_and_load_checkpoint(&empty_checkpoint);
  }

  // Assert
  check_decoding_table();
}

TEST_F(EncodingTableLabelSetFixture, EmptyCheckpointWithShrink) {
  // Arrange

  // Act
  {
    encoding_table_.emplace(ls_[0]);
    encoding_table_.emplace(ls_[1]);
    encoding_table_.emplace(ls_[2]);
    auto checkpoint = create_and_load_checkpoint(nullptr);
    encoding_table_.shrink_to_checkpoint_size(checkpoint);
  }
  {
    auto empty_checkpoint = encoding_table_.checkpoint();
    create_and_load_checkpoint(&empty_checkpoint);
  }

  // Assert
  check_decoding_table();
}

TEST_F(EncodingTableLabelSetFixture, EmptyCheckpointWithoutShrink) {
  // Arrange

  // Act
  {
    encoding_table_.emplace(ls_[0]);
    encoding_table_.emplace(ls_[1]);
    encoding_table_.emplace(ls_[2]);
    create_and_load_checkpoint(nullptr);
  }
  {
    auto empty_checkpoint = encoding_table_.checkpoint();
    create_and_load_checkpoint(&empty_checkpoint);
  }

  // Assert
  check_decoding_table();
}

}  // namespace
