#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wunknown-pragmas"
#pragma GCC optimize("no-var-tracking")  // to speed up compilation

#include <gtest/gtest.h>
#include <cstdint>
#include <initializer_list>
#include <numeric>
#include <ostream>
#include <string>
#include <string_view>
#include <vector>

#include "parallel_hashmap/btree.h"

#define PROTOZERO_USE_VIEW std::string_view
#include "third_party/protozero/pbf_writer.hpp"

#include "prometheus/relabeler.h"
#include "prometheus/remote_write.h"

namespace {

using namespace PromPP::Prometheus;  // NOLINT

using LabelViewForTest = std::pair<std::string_view, std::string_view>;
using LabelForTest = std::pair<std::string, std::string>;

class SampleForTest {
 private:
  int64_t timestamp_;
  double value_;

 public:
  PROMPP_ALWAYS_INLINE SampleForTest(int64_t timestamp, double value) noexcept : timestamp_(timestamp), value_(value) {}

  PROMPP_ALWAYS_INLINE SampleForTest() noexcept = default;

  int64_t timestamp() const noexcept { return timestamp_; }
  int64_t& timestamp() noexcept { return timestamp_; }

  double value() const noexcept { return value_; }
  double& value() noexcept { return value_; }
};

class NamesSetForTest : public std::vector<std::string_view> {
  using Base = std::vector<std::string_view>;

 public:
  using Base::Base;
  [[maybe_unused]] friend size_t hash_value(const NamesSetForTest& lns) {
    size_t res = 0;
    for (const auto& label_name : lns) {
      res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res);
    }
    return res;
  }
};

class LabelViewSetForTest : public std::vector<LabelViewForTest> {
  using Base = std::vector<LabelViewForTest>;

 public:
  using Base::Base;

  PROMPP_ALWAYS_INLINE void add(const LabelViewForTest& label) noexcept {
    if (__builtin_expect(Base::empty() || std::get<0>(label) > std::get<0>(Base::back()), true)) {
      Base::emplace_back(label);
    } else if (__builtin_expect(std::get<0>(label) == std::get<0>(Base::back()), false)) {
      std::get<1>(Base::back()) = std::get<1>(label);
    } else {
      auto i = std::lower_bound(Base::begin(), Base::end(), std::get<0>(label),
                                [](const LabelViewForTest& a, const std::string_view& b) { return std::get<0>(a) < b; });
      if (__builtin_expect(std::get<0>(*i) == std::get<0>(label), false)) {
        std::get<1>(*i) = std::get<1>(label);
      } else {
        Base::insert(i, label);
      }
    }
  }

  NamesSetForTest names() const {
    NamesSetForTest tns;

    for (auto [label_name, _] : *this) {
      tns.push_back(label_name);
    }

    return tns;
  }

  [[maybe_unused]] friend size_t hash_value(const LabelViewSetForTest& tls) {
    size_t res = 0;
    for (const auto& [label_name, label_value] : tls) {
      res = XXH3_64bits_withSeed(label_name.data(), label_name.size(), res) ^ XXH3_64bits_withSeed(label_value.data(), label_value.size(), res);
    }
    return res;
  }
};

class TimeseriesForTest {
  LabelViewSetForTest label_set_;
  std::vector<SampleForTest> samples_;

 public:
  TimeseriesForTest() noexcept = default;
  TimeseriesForTest(const TimeseriesForTest&) noexcept = default;
  TimeseriesForTest& operator=(const TimeseriesForTest&) noexcept = default;
  TimeseriesForTest(TimeseriesForTest&&) noexcept = default;
  TimeseriesForTest& operator=(TimeseriesForTest&&) noexcept = default;

  TimeseriesForTest(const LabelViewSetForTest& label_set, const std::vector<SampleForTest>& samples) noexcept : label_set_(label_set), samples_(samples) {}

  PROMPP_ALWAYS_INLINE auto& label_set() noexcept { return label_set_; }

  PROMPP_ALWAYS_INLINE const auto& label_set() const noexcept { return label_set_; }

  PROMPP_ALWAYS_INLINE const auto& samples() const noexcept { return samples_; }

  PROMPP_ALWAYS_INLINE auto& samples() noexcept { return samples_; }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    label_set().clear();
    samples().clear();
  }
};

// LabelsBuilderForTest - builder for label set.
template <class LabelSet>
class LabelsBuilderForTest {
  LabelSet* base_ = nullptr;
  LabelViewSetForTest buf_;
  std::vector<LabelForTest> add_;
  std::vector<std::string> del_;

 public:
  PROMPP_ALWAYS_INLINE LabelsBuilderForTest() noexcept {}

  // del - add label name to remove from label set.
  PROMPP_ALWAYS_INLINE void del(std::string& lname) {
    std::erase_if(add_, [lname](LabelForTest& lv) { return lv.first == lname; });

    if (auto i = std::ranges::find_if(del_.begin(), del_.end(), [lname](const std::string_view& ln) { return ln == lname; }); i != del_.end()) {
      return;
    }

    del_.emplace_back(lname);
  }

  // del - add label name to remove from label set.
  PROMPP_ALWAYS_INLINE void del(std::string_view lname) {
    std::erase_if(add_, [lname](LabelForTest& lv) { return lv.first == lname; });

    if (auto i = std::ranges::find_if(del_.begin(), del_.end(), [lname](const std::string_view& ln) { return ln == lname; }); i != del_.end()) {
      return;
    }

    del_.emplace_back(lname);
  }

  // get - returns the value for the label with the given name. Returns an empty string if the label doesn't exist.
  PROMPP_ALWAYS_INLINE std::string_view get(std::string_view lname) {
    if (auto i = std::ranges::find_if(add_.begin(), add_.end(), [lname](const LabelForTest& l) { return l.first == lname; }); i != add_.end()) {
      return (*i).second;
    }

    if (auto i = std::ranges::find_if(del_.begin(), del_.end(), [lname](const std::string_view& ln) { return ln == lname; }); i != del_.end()) {
      return "";
    }

    if (base_ != nullptr) [[likely]] {
      for (const auto& [ln, lv] : *base_) {
        if (ln == lname) {
          return lv;
        }
      }
    }

    return "";
  }

  PROMPP_ALWAYS_INLINE bool contains(std::string_view lname) {
    if (auto i = std::ranges::find_if(add_, [&lname](const LabelForTest& l) { return l.first == lname; }); i != add_.end()) {
      return true;
    }

    if (base_ != nullptr) [[likely]] {
      for (const auto& [ln, lv] : *base_) {
        if (ln == lname) {
          return true;
        }
      }
    }

    return false;
  }

  // returns size of building labels.
  PROMPP_ALWAYS_INLINE size_t size() {
    size_t count{0};
    if (base_ != nullptr) [[likely]] {
      for (const auto& ls : *base_) {
        if (auto i = std::ranges::find_if(add_.begin(), add_.end(), [ls](const LabelForTest& l) { return l.first == ls.first; }); i != add_.end()) {
          continue;
        }

        if (auto i = std::ranges::find_if(del_.begin(), del_.end(), [ls](const std::string_view& ln) { return ln == ls.first; }); i != del_.end()) {
          continue;
        }

        ++count;
      }
    }

    count += add_.size();
    return count;
  }

  // returns true if ls represents an empty set of labels.
  PROMPP_ALWAYS_INLINE bool is_empty() { return size() == 0; }

  // labels - returns the labels from the builder. If no modifications were made, the original labels are returned.
  PROMPP_ALWAYS_INLINE LabelViewSetForTest labels() {
    if (base_ != nullptr) [[likely]] {
      for (const auto& ls : *base_) {
        if (auto i = std::ranges::find_if(add_.begin(), add_.end(), [ls](const LabelForTest& l) { return l.first == ls.first; }); i != add_.end()) {
          continue;
        }

        if (auto i = std::ranges::find_if(del_.begin(), del_.end(), [ls](const std::string_view& ln) { return ln == ls.first; }); i != del_.end()) {
          continue;
        }

        buf_.add(ls);
      }
    }

    if (add_.size() != 0) {
      std::ranges::for_each(add_.begin(), add_.end(), [&](const LabelForTest& l) { buf_.add(l); });
      std::ranges::sort(buf_.begin(), buf_.end(), [](const LabelViewForTest& a, const LabelViewForTest& b) {
        if (a.first == b.first) {
          return a.second < b.second;
        }
        return a.first < b.first;
      });
    }

    return buf_;
  }

  // range - calls f on each label in the builder.
  template <class Callback>
  PROMPP_ALWAYS_INLINE void range(Callback func) {
    // take a copy of add and del, so they are unaffected by calls to set() or del().
    std::vector<LabelViewForTest> cadd;
    cadd.reserve(add_.size());
    std::ranges::copy(add_.begin(), add_.end(), std::back_inserter(cadd));

    std::vector<std::string_view> cdel;
    cdel.reserve(del_.size());
    std::ranges::copy(cdel.begin(), cdel.end(), std::back_inserter(cdel));

    if (__builtin_expect(base_ != nullptr, true)) {
      for (const auto& ls : *base_) {
        if (auto i = std::ranges::find_if(cadd.begin(), cadd.end(), [ls](const LabelViewForTest& l) { return l.first == ls.first; }); i != cadd.end()) {
          continue;
        }

        if (auto i = std::ranges::find_if(cdel.begin(), cdel.end(), [ls](const std::string_view& ln) { return ln == ls.first; }); i != cdel.end()) {
          continue;
        }

        func(ls.first, ls.second);
      }
    }

    std::ranges::for_each(cadd.begin(), cadd.end(), [&](const LabelViewForTest& l) { func(l.first, l.second); });
  }

  // reset - clears all current state for the builder.
  PROMPP_ALWAYS_INLINE void reset() {
    buf_.clear();
    add_.clear();
    del_.clear();
    base_ = nullptr;
  }

  // reset - clears all current state for the builder and init from LabelSet.
  PROMPP_ALWAYS_INLINE void reset(LabelSet* ls) {
    reset();
    base_ = ls;
  }

  // set - the name/value pair as a label. A value of "" means delete that label.
  PROMPP_ALWAYS_INLINE void set(std::string& lname, std::string& lvalue) {
    if (__builtin_expect(lvalue.size() == 0, false)) {
      del(lname);
      return;
    }

    if (auto i = std::ranges::find_if(add_.begin(), add_.end(), [lname](const LabelForTest& l) { return l.first == lname; }); i != add_.end()) {
      (*i).second = lvalue;
      return;
    }

    add_.emplace_back(lname, lvalue);
  }

  // set - the name/value pair as a label. A value of "" means delete that label.
  PROMPP_ALWAYS_INLINE void set(std::string_view lname, std::string& lvalue) {
    if (__builtin_expect(lvalue.size() == 0, false)) {
      del(lname);
      return;
    }

    if (auto i = std::ranges::find_if(add_.begin(), add_.end(), [lname](const LabelForTest& l) { return l.first == lname; }); i != add_.end()) {
      (*i).second = lvalue;
      return;
    }

    add_.emplace_back(lname, lvalue);
  }

  // set - the name/value pair as a label. A value of "" means delete that label.
  PROMPP_ALWAYS_INLINE void set(std::string& lname, std::string_view lvalue) {
    if (__builtin_expect(lvalue.size() == 0, false)) {
      del(lname);
      return;
    }

    if (auto i = std::ranges::find_if(add_.begin(), add_.end(), [lname](const LabelForTest& l) { return l.first == lname; }); i != add_.end()) {
      (*i).second = lvalue;
      return;
    }

    add_.emplace_back(lname, lvalue);
  }

  PROMPP_ALWAYS_INLINE LabelsBuilderForTest(LabelsBuilderForTest&&) noexcept = default;
  PROMPP_ALWAYS_INLINE ~LabelsBuilderForTest() = default;
};

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
  builder.reset(&incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsKeep, rstatus);

  auto rlabels = builder.labels();
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestStatelessRelabeler, KeepEQHard) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  LabelViewSetForTest incoming_labels = make_label_set({{"__name__", "value"}, {"job", "abc"}});

  LabelsBuilderForTest<LabelViewSetForTest> builder;
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);
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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

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
  builder.reset(&incoming_labels);

  Relabel::relabelStatus rstatus = sr.relabeling_process(buf_, builder);
  EXPECT_EQ(Relabel::rsDrop, rstatus);
}

//
// PerShardRelabeler
//

PROMPP_ALWAYS_INLINE std::vector<SampleForTest> make_samples(std::initializer_list<SampleForTest> samples) {
  std::vector<SampleForTest> sampleses;
  for (const SampleForTest& s : samples) {
    sampleses.push_back(s);
  }

  return sampleses;
}

struct ItemTest {
  size_t hash_;
  LabelViewSetForTest labelview_set_;
  std::vector<SampleForTest> samples_;

  PROMPP_ALWAYS_INLINE explicit ItemTest(size_t hash, LabelViewSetForTest& labelview_set, std::vector<SampleForTest>& samples)
      : hash_(hash), labelview_set_(labelview_set), samples_(samples) {}
  PROMPP_ALWAYS_INLINE size_t hash() const { return hash_; }

  template <class Timeseries>
  PROMPP_ALWAYS_INLINE void read(Timeseries& timeseries) const {
    for (const auto& labelview : labelview_set_) {
      timeseries.label_set().add({labelview.first, labelview.second});
    }

    for (const auto& sample : samples_) {
      timeseries.samples().emplace_back(sample.timestamp(), sample.value());
    }
  }
};

class HashdexTest : public std::vector<ItemTest> {
  using Base = std::vector<ItemTest>;

 public:
  using Base::Base;
};

PROMPP_ALWAYS_INLINE void make_hashdex(HashdexTest& hx, LabelViewSetForTest label_set, std::vector<SampleForTest> samples) {
  hx.emplace_back(hash_value(label_set), label_set, samples);
}

struct TestPerShardRelabeler : public testing::Test {
  // shards_inner_series
  std::vector<std::unique_ptr<PromPP::Prometheus::Relabel::InnerSeries>> vector_shards_inner_series_;
  PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series_;

  // relabeled_results
  std::vector<std::unique_ptr<PromPP::Prometheus::Relabel::RelabeledSeries>> vector_relabeled_results_;
  PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> relabeled_results_;

  // external_labels
  std::vector<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> vector_external_labels_;
  PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels_;

  void reset() {
    TearDown();
    SetUp();
  }

  void SetUp() final {
    // shards_inner_series
    vector_shards_inner_series_.emplace_back(std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>());
    vector_shards_inner_series_.emplace_back(std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>());
    shards_inner_series_.reset_to(reinterpret_cast<PromPP::Prometheus::Relabel::InnerSeries**>(vector_shards_inner_series_.data()),
                                  vector_shards_inner_series_.size());

    // relabeled_results
    vector_relabeled_results_.emplace_back(std::make_unique<PromPP::Prometheus::Relabel::RelabeledSeries>());
    vector_relabeled_results_.emplace_back(std::make_unique<PromPP::Prometheus::Relabel::RelabeledSeries>());
    relabeled_results_.reset_to(reinterpret_cast<PromPP::Prometheus::Relabel::RelabeledSeries**>(vector_relabeled_results_.data()),
                                vector_shards_inner_series_.size());

    // external_labels
    external_labels_.reset_to(vector_external_labels_.data(), vector_external_labels_.size());
  }

  void TearDown() final {
    // clear memory
    vector_shards_inner_series_.clear();
    vector_relabeled_results_.clear();
  }
};

TEST_F(TestPerShardRelabeler, KeepEQ) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);

  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  prs.update_relabeler_state(&update_data, 1);

  vector_shards_inner_series_[1] = std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>();
  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
}

TEST_F(TestPerShardRelabeler, KeepEQ_OrderedEncodingBimap) {
  PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  prs.update_relabeler_state(&update_data, 1);

  vector_shards_inner_series_[1] = std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>();
  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
}

TEST_F(TestPerShardRelabeler, KeepEQReset) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  prs.update_relabeler_state(&update_data, 1);

  prs.reset_to(1, 2);
  vector_shards_inner_series_[1] = std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>();
  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
}

TEST_F(TestPerShardRelabeler, KeepNE) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "no-match", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
}

TEST_F(TestPerShardRelabeler, KeepEQNE) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  prs.update_relabeler_state(&update_data, 1);

  reset();
  hx.clear();
  make_hashdex(hx, make_label_set({{"__name__", "value"}, {"job", "abcd"}}), make_samples({{1712567046855, 0.1}}));
  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1 skip
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
}

TEST_F(TestPerShardRelabeler, ReplaceToNewLS2) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .separator = ";",
                        .regex = ".*(o).*",
                        .target_label = "replaced",
                        .replacement = "$1",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(&update_data, 1);

  auto rlabels = lss[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "o"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestPerShardRelabeler, ReplaceToNewLS3) {
  PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap lss;

  RelabelConfigTest rct{.separator = ";", .regex = ".*", .target_label = "replaced", .replacement = "blabla", .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 1);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[0], relabeled_results_[0], &update_data);
  EXPECT_EQ(shards_inner_series_[0]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(&update_data, 1);

  auto rlabels = lss[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "blabla"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestPerShardRelabeler, ReplaceToNewLS2_OrderedEncodingBimap) {
  PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .separator = ";",
                        .regex = ".*(o).*",
                        .target_label = "replaced",
                        .replacement = "$1",
                        .action = 5};  // Replace
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);
  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, nullptr, hx, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(&update_data, 1);

  auto rlabels = lss[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "o"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestPerShardRelabeler, InputRelabelingWithStalenans) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;

  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  std::vector<RelabelConfigTest*> rcts;
  rcts.emplace_back(&rct);

  Relabel::StatelessRelabeler sr(rcts);

  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);

  HashdexTest hx;
  make_hashdex(hx, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));
  PromPP::Prometheus::Relabel::SourceState state = nullptr;

  PromPP::Prometheus::Relabel::SourceState newstate =
      prs.input_relabeling_with_stalenans(lss, hx, shards_inner_series_, relabeled_results_, nullptr, state, 1712567046855);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data(0);
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  prs.update_relabeler_state(&update_data, 1);

  vector_shards_inner_series_[1] = std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>();
  HashdexTest empty_hx;
  PromPP::Primitives::Timestamp stale_ts = 1712567047055;
  newstate = prs.input_relabeling_with_stalenans(lss, empty_hx, shards_inner_series_, relabeled_results_, nullptr, newstate, stale_ts);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[1]->data()[0].samples[0].timestamp(), stale_ts);
  EXPECT_EQ(std::bit_cast<uint64_t>(shards_inner_series_[1]->data()[0].samples[0].value()), std::bit_cast<uint64_t>(BareBones::Encoding::Gorilla::STALE_NAN));
}

struct TestProcessExternalLabels : public testing::Test {
  // external_labels
  std::vector<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> vector_external_labels_;
  PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels_;

  // for init PerShardRelabeler
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss_;
  RelabelConfigTest rct_{};
  std::vector<RelabelConfigTest*> rcts_;

  void SetUp() final { rcts_.emplace_back(&rct_); }

  void add_external_labels(std::vector<std::pair<std::string, std::string>>& list_external_labels) {
    vector_external_labels_.resize(list_external_labels.size());
    for (size_t i = 0; i < vector_external_labels_.size(); i++) {
      vector_external_labels_[i].first.reset_to(list_external_labels[i].first.data(), list_external_labels[i].first.size());
      vector_external_labels_[i].second.reset_to(list_external_labels[i].second.data(), list_external_labels[i].second.size());
    }
    external_labels_.reset_to(vector_external_labels_.data(), vector_external_labels_.size());
  }
};

TEST_F(TestProcessExternalLabels, AddingLSEnd) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{{"c_name", "c_value"}};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({{"a_name", "a_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}, {"c_name", "c_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestProcessExternalLabels, AddingLSBeginning) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{{"a_name", "a_value"}};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({{"c_name", "c_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}, {"c_name", "c_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestProcessExternalLabels, OverrideExistingLabels) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{{"a_name", "b_value"}};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({{"a_name", "a_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestProcessExternalLabels, EmptyExternalLabels) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({{"a_name", "a_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestProcessExternalLabels, EmptyLabels) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{{"a_name", "a_value"}};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestProcessExternalLabels, LabelsLongerExternalLabels) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{{"c_name", "c_value"}};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({{"a_name", "a_value"}, {"b_name", "b_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}, {"b_name", "b_value"}, {"c_name", "c_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestProcessExternalLabels, ExternalLabelsLongerLabels) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{{"a_name", "a_value"}, {"c_name", "c_value"}};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({{"b_name", "b_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}, {"b_name", "b_value"}, {"c_name", "c_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestProcessExternalLabels, AddingWithWithoutClashingLabels) {
  // make external_labels
  std::vector<std::pair<std::string, std::string>> list_external_labels{{"a_name", "a1_value"}, {"b_name", "b1_value"}, {"c_name", "c1_value"}};
  add_external_labels(list_external_labels);

  auto labels = make_label_set({{"a_name", "a_value"}, {"b_name", "b_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder builder = PromPP::Primitives::LabelsBuilder<LabelViewSetForTest, PromPP::Primitives::LabelsBuilderStateMap>(builder_state);
  builder.reset(&labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 0, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}, {"b_name", "b_value"}, {"c_name", "c1_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

struct TestLabelsValidator : public testing::Test {};

TEST_F(TestLabelsValidator, LabelNameIsValid) {
  std::vector<std::string_view> lns{"Avalid_23name", "_Avalid_23name", "avalid_23name"};

  for (const std::string_view lname : lns) {
    EXPECT_TRUE(PromPP::Prometheus::Relabel::label_name_is_valid(lname));
  }
}

TEST_F(TestLabelsValidator, LabelNameIsInValid) {
  std::vector<std::string_view> lns{"", "1valid_23name", "Ava:lid_23name", "a lid_23name", ":leading_colon", "colon:in:the:middle", "a\xc5z"};

  for (const std::string_view lname : lns) {
    EXPECT_FALSE(PromPP::Prometheus::Relabel::label_name_is_valid(lname));
  }
}

TEST_F(TestLabelsValidator, MetricNameValueIsValid) {
  std::vector<std::string_view> lns{"Avalid_23name", "_Avalid_23name", "avalid_23name", "Ava:lid_23name", ":leading_colon", "colon:in:the:middle"};

  for (const std::string_view lname : lns) {
    EXPECT_TRUE(PromPP::Prometheus::Relabel::metric_name_value_is_valid(lname));
  }
}

TEST_F(TestLabelsValidator, MetricNameValueIsInValid) {
  std::vector<std::string_view> lns{"", "1valid_23name", "a lid_23name", "a\xc5z"};

  for (const std::string_view lname : lns) {
    EXPECT_FALSE(PromPP::Prometheus::Relabel::label_name_is_valid(lname));
  }
}

TEST_F(TestLabelsValidator, LabelValueIsValid) {
  std::vector<std::string_view> lvs{
      "Avalid_23name",
      "_Avalid_23name",
      "avalid_23name",
      "",
      "1valid_23name",
      "Ava:lid_23name",
      "a lid_23name",
      ":leading_colon",
      "colon:in:the:middle",
      "ol\xc3\xa1 mundo",
      "\xe4\xbd\xa0\xe5\xa5\xbd\xe4\xb8\x96\xe7\x95\x8c",
      "\x7e",
  };

  for (const std::string_view lvalue : lvs) {
    EXPECT_TRUE(PromPP::Prometheus::Relabel::label_value_is_valid(lvalue));
  }
}

TEST_F(TestLabelsValidator, LabelValueIsInValid) {
  std::vector<std::string_view> lvs{
      "\xa0\xa1",
      "a\xc5z",
      "\x80\x8F\x90\x9FzxcasdAA:",
  };

  for (const std::string_view lvalue : lvs) {
    EXPECT_FALSE(PromPP::Prometheus::Relabel::label_value_is_valid(lvalue));
  }
}

struct TestStaleNaNsState : public testing::Test {};

TEST_F(TestStaleNaNsState, ParentEQ) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;

  PromPP::Prometheus::Relabel::StaleNaNsState* result = new PromPP::Prometheus::Relabel::StaleNaNsState(&lss);
  EXPECT_TRUE(result->parent_eq(&lss));
}

TEST_F(TestStaleNaNsState, NotParentEQ) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss1;
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss2;

  PromPP::Prometheus::Relabel::StaleNaNsState* result = new PromPP::Prometheus::Relabel::StaleNaNsState(&lss1);
  EXPECT_FALSE(result->parent_eq(&lss2));
}

TEST_F(TestStaleNaNsState, ResetTo) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss1;
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss2;

  PromPP::Prometheus::Relabel::StaleNaNsState* result = new PromPP::Prometheus::Relabel::StaleNaNsState(&lss1);
  EXPECT_TRUE(result->parent_eq(&lss1));

  result->reset_to(&lss2);
  EXPECT_TRUE(result->parent_eq(&lss2));
}

TEST_F(TestStaleNaNsState, Swap) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;
  PromPP::Prometheus::Relabel::StaleNaNsState* result = new PromPP::Prometheus::Relabel::StaleNaNsState(&lss);

  uint32_t current_ls_id{42};
  result->add(current_ls_id);

  auto fn = [&](const uint32_t ls_id) { EXPECT_EQ(ls_id, current_ls_id); };
  result->swap(fn);
  result->swap(fn);
}

struct TestLSSWithStaleNaNs : public testing::Test {};

TEST_F(TestLSSWithStaleNaNs, FindOrEmplace) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;
  PromPP::Prometheus::Relabel::StaleNaNsState* result = new PromPP::Prometheus::Relabel::StaleNaNsState(&lss);
  PromPP::Prometheus::Relabel::LSSWithStaleNaNs wrapped_lss(lss, result);

  LabelViewSetForTest label_set = make_label_set({{"__name__", "value"}, {"job", "abs"}});
  uint32_t current_ls_id = wrapped_lss.find_or_emplace(label_set, hash_value(label_set));

  EXPECT_EQ(lss[current_ls_id], label_set);
  auto fn = [&](const uint32_t ls_id) { EXPECT_EQ(ls_id, current_ls_id); };
  result->swap(fn);
  result->swap(fn);
}

}  // namespace
#pragma GCC diagnostic pop
