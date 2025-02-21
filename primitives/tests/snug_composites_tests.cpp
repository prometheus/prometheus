#include <sstream>

#include "gtest/gtest.h"
#include "primitives/hash.h"
#include "primitives/label_set.h"

#include "primitives/snug_composites.h"
#include "primitives/snug_composites_filaments.h"

namespace {

using BareBones::SharedSpan;
using BareBones::SharedVector;
using BareBones::Vector;
using PromPP::Primitives::LabelViewSet;
using std::operator""sv;

template <template <class> class Vector>
class GenericNamesSetForTest : public Vector<std::string> {
  using Base = Vector<std::string>;

 public:
  using Base::Base;
  friend size_t hash_value(const GenericNamesSetForTest& lns) { return PromPP::Primitives::hash::hash_of_string_list(lns); }
};

using NamesSetForTest = GenericNamesSetForTest<std::vector>;

template <template <class> class Vector>
class GenericLabelSetForTest : public Vector<std::pair<std::string, std::string>> {
  using Base = Vector<std::pair<std::string, std::string>>;

 public:
  using Base::Base;

  GenericNamesSetForTest<Vector> names() const {
    GenericNamesSetForTest<Vector> tns;

    for (const auto& [label_name, _] : *this) {
      tns.push_back(label_name);
    }

    return tns;
  }
};

using LabelSetForTest = GenericLabelSetForTest<std::vector>;

template <template <class> class Vector>
using SymbolFilament = BareBones::SnugComposite::EncodingBimap<PromPP::Primitives::SnugComposites::Filaments::Symbol, Vector>;

template <template <class> class Vector>
using LabelNameSetFilament = PromPP::Primitives::SnugComposites::Filaments::LabelNameSet<SymbolFilament, Vector>;

template <template <class> class Vector>
using LabelSetFilament = BareBones::SnugComposite::EncodingBimap<LabelNameSetFilament, Vector>;

template <template <class> class Vector>
using GenericLabelSet = PromPP::Primitives::SnugComposites::Filaments::LabelSet<SymbolFilament, LabelSetFilament, Vector>;

using LabelSet = GenericLabelSet<Vector>;

struct SnugComposites : public testing::Test {};

TEST(SnugComposites, OrderedDecodingTableWithSymbolsHandlesZeroStringAsFirstElementCorrectly) {
  PromPP::Primitives::SnugComposites::Symbol::OrderedDecodingTable<Vector> t;

  EXPECT_EQ(t.emplace_back(""), 0);
  EXPECT_EQ(t[0], "");
}

TEST(SnugComposites, SnapshotRollbackSymbolEncodingBimap) {
  PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<Vector> encoding_bimap;

  const std::vector<std::string> etalons_rollback = {"11111", "22222", "33333"};
  const std::vector<std::string> etalons = {"44444", "55555", "66666"};
  const auto first_element = "xxxxx";

  // add one element
  encoding_bimap.find_or_emplace(first_element);
  // checkpoint
  auto checkpoint = encoding_bimap.checkpoint();

  // add elements
  for (const auto& etalon : etalons_rollback) {
    encoding_bimap.find_or_emplace(etalon);
  }

  // check elements
  auto etalon = etalons_rollback.begin();
  auto outcome = encoding_bimap.begin();
  EXPECT_EQ(*outcome, first_element);
  outcome++;
  while (etalon != etalons_rollback.end() && outcome != encoding_bimap.end()) {
    EXPECT_EQ(*outcome++, *etalon++);
  }

  // rollback
  encoding_bimap.rollback(checkpoint);

  // add new elements
  for (const auto& str : etalons) {
    encoding_bimap.find_or_emplace(str);
  }

  // check new elements
  etalon = etalons.begin();
  outcome = encoding_bimap.begin();
  EXPECT_EQ(*outcome, first_element);
  outcome++;
  while (etalon != etalons.end() && outcome != encoding_bimap.end()) {
    EXPECT_EQ(*outcome++, *etalon++);
  }
}

TEST(SnugComposites, SnapshotRollbackLabelNameSetEncodingBimap) {
  PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<Vector> encoding_bimap;

  const std::vector<NamesSetForTest> etalons_rollback = {{"11111", "22222", "33333"}, {"44444", "55555", "66666"}};
  const std::vector<NamesSetForTest> etalons = {{"77777", "88888", "99999"}, {"aaaaa", "sssss", "ddddd"}};
  const NamesSetForTest first_element = {"xxxxx", "zzzzz", "ccccc"};

  // add one element
  encoding_bimap.find_or_emplace(first_element);
  // checkpoint
  auto checkpoint = encoding_bimap.checkpoint();

  // add elements
  for (const auto& etalon : etalons_rollback) {
    encoding_bimap.find_or_emplace(etalon);
  }

  // check elements
  auto etalon = etalons_rollback.begin();
  auto outcome = encoding_bimap.begin();
  EXPECT_EQ(*outcome, first_element);
  outcome++;
  while (etalon != etalons_rollback.end() && outcome != encoding_bimap.end()) {
    EXPECT_EQ(*outcome++, *etalon++);
  }

  // rollback
  encoding_bimap.rollback(checkpoint);

  // add new elements
  for (const auto& etalon : etalons) {
    encoding_bimap.find_or_emplace(etalon);
  }

  // check new elements
  etalon = etalons.begin();
  outcome = encoding_bimap.begin();
  EXPECT_EQ(*outcome, first_element);
  outcome++;
  while (etalon != etalons.end() && outcome != encoding_bimap.end()) {
    EXPECT_EQ(*outcome++, *etalon++);
  }
}

TEST(SnugComposites, SnapshotRollbackLabelSetEncodingBimap) {
  LabelSet::data_type data;
  auto data_checkpoint = data.checkpoint();

  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector> encoding_bimap;

  const std::vector<LabelSetForTest> etalons_rollback = {{{"11111", "22222"}}, {{"33333", "44444"}}, {{"55555", "66666"}}};
  const std::vector<LabelSetForTest> etalons = {{{"77777", "88888"}}, {{"99999", "aaaaa"}}, {{"sssss", "ddddd"}}};
  const LabelSetForTest first_element = {{"xxxxx", "zzzzz"}};

  // add one element
  auto ls = LabelSet(data, first_element);
  EXPECT_NO_THROW(ls.validate(data));
  encoding_bimap.find_or_emplace(ls.composite(data));
  // checkpoint
  auto checkpoint = encoding_bimap.checkpoint();

  // add elements
  for (const auto& etalon : etalons_rollback) {
    ls = LabelSet(data, etalon);
    EXPECT_NO_THROW(ls.validate(data));
    encoding_bimap.find_or_emplace(ls.composite(data));
  }

  // check elements
  auto etalon = etalons_rollback.begin();
  auto outcome = encoding_bimap.begin();

  auto etalon_first = first_element.begin();
  auto outcome_first = (*outcome).begin();
  while (etalon_first != first_element.end() && outcome_first != (*outcome).end()) {
    EXPECT_EQ((*outcome_first).first, (*etalon_first).first);
    EXPECT_EQ((*outcome_first).second, (*etalon_first).second);
    ++etalon_first;
    ++outcome_first;
  }

  outcome++;
  while (etalon != etalons_rollback.end() && outcome != encoding_bimap.end()) {
    auto etalon_another = (*etalon).begin();
    auto outcome_another = (*outcome).begin();

    while (etalon_another != (*etalon).end() && outcome_another != (*outcome).end()) {
      EXPECT_EQ((*outcome_another).first, (*etalon_another).first);
      EXPECT_EQ((*outcome_another).second, (*etalon_another).second);
      ++etalon_another;
      ++outcome_another;
    }

    etalon++;
    outcome++;
  }

  // rollback
  data.rollback(data_checkpoint);
  encoding_bimap.rollback(checkpoint);

  // add new elements
  for (const auto& etalon : etalons) {
    ls = LabelSet(data, etalon);
    EXPECT_NO_THROW(ls.validate(data));
    encoding_bimap.find_or_emplace(ls.composite(data));
  }

  // check new elements
  etalon = etalons.begin();
  outcome = encoding_bimap.begin();

  etalon_first = first_element.begin();
  outcome_first = (*outcome).begin();
  while (etalon_first != first_element.end() && outcome_first != (*outcome).end()) {
    EXPECT_EQ((*outcome_first).first, (*etalon_first).first);
    EXPECT_EQ((*outcome_first).second, (*etalon_first).second);
    ++etalon_first;
    ++outcome_first;
  }

  outcome++;
  while (etalon != etalons.end() && outcome != encoding_bimap.end()) {
    auto etalon_another = (*etalon).begin();
    auto outcome_another = (*outcome).begin();

    while (etalon_another != (*etalon).end() && outcome_another != (*outcome).end()) {
      EXPECT_EQ((*outcome_another).first, (*etalon_another).first);
      EXPECT_EQ((*outcome_another).second, (*etalon_another).second);
      ++etalon_another;
      ++outcome_another;
    }

    etalon++;
    outcome++;
  }
}

template <class T>
class EncodingBimap : public testing::Test {
 public:
  static void find_or_emplace(PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<Vector>& encoding_bimap) {
    const auto str = "11111";
    encoding_bimap.find_or_emplace(str);
  };

  static void find_or_emplace(PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<Vector>& encoding_bimap) {
    const NamesSetForTest nst = {"11111"};
    encoding_bimap.find_or_emplace(nst);
  };

  static void find_or_emplace(PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector>& encoding_bimap) {
    const LabelSetForTest lst = {{"11111", "22222"}};
    LabelSet::data_type data;
    auto ls = LabelSet(data, lst);
    EXPECT_NO_THROW(ls.validate(data));

    encoding_bimap.find_or_emplace(ls.composite(data));
  };
};

typedef testing::Types<PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<Vector>,
                       PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<Vector>,
                       PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<Vector>>
    FilamentTypes;
TYPED_TEST_SUITE(EncodingBimap, FilamentTypes);

TYPED_TEST(EncodingBimap, SaveSizeFULL) {
  TypeParam encoding_bimap;
  std::ostringstream stream;

  EncodingBimap<TypeParam>::find_or_emplace(encoding_bimap);
  auto checkpoint = encoding_bimap.checkpoint();
  const auto etalon_save_size_full = checkpoint.save_size();
  stream << checkpoint;
  EXPECT_EQ(stream.tellp(), etalon_save_size_full);
}

TYPED_TEST(EncodingBimap, SaveSizeWAL) {
  TypeParam encoding_bimap;
  std::ostringstream stream;

  auto base = encoding_bimap.checkpoint();
  EncodingBimap<TypeParam>::find_or_emplace(encoding_bimap);
  auto checkpoint = encoding_bimap.checkpoint();
  auto delta = checkpoint - base;
  const auto etalon_save_size_wal = delta.save_size();
  stream << delta;
  EXPECT_EQ(stream.tellp(), etalon_save_size_wal);
}

class LssViewFixture : public testing::Test {
 protected:
  using Symbol = PromPP::Primitives::SnugComposites::Symbol::EncodingBimap<SharedVector>;
  using SymbolView = PromPP::Primitives::SnugComposites::Symbol::DecodingTable<SharedSpan>;

  using LabelNameSet = PromPP::Primitives::SnugComposites::LabelNameSet::EncodingBimap<SharedVector>;
  using LabelNameSetView = PromPP::Primitives::SnugComposites::LabelNameSet::DecodingTable<SharedSpan>;

  using LabelSet = PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap<SharedVector>;
  using LabelSetView = PromPP::Primitives::SnugComposites::LabelSet::DecodingTable<SharedSpan>;
};

TEST_F(LssViewFixture, CopySymbol) {
  // Arrange
  Symbol source;
  constexpr auto source_data = "string1"sv;
  source.find_or_emplace(source_data);

  // Act
  const SymbolView view(source);
  source.find_or_emplace("string2"sv);

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_EQ(source_data, view[0]);
}

TEST_F(LssViewFixture, CopyLabelNameSet) {
  // Arrange
  LabelNameSet source;
  const NamesSetForTest source_data{"name1", "name2", "name3"};
  source.find_or_emplace(source_data);

  // Act
  const LabelNameSetView view(source);
  source.find_or_emplace(NamesSetForTest{"name4", "name5"});

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_EQ(source_data, view[0]);
}

TEST_F(LssViewFixture, CopyLabelSet) {
  // Arrange
  LabelSet source;
  const LabelViewSet source_data{{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  source.find_or_emplace(source_data);

  // Act
  const LabelSetView view(source);
  source.find_or_emplace(LabelViewSet{{"name4", "value4"}});

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_EQ(source_data, view[0]);
}

TEST_F(LssViewFixture, UseCopyLabelSetAfterFreeSourceLabelSet) {
  // Arrange
  auto source = std::make_unique<LabelSet>();
  const LabelViewSet source_data{{"name1", "value1"}, {"name2", "value2"}, {"name3", "value3"}};
  source->find_or_emplace(source_data);

  // Act
  const LabelSetView view(*source);
  source.reset();

  // Assert
  EXPECT_EQ(1U, view.size());
  EXPECT_EQ(source_data, view[0]);
}

}  // namespace
