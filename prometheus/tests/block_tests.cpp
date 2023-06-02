#include <gtest/gtest.h>

#include "prometheus/block.h"

namespace {

TEST(TestBlock, OrderedSeriesList) {
  using PromPP::Primitives::LabelView;
  using PromPP::Prometheus::Block::OrderedSeriesList;
  using PromPP::Prometheus::Block::Series;

  Series x;
  x.label_set().add(LabelView("namespace", "default"));
  x.label_set().add(LabelView("pod", "bar"));
  x.chunks().add(1, 2, 3);
  x.chunks().add(4, 5, 6);

  OrderedSeriesList y;
  y.push_back(x);
  EXPECT_EQ(y.size(), 1);

  x.label_set().add(LabelView("port", "100500"));
  x.chunks().add(8, 8, 8);
  y.push_back(x);
  EXPECT_EQ(y.size(), 2);

  auto i = y.begin();
  EXPECT_NE(i, y.end());
  auto ts1 = *i;
  EXPECT_EQ(ts1.label_set().size(), 2);
  EXPECT_EQ(ts1.chunks().size(), 2);

  ++i;
  EXPECT_NE(i, y.end());
  auto ts2 = *i;
  EXPECT_EQ(ts2.label_set().size(), 3);
  EXPECT_EQ(ts2.chunks().size(), 3);

  ++i;
  EXPECT_EQ(i, y.end());
}
}  // namespace
