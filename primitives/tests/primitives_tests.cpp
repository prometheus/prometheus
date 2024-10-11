#include <gtest/gtest.h>

#include "primitives/primitives.h"

namespace {

struct TestLabelViewSet : public testing::Test {};

TEST_F(TestLabelViewSet, Add) {
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

TEST_F(TestLabelViewSet, Add_empty_value) {
  using PromPP::Primitives::LabelView;
  using PromPP::Primitives::LabelViewSet;

  LabelViewSet ls;

  ls.add(LabelView("a", "b"));
  EXPECT_EQ(ls.size(), 1);

  ls.add(LabelView("c", ""));
  EXPECT_EQ(ls.size(), 1);
}

TEST_F(TestLabelViewSet, Add_exist_label_empty_value) {
  using PromPP::Primitives::LabelView;
  using PromPP::Primitives::LabelViewSet;

  LabelViewSet ls;

  ls.add(LabelView("a", "b"));
  EXPECT_EQ(ls.size(), 1);

  ls.add(LabelView("a", ""));
  EXPECT_EQ(ls.size(), 1);
  EXPECT_EQ(std::get<1>(*ls.begin()), "b");
}

TEST_F(TestLabelViewSet, Ctor_other) {
  using PromPP::Primitives::Label;
  using PromPP::Primitives::LabelSet;
  using PromPP::Primitives::LabelView;
  using PromPP::Primitives::LabelViewSet;

  LabelSet other_ls{Label{"a", "b"}, Label{"c", "d"}, Label{"e", "f"}};
  EXPECT_EQ(other_ls.size(), 3);

  LabelViewSet ls(other_ls);
  EXPECT_EQ(ls.size(), 3);
  EXPECT_EQ(std::get<1>(*(ls.begin())), "b");
  EXPECT_EQ(std::get<1>(*(ls.begin() + 1)), "d");
  EXPECT_EQ(std::get<1>(*(ls.begin() + 2)), "f");
}

TEST_F(TestLabelViewSet, Ctor_other_empty_value) {
  using PromPP::Primitives::Label;
  using PromPP::Primitives::LabelSet;
  using PromPP::Primitives::LabelView;
  using PromPP::Primitives::LabelViewSet;

  std::initializer_list<Label> other_ls{Label{"a", ""}, Label{"c", "d"}, Label{"e", "f"}};

  LabelViewSet ls(other_ls);
  EXPECT_EQ(ls.size(), 2);
  EXPECT_EQ(std::get<1>(*(ls.begin())), "d");
  EXPECT_EQ(std::get<1>(*(ls.begin() + 1)), "f");
}

}  // namespace
