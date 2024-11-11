#include <gtest/gtest.h>

#include "prometheus/stateless_relabeler.h"

namespace {

using namespace PromPP::Prometheus;  // NOLINT

// Incoming relabel config.
struct RelabelConfigTest {
  std::vector<std::string_view> source_labels{};
  std::string_view separator{};
  std::string_view regex{};
  uint64_t modulus{0};
  std::string_view target_label{};
  std::string_view replacement{};
  uint8_t action{0};
};

PROMPP_ALWAYS_INLINE LabelViewSetForTest make_label_set(std::initializer_list<LabelViewForTest> lvs) {
  LabelViewSetForTest labels;
  for (const LabelViewForTest& lv : lvs) {
    labels.add(lv);
  }

  return labels;
}

//
// PatternPart
//

struct TestPatternPart : public testing::Test {
  const std::string_view STRING_VALUE = "test_string_value";
  const int GROUP_VALUE = 1;
  std::vector<std::string> GROUPS = {"group_0", "group_1"};

  std::stringstream buf_;
  std::ostream& buf = buf_;

  void SetUp() final {
    buf_.str("");
    buf_.clear();
  }
};

TEST_F(TestPatternPart, StringType) {
  Relabel::PatternPart pp(STRING_VALUE);

  pp.write(buf, GROUPS);

  EXPECT_EQ(buf_.str(), STRING_VALUE);
}

TEST_F(TestPatternPart, GroupType) {
  Relabel::PatternPart pp(GROUP_VALUE);

  pp.write(buf, GROUPS);

  EXPECT_EQ(buf_.str(), GROUPS[GROUP_VALUE]);
}

//
// Regexp
//

struct TestRegexp : public testing::Test {};

TEST_F(TestRegexp, FullMatch) {
  Relabel::Regexp rgx("job");

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 0);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  bool ok = rgx.full_match("job");
  EXPECT_TRUE(ok);
}

TEST_F(TestRegexp, NotMatch) {
  Relabel::Regexp rgx("job");

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 0);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  bool ok = rgx.full_match("jobs");
  EXPECT_FALSE(ok);

  ok = rgx.full_match("jo");
  EXPECT_FALSE(ok);

  ok = rgx.full_match("jos");
  EXPECT_FALSE(ok);

  ok = rgx.full_match("jobs");
  EXPECT_FALSE(ok);

  ok = rgx.full_match("ajobs");
  EXPECT_FALSE(ok);
}

TEST_F(TestRegexp, MatchToArgs0) {
  std::string etalon = "job";
  Relabel::Regexp rgx(etalon);

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 0);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  std::vector<std::string> res_args;
  bool ok = rgx.match_to_args(etalon, res_args);
  EXPECT_TRUE(ok);

  for (const auto& s : res_args) {
    EXPECT_EQ(s, etalon);
  }
}

TEST_F(TestRegexp, MatchToArgs1) {
  std::vector<std::string> etalons{"bar", "bar", "boom"};
  Relabel::Regexp rgx("(b.*)");

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 1);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  etalons_map["1"] = 1;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  for (const auto& e : etalons) {
    std::vector<std::string> res_args;
    bool ok = rgx.match_to_args(e, res_args);
    EXPECT_TRUE(ok);

    for (auto& s : res_args) {
      EXPECT_EQ(s, e);
    }
  }

  std::vector<std::string> res_args;
  bool ok = rgx.match_to_args("job", res_args);
  EXPECT_FALSE(ok);
}

TEST_F(TestRegexp, MatchToArgs1_1) {
  std::vector<std::string> etalons{"foo;bar", "oo", "ba"};
  Relabel::Regexp rgx("f(.*);(.*)r");

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 2);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  etalons_map["1"] = 1;
  etalons_map["2"] = 2;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  std::vector<std::string> res_args;
  bool ok = rgx.match_to_args("foo;bar", res_args);
  EXPECT_TRUE(ok);

  for (size_t i = 0; i < etalons.size(); ++i) {
    EXPECT_EQ(res_args[i], etalons[i]);
  }
}

TEST_F(TestRegexp, MatchToArgsNamed) {
  std::vector<std::string> etalons{"bvc", "bvc"};
  Relabel::Regexp rgx("(?P<name>[a-z]+)");

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 1);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  etalons_map["1"] = 1;
  etalons_map["name"] = 1;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  std::vector<std::string> res_args;
  bool ok = rgx.match_to_args("bvc", res_args);
  EXPECT_TRUE(ok);

  for (size_t i = 0; i < etalons.size(); ++i) {
    EXPECT_EQ(res_args[i], etalons[i]);
  }
}

TEST_F(TestRegexp, MatchToArgsMixed) {
  std::vector<std::string> etalons{"99-bvc", "99", "bvc"};
  Relabel::Regexp rgx("([1-9]+)-(?P<name>[a-z]+)");

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 2);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  etalons_map["1"] = 1;
  etalons_map["2"] = 2;
  etalons_map["name"] = 2;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  std::vector<std::string> res_args;
  bool ok = rgx.match_to_args("99-bvc", res_args);
  EXPECT_TRUE(ok);

  for (size_t i = 0; i < etalons.size(); ++i) {
    EXPECT_EQ(res_args[i], etalons[i]);
  }
}

TEST_F(TestRegexp, NoMatchToArgsMixed) {
  Relabel::Regexp rgx("([1-9]+)-(?P<name>[a-z]+)");

  int n = rgx.number_of_capturing_groups();
  EXPECT_EQ(n, 2);

  std::map<std::string, int> etalons_map;
  etalons_map["0"] = 0;
  etalons_map["1"] = 1;
  etalons_map["2"] = 2;
  etalons_map["name"] = 2;
  std::map<std::string, int> g = rgx.groups();
  EXPECT_EQ(g.size(), etalons_map.size());
  for (const auto& [v, i] : g) {
    EXPECT_EQ(i, etalons_map[v]);
  }

  std::vector<std::string> res_args;
  bool ok = rgx.match_to_args("aaa-bvc", res_args);
  EXPECT_FALSE(ok);
}

//
// RelabelConfig
//

struct TestRelabelConfig : public testing::Test {
  std::vector<std::string> GROUPS = {"group_0", "group_1", "group_2"};

  RelabelConfigTest RCT{.source_labels = std::vector<std::string_view>{"job"},
                        .separator = ";",
                        .regex = "some-([^-]+)-(?P<name>[^,]+)",
                        .modulus = 1000,
                        .target_label = "$1${1}",
                        .replacement = "$2${2}$$2${name}$name+",
                        .action = 1};

  std::stringstream buf_;
  std::ostream& buf = buf_;
};

TEST_F(TestRelabelConfig, Init) {
  Relabel::RelabelConfig rc(&RCT);
  EXPECT_EQ(rc.source_labels(), RCT.source_labels);
  EXPECT_EQ(rc.separator(), RCT.separator);
  EXPECT_EQ(rc.modulus(), RCT.modulus);
  EXPECT_EQ(rc.target_label(), RCT.target_label);
  EXPECT_EQ(rc.replacement(), RCT.replacement);
  EXPECT_EQ(rc.action(), RCT.action);
}

TEST_F(TestRelabelConfig, TargetLabel) {
  Relabel::RelabelConfig rc(&RCT);
  auto tlps = rc.target_label_parts();
  for (auto& tlp : tlps) {
    tlp.write(buf, GROUPS);
  }
  EXPECT_EQ(buf_.str(), "group_1group_1");
}

TEST_F(TestRelabelConfig, Replacement) {
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }
  EXPECT_EQ(buf_.str(), "group_2group_2$2group_2group_2+");
}

TEST_F(TestRelabelConfig, UnknownGroup) {
  RCT.replacement = "${3}";
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }
  EXPECT_EQ(buf_.str(), "");
}

TEST_F(TestRelabelConfig, UnknownGroup2) {
  RCT.replacement = "$3";
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }
  EXPECT_EQ(buf_.str(), "");
}

TEST_F(TestRelabelConfig, UnknownGroupName) {
  RCT.replacement = "${names}";
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }
  EXPECT_EQ(buf_.str(), "");
}

TEST_F(TestRelabelConfig, UnknownGroupName2) {
  RCT.replacement = "$names";
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }
  EXPECT_EQ(buf_.str(), "");
}

TEST_F(TestRelabelConfig, InvalidGroupName) {
  RCT.replacement = "${name+}";
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }

  EXPECT_EQ(buf_.str(), "${name+}");
}

TEST_F(TestRelabelConfig, UnclosedBrace) {
  RCT.replacement = "${name";
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }

  EXPECT_EQ(buf_.str(), "${name");
}

TEST_F(TestRelabelConfig, Dollar) {
  RCT.replacement = "$";
  Relabel::RelabelConfig rc(&RCT);
  auto rps = rc.replacement_parts();
  for (auto& rp : rps) {
    rp.write(buf, GROUPS);
  }

  EXPECT_EQ(buf_.str(), "$");
}

//
// StatelessRelabeler
//

struct TestStatelessRelabeler : public testing::Test {
  std::stringstream buf_;
};

TEST_F(TestStatelessRelabeler, KeepEQ) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, KeepEQHard) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts{&rct};
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  Relabel::hard_validate(rstatus, builder, nullptr);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, KeepEQHardInvalid) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__value__", "value"}, {"job", "abc"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);
  Relabel::hard_validate(rstatus, builder, nullptr);
  EXPECT_EQ(Relabel::rsInvalid, rstatus);
}

TEST_F(TestStatelessRelabeler, KeepEQInvalidLabelLimit) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}, {"jub", "buj"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  // label_limit 0
  Relabel::MetricLimits ll{};
  Relabel::hard_validate(rstatus, builder, &ll);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  // label_limit 2
  ll.label_limit = 2;
  Relabel::hard_validate(rstatus, builder, &ll);
  EXPECT_EQ(Relabel::rsInvalid, rstatus);

  // label_name_length_limit 3
  rstatus = Relabel::rsKeep;
  ll.label_limit = 3;
  ll.label_name_length_limit = 3;
  Relabel::hard_validate(rstatus, builder, &ll);
  EXPECT_EQ(Relabel::rsInvalid, rstatus);

  // label_value_length_limit 3
  rstatus = Relabel::rsKeep;
  ll.label_limit = 3;
  ll.label_name_length_limit = 10;
  ll.label_value_length_limit = 3;
  Relabel::hard_validate(rstatus, builder, &ll);
  EXPECT_EQ(Relabel::rsInvalid, rstatus);
}

TEST_F(TestStatelessRelabeler, KeepRegexpEQ) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .regex = "b.*", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "boom"}, {"job", "abc"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "boom"}, {"job", "abc"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, KeepNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "no-match", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abs"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

TEST_F(TestStatelessRelabeler, KeepNENoLabel) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"jub"}, .regex = "no-match", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abs"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

TEST_F(TestStatelessRelabeler, KeepRegexpNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .regex = "b.*", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "zoom"}, {"job", "abc"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

TEST_F(TestStatelessRelabeler, DropEQ) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 1};  // Drop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

TEST_F(TestStatelessRelabeler, DropRegexpEQ) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .regex = ".*o.*", .action = 1};  // Drop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "boom"}, {"job", "beee"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

TEST_F(TestStatelessRelabeler, DropNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "no-match", .action = 1};  // Drop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abs"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "value"}, {"job", "abs"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, DropRegexpNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .regex = "f|o", .action = 1};  // Drop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "boom"}, {"job", "beee"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "boom"}, {"job", "beee"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, DropRegexpNENoLabel) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"jub"}, .regex = "f|o", .action = 1};  // Drop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "boom"}, {"job", "beee"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "boom"}, {"job", "beee"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, DropEqualEQ) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "job", .action = 3};  // DropEqual
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "main"}, {"job", "main"}, {"instance", "else"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

TEST_F(TestStatelessRelabeler, DropEqualNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "job", .action = 3};  // DropEqual
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "main"}, {"job", "ban"}, {"instance", "else"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);
  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "main"}, {"job", "ban"}, {"instance", "else"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, KeepEqualEQ) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "job", .action = 4};  // KeepEqual
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "main"}, {"job", "main"}, {"instance", "else"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "main"}, {"job", "main"}, {"instance", "else"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, KeepEqualNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "job", .action = 4};  // KeepEqual
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "main"}, {"job", "niam"}, {"instance", "else"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

TEST_F(TestStatelessRelabeler, Lowercase) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "name_lowercase", .action = 6};  // Lowercase
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "lOwEr_123_UpPeR_123_cAsE"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "lOwEr_123_UpPeR_123_cAsE"}, {"name_lowercase", "lower_123_upper_123_case"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, Uppercase) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "name_uppercase", .action = 7};  // Uppercase
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "lOwEr_123_UpPeR_123_cAsE"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "lOwEr_123_UpPeR_123_cAsE"}, {"name_uppercase", "LOWER_123_UPPER_123_CASE"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LowercaseUppercase) {
  RelabelConfigTest lrct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "name_lowercase", .action = 6};  // Lowercase
  RelabelConfigTest urct{.source_labels = std::vector<std::string_view>{"__name__"}, .target_label = "name_uppercase", .action = 7};  // Uppercase
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&lrct);
  rcts.emplace_back(&urct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "lOwEr_123_UpPeR_123_cAsE"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set(
      {{"__name__", "lOwEr_123_UpPeR_123_cAsE"}, {"name_lowercase", "lower_123_upper_123_case"}, {"name_uppercase", "LOWER_123_UPPER_123_CASE"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, HashMod) {
  RelabelConfigTest rct{
      .source_labels = std::vector<std::string_view>{"instance"}, .separator = ";", .modulus = 1000, .target_label = "hash_mod", .action = 8};  // HashMod
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"job", "boj"}, {"instance", "ecnatsni"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "eman"}, {"hash_mod", "72"}, {"job", "boj"}, {"instance", "ecnatsni"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, HashMod2) {
  RelabelConfigTest rct{
      .source_labels = std::vector<std::string_view>{"instance"}, .separator = ";", .modulus = 1000, .target_label = "hash_mod", .action = 8};  // HashMod
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"job", "boj"}, {"instance", "ecna\ntsni"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "eman"}, {"hash_mod", "483"}, {"job", "boj"}, {"instance", "ecna\ntsni"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LabelMap) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{}, .regex = "(j.*)", .replacement = "label_map_${1}", .action = 9};  // LabelMap
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({
      {"__name__", "eman"},
      {"jab", "baj"},
      {"job", "boj"},
      {"label_map_jab", "baj"},
      {"label_map_job", "boj"},
  });

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LabelMap2) {
  RelabelConfigTest rct{.regex = "meta_(ng.*)", .replacement = "${1}", .action = 9};  // LabelMap
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"meta_ng_jab", "baj"}, {"meta_ng_job", "boj"}, {"meta_jzb", "bzj"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels =
      make_label_set({{"__name__", "eman"}, {"meta_ng_jab", "baj"}, {"meta_ng_job", "boj"}, {"meta_jzb", "bzj"}, {"ng_jab", "baj"}, {"ng_job", "boj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LabelDrop) {
  RelabelConfigTest rct{.regex = "(j.*)", .replacement = "label_map_${1}", .action = 10};  // LabelDrop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  EXPECT_FALSE(builder.is_empty());

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "eman"}});
  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LabelDropTransparent) {
  RelabelConfigTest rct{.regex = "(j.*)", .replacement = "label_map_${1}", .action = 10};  // LabelDrop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"hab", "baj"}, {"hob", "boj"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "eman"}, {"hab", "baj"}, {"hob", "boj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LabelDropFullDrop) {
  RelabelConfigTest rct1{.regex = "(j.*)", .action = 10};   // LabelDrop
  RelabelConfigTest rct2{.regex = "(__.*)", .action = 10};  // LabelDrop
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct1);
  rcts.emplace_back(&rct2);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);
  soft_validate(rstatus, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
  EXPECT_TRUE(builder.is_empty());
}

TEST_F(TestStatelessRelabeler, LabelDropFullDropAndAdd) {
  RelabelConfigTest rct1{.regex = "(j.*)", .action = 10};   // LabelDrop
  RelabelConfigTest rct2{.regex = "(__.*)", .action = 10};  // LabelDrop
  RelabelConfigTest rct3{
      .source_labels = std::vector<std::string_view>{"jab"}, .separator = ";", .modulus = 1000, .target_label = "hash_mod", .action = 8};  // HashMod
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct1);
  rcts.emplace_back(&rct2);
  rcts.emplace_back(&rct3);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);
  EXPECT_FALSE(builder.is_empty());
  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"hash_mod", "958"}});
  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LabelKeep) {
  RelabelConfigTest rct{.regex = "(j.*)", .replacement = "label_map_${1}", .action = 11};  // LabelKeep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eman"}, {"jab", "baj"}, {"job", "boj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"jab", "baj"}, {"job", "boj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, LabelKeepTransparent) {
  RelabelConfigTest rct{.regex = "(j.*)", .replacement = "label_map_${1}", .action = 11};  // LabelKeep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"jab", "eman"}, {"job", "baj"}, {"jub", "boj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"jab", "eman"}, {"job", "baj"}, {"jub", "boj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceToNewLS) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .separator = ";",
                        .regex = "e(.*)",
                        .target_label = "replaced",
                        .replacement = "ch${1}-ch${1}",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eoo"}, {"jab", "baj"}, {"job", "boj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "eoo"}, {"jab", "baj"}, {"job", "boj"}, {"replaced", "choo-choo"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceToNewLS2) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .separator = ";",
                        .regex = ".*(o).*",
                        .target_label = "replaced",
                        .replacement = "$1",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "o"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceToNewLS3) {
  RelabelConfigTest rct{.separator = ";", .regex = ".*", .target_label = "replaced", .replacement = "tag", .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "tag"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceFullMatches) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"jub"},
                        .separator = ";",
                        .regex = ".*",
                        .target_label = "replaced",
                        .replacement = "tag",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "tag"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceNoMatches) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"jub"},
                        .separator = ";",
                        .regex = "baj;(.*)g",
                        .target_label = "replaced",
                        .replacement = "tag",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceMatches) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"jab", "job"},
                        .separator = ";",
                        .regex = "baj;(.*)g",
                        .target_label = "replaced",
                        .replacement = "tag",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "bag"}, {"replaced", "tag"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceNoReplacement) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .separator = ";",
                        .regex = "f",
                        .target_label = "replaced",
                        .replacement = "var",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceBlankReplacement) {
  RelabelConfigTest rct{
      .source_labels = std::vector<std::string_view>{"__name__"}, .regex = "(j).*", .target_label = "$1", .replacement = "$2", .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "jazz"}, {"j", "baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "jazz"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceCreateNewFromValue) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .regex = "some-([^-]+)-([^,]+)",
                        .target_label = "${1}",
                        .replacement = "${2}",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "some-job2-boj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "some-job2-boj"}, {"job2", "boj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceInvalidLabelName) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .regex = "some-([^-]+)-([^,]+)",
                        .target_label = "${1}",
                        .replacement = "${2}",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "some-2job-boj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "some-2job-boj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceInvalidReplacement) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .regex = "some-([^-]+)-([^,]+)",
                        .target_label = "${1}",
                        .replacement = "${3}",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "some-job-boj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "some-job-boj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceInvalidTargetLabels) {
  RelabelConfigTest rct1{.source_labels = std::vector<std::string_view>{"__name__"},
                         .regex = "some-([^-]+)-([^,]+)",
                         .target_label = "${3}",
                         .replacement = "${1}",
                         .action = 5};  // Replace
  RelabelConfigTest rct2{.source_labels = std::vector<std::string_view>{"__name__"},
                         .regex = "some-([^-]+)-([^,]+)",
                         .target_label = "${3}",
                         .replacement = "${1}",
                         .action = 5};  // Replace
  RelabelConfigTest rct3{.source_labels = std::vector<std::string_view>{"__name__"},
                         .regex = "some-([^-]+)(-[^,]+)",
                         .target_label = "${3}",
                         .replacement = "${1}",
                         .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct1);
  rcts.emplace_back(&rct2);
  rcts.emplace_back(&rct3);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "some-job-0"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "some-job-0"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceComplexLikeUsecase) {
  RelabelConfigTest rct1{.source_labels = std::vector<std::string_view>{"__meta_sd_tags"},
                         .regex = "(?:.+,|^)path:(/[^,]+).*",
                         .target_label = "__metrics_path__",
                         .replacement = "${1}",
                         .action = 5};  // Replace
  RelabelConfigTest rct2{.source_labels = std::vector<std::string_view>{"__meta_sd_tags"},
                         .regex = "(?:.+,|^)job:([^,]+).*",
                         .target_label = "job",
                         .replacement = "${1}",
                         .action = 5};  // Replace
  RelabelConfigTest rct3{.source_labels = std::vector<std::string_view>{"__meta_sd_tags"},
                         .regex = "(?:.+,|^)label:([^=]+)=([^,]+).*",
                         .target_label = "${1}",
                         .replacement = "${2}",
                         .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct1);
  rcts.emplace_back(&rct2);
  rcts.emplace_back(&rct3);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__meta_sd_tags", "path:/secret,job:some-job,label:jab=baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels =
      make_label_set({{"__meta_sd_tags", "path:/secret,job:some-job,label:jab=baj"}, {"__metrics_path__", "/secret"}, {"job", "some-job"}, {"jab", "baj"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceIssues12283) {
  RelabelConfigTest rct1{.regex = "^__meta_kubernetes_pod_container_port_name$", .action = 10};  // LabelDrop
  RelabelConfigTest rct2{.source_labels = std::vector<std::string_view>{"__meta_kubernetes_pod_annotation_XXX_metrics_port"},
                         .regex = "(.+)",
                         .target_label = "__meta_kubernetes_pod_container_port_name",
                         .replacement = "metrics",
                         .action = 5};  // Replace
  RelabelConfigTest rct3{
      .source_labels = std::vector<std::string_view>{"__meta_kubernetes_pod_container_port_name"}, .regex = "^metrics$", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct1);
  rcts.emplace_back(&rct2);
  rcts.emplace_back(&rct3);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels =
      make_label_set({{"__meta_kubernetes_pod_container_port_name", "foo"}, {"__meta_kubernetes_pod_annotation_XXX_metrics_port", "9091"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels =
      make_label_set({{"__meta_kubernetes_pod_annotation_XXX_metrics_port", "9091"}, {"__meta_kubernetes_pod_container_port_name", "metrics"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, ReplaceWithReplace) {
  RelabelConfigTest rct1{.source_labels = std::vector<std::string_view>{"__name__", "jab"},
                         .separator = ";",
                         .regex = "e(.*);(.*)j",
                         .target_label = "__name__",
                         .replacement = "b${1}${2}m",
                         .action = 5};  // Replace
  RelabelConfigTest rct2{.source_labels = std::vector<std::string_view>{"job", "__name__"},
                         .separator = ";",
                         .regex = "(b).*b(.*)ba(.*)",
                         .target_label = "replaced",
                         .replacement = "$1$2$2$3",
                         .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct1);
  rcts.emplace_back(&rct2);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eoo"}, {"jab", "baj"}, {"job", "baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsRelabel, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "boobam"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "boooom"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, DropReplace) {
  RelabelConfigTest rct1{.source_labels = std::vector<std::string_view>{"__name__"}, .regex = ".*o.*", .action = 1};  // Drop
  RelabelConfigTest rct2{.source_labels = std::vector<std::string_view>{"__name__"},
                         .separator = ";",
                         .regex = "e(.*)",
                         .target_label = "replaced",
                         .replacement = "ch$1-ch$1",
                         .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts{};
  rcts.emplace_back(&rct1);
  rcts.emplace_back(&rct2);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "eoo"}, {"jab", "baj"}, {"job", "baj"}});
  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

}  // namespace
