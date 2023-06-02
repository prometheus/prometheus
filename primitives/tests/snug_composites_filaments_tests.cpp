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
}  // namespace
