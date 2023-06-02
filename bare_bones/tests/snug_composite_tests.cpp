#include <random>

#include <gtest/gtest.h>

#include "bare_bones/snug_composite.h"

namespace {

class TestSnugCompositesStringFilament {
  uint32_t pos_;
  uint32_t len_;

 public:
  // NOLINTNEXTLINE
  class data_type : public BareBones::Vector<char> {
    class Checkpoint {
      uint32_t size_;

      using SerializationMode = BareBones::SnugComposite::SerializationMode;

     public:
      explicit Checkpoint(uint32_t size) noexcept : size_(size) {}

      uint32_t size() const { return size_; }

      template <BareBones::OutputStream S>
      void save(S& out, data_type const data, Checkpoint const* from = nullptr) const {
        SerializationMode mode = (from != nullptr) ? SerializationMode::DELTA : SerializationMode::SNAPSHOT;
        out.put(static_cast<char>(mode));
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->size_;
          out.write(reinterpret_cast<const char*>(&first_to_save), sizeof(first_to_save));
        }
        uint32_t size_to_save = size_ - first_to_save;
        out.write(reinterpret_cast<char*>(&size_to_save), sizeof(size_to_save));
        out.write(data.begin() + first_to_save, size_to_save);
      }

      uint32_t save_size(data_type const _data, Checkpoint const* from = nullptr) const {
        uint32_t res = 1;  // mode
        uint32_t first_to_save = 0;
        if (from != nullptr) {
          first_to_save = from->size_;
          res += sizeof(uint32_t);  // first index
        }
        uint32_t size_to_save = size_ - first_to_save;
        res += sizeof(uint32_t);  // length
        res += size_to_save;      // data
        return res;
      }
    };

   public:
    using checkpoint_type = Checkpoint;
    inline __attribute__((always_inline)) auto checkpoint() const noexcept { return Checkpoint(size()); }
    inline __attribute__((always_inline)) void rollback(const checkpoint_type& s) noexcept {
      assert(s.size() <= size());
      resize(s.size());
    }

    template <BareBones::InputStream S>
    void load(S& in) {
      BareBones::SnugComposite::SerializationMode mode = static_cast<BareBones::SnugComposite::SerializationMode>(in.get());
      // read pos of first symbol in the portion, if we are reading wal
      uint32_t first_to_load_i = 0;
      if (mode == BareBones::SnugComposite::SerializationMode::DELTA) {
        in.read(reinterpret_cast<char*>(&first_to_load_i), sizeof(first_to_load_i));
      }

      if (first_to_load_i != size()) {
        throw std::runtime_error("something went wrong");
      }

      uint32_t size_to_load;
      in.read(reinterpret_cast<char*>(&size_to_load), sizeof(size_to_load));

      // read data
      resize(size() + size_to_load);
      in.read(begin() + first_to_load_i, size_to_load);
    }
  };

  using composite_type = std::string_view;

  TestSnugCompositesStringFilament() = default;
  TestSnugCompositesStringFilament(data_type& data, std::string x) : pos_(data.size()), len_(x.length()) { data.push_back(x.begin(), x.end()); }

  const std::string_view composite(const data_type& data) const { return std::string_view(data.begin() + pos_, len_); }

  void validate(const data_type& data) const {
    if (pos_ + len_ > data.size())
      throw std::runtime_error("something went wrong");
  }
};

typedef testing::Types<BareBones::SnugComposite::EncodingBimap<TestSnugCompositesStringFilament>,
                       BareBones::SnugComposite::ParallelEncodingBimap<TestSnugCompositesStringFilament>,
                       BareBones::SnugComposite::OrderedEncodingBimap<TestSnugCompositesStringFilament>,
                       BareBones::SnugComposite::EncodingBimapWithOrderedAccess<TestSnugCompositesStringFilament>>
    SnugCompositesBimapTypes;

template <class T>
struct SnugComposite : public testing::Test {
  using StringFilament = TestSnugCompositesStringFilament;
};

TYPED_TEST_SUITE(SnugComposite, SnugCompositesBimapTypes);

TYPED_TEST(SnugComposite, should_return_same_id_for_same_data) {
  TypeParam outcomes;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  for (auto i = 0; i < 100; ++i) {
    outcomes.find_or_emplace(std::to_string(gen32()));
  }
  std::string v = "12";
  auto id = outcomes.find_or_emplace(v);
  ASSERT_EQ(id, outcomes.find_or_emplace(v));

  ASSERT_EQ(outcomes.find_or_emplace(v), outcomes.find(v));
}

TYPED_TEST(SnugComposite, should_return_same_id_for_same_data_with_hash) {
  TypeParam outcomes;

  if constexpr (!std::is_same<TypeParam, BareBones::SnugComposite::OrderedEncodingBimap<TestSnugCompositesStringFilament>>::value) {
    std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
    for (auto i = 0; i < 100; ++i) {
      outcomes.find_or_emplace(std::to_string(gen32()));
    }

    std::string v = "12";
    auto hash_val = std::hash<std::string>()(v);
    auto id = outcomes.find_or_emplace(v, hash_val);

    EXPECT_EQ(id, outcomes.find_or_emplace(v, hash_val));
    EXPECT_EQ(id, outcomes.find_or_emplace(v));

    EXPECT_EQ(outcomes.find_or_emplace(v, hash_val), outcomes.find(v, hash_val));
    EXPECT_EQ(outcomes.find_or_emplace(v, hash_val), outcomes.find(v));

    v = "21";
    hash_val = std::hash<std::string>()(v);
    id = outcomes.find_or_emplace(v);

    EXPECT_EQ(id, outcomes.find_or_emplace(v, hash_val));
    EXPECT_EQ(id, outcomes.find_or_emplace(v));

    EXPECT_EQ(outcomes.find_or_emplace(v), outcomes.find(v));
    EXPECT_EQ(outcomes.find_or_emplace(v), outcomes.find(v, hash_val));
  }
}

TYPED_TEST(SnugComposite, should_return_different_id_for_different_data) {
  TypeParam outcomes;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  for (auto i = 0; i < 100; ++i) {
    outcomes.find_or_emplace(std::to_string(gen32()));
  }
  std::string v = "12";
  auto id = outcomes.find_or_emplace(v);
  v = "21";
  ASSERT_NE(id, outcomes.find_or_emplace(v));
}

TYPED_TEST(SnugComposite, should_return_different_id_for_different_data_with_hash) {
  TypeParam outcomes;

  if constexpr (!std::is_same<TypeParam, BareBones::SnugComposite::OrderedEncodingBimap<TestSnugCompositesStringFilament>>::value) {
    std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
    for (auto i = 0; i < 100; ++i) {
      outcomes.find_or_emplace(std::to_string(gen32()));
    }

    std::string v = "12";
    size_t hash_val = std::hash<std::string>()(v);
    auto id = outcomes.find_or_emplace(v, hash_val);

    v = "21";
    hash_val = std::hash<std::string>()(v);

    EXPECT_NE(id, outcomes.find_or_emplace(v, hash_val));
    EXPECT_NE(id, outcomes.find_or_emplace(v));

    v = "13";
    id = outcomes.find_or_emplace(v);

    v = "31";
    hash_val = std::hash<std::string>()(v);

    EXPECT_NE(id, outcomes.find_or_emplace(v, hash_val));
    EXPECT_NE(id, outcomes.find_or_emplace(v));
  }
}

TYPED_TEST(SnugComposite, should_has_all_pushed_items) {
  TypeParam outcomes;
  std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
  for (auto i = 0; i < 100; ++i) {
    outcomes.find_or_emplace(std::to_string(gen32()));
  }
  std::string v = "12";
  auto id = outcomes.find_or_emplace(v);
  v = "21";
  ASSERT_NE(id, outcomes.find_or_emplace(v));
  // std::set<std::string> expected;
  // for (auto i = 0; i < 100; ++i) {
  //   v = std::to_string(gen32());
  // expected.insert(s);
  //   x.find_or_emplace(std::to_string(gen32()));
  // }
  // std::set<std::string> actual;
  // for (auto i = x.begin(); i != x.end(); ++i) {
  //   std::string_view sv = *i;
  //   std::string s(sv);
  //   actual.insert(s);
  // }
  // ASSERT_EQ(expected, actual);
}

TYPED_TEST(SnugComposite, should_assign_new_ids_after_rollback) {
  TypeParam outcomes;

  auto checkpoint0 = outcomes.checkpoint();
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 1);

  auto checkpoint1 = outcomes.checkpoint();
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test2")), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test3")), 2);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 3);

  outcomes.rollback(checkpoint1);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test_new")), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test2")), 2);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test3")), 3);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 4);

  auto checkpoint2 = outcomes.checkpoint();
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test4")), 4);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test_new")), 1);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 5);

  outcomes.rollback(checkpoint2);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 4);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test_more_new")), 4);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test4")), 5);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test_new")), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test2")), 2);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test3")), 3);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);

  outcomes.rollback(checkpoint1);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test_more_new")), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);

  outcomes.rollback(checkpoint0);
  ASSERT_EQ(std::ranges::distance(outcomes.begin(), outcomes.end()), 0);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test_new")), 0);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 1);
}

TYPED_TEST(SnugComposite, should_write_the_same_wal_after_rollback) {
  TypeParam outcomes;

  // checkpoint names template is checkpoint<base><variant>
  // for example, if we have state X and checkpoint to this state checkpoint21
  // then we change state with adding some data, then we get new checkpoint211
  // Now if we rollback state to checkpoint2_1 and add some data we get new checkpoint212
  //
  // wal names template is wal<last checkpoint index>
  // for example, wal between checkpoint212 and checkpoint21 we get wal212

  auto checkpoint = outcomes.checkpoint();
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);

  auto checkpoint1 = outcomes.checkpoint();
  std::ostringstream wal1;
  wal1 << (checkpoint1 - checkpoint) << std::flush;

  ASSERT_EQ(outcomes.find_or_emplace(std::string("test2")), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test3")), 2);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);

  auto checkpoint11 = outcomes.checkpoint();
  std::ostringstream wal11;
  wal11 << (checkpoint11 - checkpoint1) << std::flush;

  outcomes.rollback(checkpoint1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test2")), 1);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test3")), 2);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);

  auto checkpoint12 = outcomes.checkpoint();
  std::ostringstream wal12;
  wal12 << (checkpoint12 - checkpoint1) << std::flush;
  ASSERT_EQ(wal11.str(), wal12.str());

  ASSERT_EQ(outcomes.find_or_emplace(std::string("test4")), 3);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test5")), 4);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test3")), 2);

  auto checkpoint121 = outcomes.checkpoint();
  std::ostringstream wal121;
  wal121 << (checkpoint121 - checkpoint12) << std::flush;

  ASSERT_EQ(outcomes.find_or_emplace(std::string("test6")), 5);

  outcomes.rollback(checkpoint12);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test4")), 3);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test5")), 4);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test3")), 2);

  auto checkpoint122 = outcomes.checkpoint();
  std::ostringstream wal122;
  wal122 << (checkpoint122 - checkpoint12) << std::flush;
  ASSERT_EQ(wal121.str(), wal122.str());

  outcomes.rollback(checkpoint);
  ASSERT_EQ(outcomes.find_or_emplace(std::string("test1")), 0);

  auto checkpoint2 = outcomes.checkpoint();
  std::ostringstream wal2;
  wal2 << (checkpoint2 - checkpoint) << std::flush;
  ASSERT_EQ(wal1.str(), wal2.str());
}
}  // namespace
