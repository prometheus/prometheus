#include <gtest/gtest.h>

#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "primitives/snug_composites.h"

namespace {

struct TestLabelsBuilder : public testing::Test {
  PromPP::Primitives::LabelsBuilderStateMap builder_state_;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelViewSet, PromPP::Primitives::LabelsBuilderStateMap> builder_ =
      PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelViewSet, PromPP::Primitives::LabelsBuilderStateMap>(builder_state_);
  PromPP::Primitives::LabelViewSet ls_view_;
  PromPP::Primitives::LabelSet ls_;
  std::vector<std::pair<std::string, std::string>> DATA{{"qwe", "ewq"}, {"asd", "dsa"}, {"zxc", "cxz"}};
};

TEST_F(TestLabelsBuilder, Reset) {
  EXPECT_EQ(builder_.label_view_set().size(), 0);
  EXPECT_EQ(builder_.label_set().size(), 0);

  for (auto& [lname, lvalue] : DATA) {
    ls_view_.add({lname, lvalue});
    ls_.add({lname, lvalue});
  }

  builder_.reset(&ls_view_);

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, BaseWithEmptyLValue) {
  EXPECT_EQ(builder_.label_view_set().size(), 0);
  EXPECT_EQ(builder_.label_set().size(), 0);
  PromPP::Primitives::LabelViewSet base;

  for (auto& [lname, lvalue] : DATA) {
    base.add({lname, lvalue});
    ls_view_.add({lname, lvalue});
    ls_.add({lname, lvalue});
  }

  base.add({"lname", ""});

  builder_.reset(&ls_view_);

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, Set) {
  for (auto& [lname, lvalue] : DATA) {
    ls_view_.add({lname, lvalue});
    builder_.set(lname, lvalue);
    ls_.add({lname, lvalue});
  }

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, SetEmpty) {
  for (size_t i = 0; i < DATA.size(); ++i) {
    builder_.set(DATA[i].first, DATA[i].second);
    // skip first element for compare
    if (i == 0) {
      continue;
    }
    ls_view_.add({DATA[i].first, DATA[i].second});
    ls_.add({DATA[i].first, DATA[i].second});
  }

  std::string empty = "";
  builder_.set(DATA[0].first, empty);

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);

  EXPECT_EQ(builder_.label_view_set().size(), ls_view_.size());
  EXPECT_EQ(builder_.label_set().size(), ls_.size());
}

TEST_F(TestLabelsBuilder, SetChange) {
  for (auto& [lname, lvalue] : DATA) {
    ls_view_.add({lname, lvalue});
    builder_.set(lname, lvalue);
    ls_.add({lname, lvalue});
  }

  std::string value = "zxcv";
  builder_.set(DATA[0].first, value);

  EXPECT_NE(builder_.label_view_set(), ls_view_);
  EXPECT_NE(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, Get) {
  for (auto& [lname, lvalue] : DATA) {
    ls_view_.add({lname, lvalue});
    builder_.set(lname, lvalue);
  }

  for (auto& [lname, lvalue] : ls_view_) {
    std::string_view b_lvalue = builder_.get(lname);
    EXPECT_EQ(b_lvalue, lvalue);
  }
}

TEST_F(TestLabelsBuilder, Del) {
  for (auto& [lname, lvalue] : DATA) {
    builder_.set(lname, lvalue);
  }

  builder_.del(DATA[0].first);

  std::string_view b_lvalue = builder_.get(DATA[0].first);

  EXPECT_EQ(b_lvalue, "");
}

TEST_F(TestLabelsBuilder, SetDelSet) {
  for (auto& [lname, lvalue] : DATA) {
    ls_view_.add({lname, lvalue});
    builder_.set(lname, lvalue);
    ls_.add({lname, lvalue});
  }

  builder_.del(DATA[0].first);
  builder_.set(DATA[0].first, DATA[0].second);

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, Range) {
  for (auto& [lname, lvalue] : DATA) {
    ls_view_.add({lvalue, lname});
    builder_.set(lname, lvalue);
    ls_.add({lvalue, lname});
  }

  builder_.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    builder_.del(lname);
    builder_.set(lvalue, lname);
    return true;
  });

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, RangeFastExit) {
  for (size_t i = 0; i < DATA.size(); ++i) {
    builder_.set(DATA[i].first, DATA[i].second);
    if (i == 2) {
      // for last label not swap lname and lvalue
      ls_view_.add({DATA[i].first, DATA[i].second});
      ls_.add({DATA[i].first, DATA[i].second});
      continue;
    }
    ls_view_.add({DATA[i].second, DATA[i].first});
    ls_.add({DATA[i].second, DATA[i].first});
  }

  size_t count{0};
  builder_.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    builder_.del(lname);
    builder_.set(lvalue, lname);
    ++count;
    if (count == 2) {
      return false;
    };
    return true;
  });

  EXPECT_EQ(count, 2);
  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, ResetRange) {
  PromPP::Primitives::LabelViewSet ls;
  for (auto& [lname, lvalue] : DATA) {
    ls.add({lname, lvalue});
    ls_view_.add({lvalue, lname});
    ls_.add({lvalue, lname});
  }

  builder_.reset(&ls);

  builder_.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    builder_.del(lname);
    builder_.set(lvalue, lname);
    return true;
  });

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_EQ(builder_.label_set(), ls_);
}

TEST_F(TestLabelsBuilder, NotIsEmpty) {
  for (auto& [lname, lvalue] : DATA) {
    builder_.set(lname, lvalue);
  }

  EXPECT_EQ(builder_.size(), 3);
  EXPECT_FALSE(builder_.is_empty());
}

TEST_F(TestLabelsBuilder, IsEmpty) {
  EXPECT_EQ(builder_.size(), 0);
  EXPECT_TRUE(builder_.is_empty());
}

TEST_F(TestLabelsBuilder, Contains) {
  for (auto& [lname, lvalue] : DATA) {
    ls_view_.add({lname, lvalue});
    builder_.set(lname, lvalue);
  }

  EXPECT_EQ(builder_.label_view_set(), ls_view_);
  EXPECT_TRUE(builder_.contains(DATA[0].first));
}

};  // namespace
