#include <iterator>
#include <random>
#include <string>
#include <vector>

#include <gtest/gtest.h>

#include "bare_bones/stream_v_byte.h"
#include "bare_bones/vector.h"

namespace {

const size_t NUM_VALUES = 100;

template <class T>
class ContainerTest : public testing::Test {
 public:
  static std::vector<uint32_t> make_data(const char* key) {
    std::vector<uint32_t> data;

    if (strcmp(key, "empty") == 0) {
      return data;
    }
    if (strcmp(key, "zeros") == 0) {
      for (size_t i = 0; i < NUM_VALUES; ++i) {
        data.push_back(0);
      }
      return data;
    }
    if (strcmp(key, "random") == 0) {
      std::mt19937 gen32(testing::UnitTest::GetInstance()->random_seed());
      for (size_t i = 0; i < NUM_VALUES; ++i) {
        data.push_back(gen32());
      }
      return data;
    }
    throw std::invalid_argument(key);
  }
};

typedef testing::Types<BareBones::Vector<uint32_t>,
                       BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec0124>,
                       BareBones::StreamVByte::Sequence<BareBones::StreamVByte::Codec1234> >
    ContainerTypes;
TYPED_TEST_SUITE(ContainerTest, ContainerTypes);

TYPED_TEST(ContainerTest, should_keep_push_back_order) {
  const char* cases[] = {"empty", "zeros", "random"};
  for (auto key : cases) {
    SCOPED_TRACE(key);
    auto data = ContainerTest<TypeParam>::make_data(key);

    TypeParam actual;
    for (auto x : data) {
      actual.push_back(x);
    }
    ASSERT_TRUE(std::ranges::equal(std::begin(data), std::end(data), std::begin(actual), std::end(actual)));
  }
}

TYPED_TEST(ContainerTest, should_dump_and_restore) {
  const char* cases[] = {"empty", "zeros", "random"};
  for (auto key : cases) {
    SCOPED_TRACE(key);
    auto data = ContainerTest<TypeParam>::make_data(key);

    TypeParam src, dst;
    for (auto x : data) {
      src.push_back(x);
    }
    auto save_size = src.save_size();
    std::ostringstream out;
    out << src;
    ASSERT_EQ(out.str().length(), save_size);
    std::istringstream in(out.str());
    in >> dst;

    ASSERT_TRUE(std::ranges::equal(std::begin(src), std::end(src), std::begin(dst), std::end(dst)));
  }
}
}  // namespace
