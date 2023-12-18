#include <gtest/gtest.h>
#include <algorithm>
#include <string>

#include "primitives/go_slice.h"

namespace {
TEST(TestGoSlice, write_and_read) {
  using PromPP::Primitives::Go::BytesStream;
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::SliceView;

  Slice<char> s;
  BytesStream bs(&s);

  std::string test = "Hello!";
  bs.write(test.data(), test.size());

  SliceView<char>* sv = reinterpret_cast<SliceView<char>*>(&s);

  EXPECT_EQ(test, std::string_view(sv->data(), sv->size()));
}
}  // namespace
