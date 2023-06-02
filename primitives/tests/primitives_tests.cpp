#include <gtest/gtest.h>

#include "primitives/primitives.h"

namespace {
TEST(TestPrimitives, LabelViewSet__add) {
  using PromPP::Primitives::LabelView;
  using PromPP::Primitives::LabelViewSet;

  LabelViewSet ls;

  ls.add(LabelView("a", "b"));
  EXPECT_EQ(ls.size(), 1);

  ls.add(LabelView("a", "c"));
  EXPECT_EQ(ls.size(), 1);
  EXPECT_EQ(std::get<1>(*ls.begin()), "c");

  ls.add(LabelView("d", "a"));
  EXPECT_EQ(ls.size(), 2);

  ls.add(LabelView("d", "c"));
  EXPECT_EQ(ls.size(), 2);
  EXPECT_EQ(std::get<1>(*(ls.begin() + 1)), "c");

  ls.add(LabelView("b", "f"));
  EXPECT_EQ(ls.size(), 3);
  EXPECT_EQ(std::get<1>(*(ls.begin() + 1)), "f");

  ls.add(LabelView("a", "eee"));
  EXPECT_EQ(ls.size(), 3);
  EXPECT_EQ(std::get<1>(*ls.begin()), "eee");
}
}  // namespace
