#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wunknown-pragmas"
#pragma GCC optimize("no-var-tracking")  // to speed up compilation

#include <gtest/gtest.h>
#include <cstdint>
#include <initializer_list>
#include <ostream>
#include <string>
#include <string_view>
#include <vector>

#include "primitives/snug_composites.h"
#include "prometheus/relabeler.h"

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
  [[maybe_unused]] friend size_t hash_value(const NamesSetForTest& lns) { return PromPP::Primitives::hash::hash_of_string_list(lns); }
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

  template <class SymbolType>
  PROMPP_ALWAYS_INLINE SymbolType get(const SymbolType& label_name) noexcept {
    for (const auto& [ln, lv] : *this) {
      if (ln == label_name) [[unlikely]] {
        return lv;
      }
    }

    return "";
  }

  NamesSetForTest names() const {
    NamesSetForTest tns;

    for (auto [label_name, _] : *this) {
      tns.push_back(label_name);
    }

    return tns;
  }

  [[maybe_unused]] friend size_t hash_value(const LabelViewSetForTest& tls) { return PromPP::Primitives::hash::hash_of_label_set(tls); }
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
  PROMPP_ALWAYS_INLINE void reset(LabelSet& ls) {
    reset();
    base_ = &ls;
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

  // target_labels
  std::vector<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> vector_target_labels_{};

  // Options
  PromPP::Prometheus::Relabel::RelabelerOptions o_;

  // Hashdex
  HashdexTest hx_;

  // LSS
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss_;

  // Cache
  PromPP::Prometheus::Relabel::Cache cache_{};

  void reset() {
    TearDown();
    SetUp();
  }

  void add_target_labels(std::vector<std::pair<std::string, std::string>>& list_target_labels) {
    vector_target_labels_.resize(list_target_labels.size());
    for (size_t i = 0; i < vector_target_labels_.size(); i++) {
      vector_target_labels_[i].first.reset_to(list_target_labels[i].first.data(), list_target_labels[i].first.size());
      vector_target_labels_[i].second.reset_to(list_target_labels[i].second.data(), list_target_labels[i].second.size());
    }
    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size());
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

    // target_labels
    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size());
  }

  void TearDown() final {
    // clear memory
    vector_shards_inner_series_.clear();
    vector_relabeled_results_.clear();
  }
};

TEST_F(TestPerShardRelabeler, KeepEQ) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  vector_shards_inner_series_[1] = std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>();
  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
}

TEST_F(TestPerShardRelabeler, KeepEQ_OrderedEncodingBimap) {
  PromPP::Primitives::SnugComposites::LabelSet::OrderedEncodingBimap lss;
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, lss, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  vector_shards_inner_series_[1] = std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>();
  prs.input_relabeling(lss, lss, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
}

TEST_F(TestPerShardRelabeler, KeepNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "no-match", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
}

TEST_F(TestPerShardRelabeler, KeepEQNE) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  reset();
  hx_.clear();
  make_hashdex(hx_, make_label_set({{"__name__", "value"}, {"job", "abcd"}}), make_samples({{1712567046855, 0.1}}));
  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1 skip
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);
}

TEST_F(TestPerShardRelabeler, ReplaceToNewLS2) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"__name__"},
                        .separator = ";",
                        .regex = ".*(o).*",
                        .target_label = "replaced",
                        .replacement = "$1",
                        .action = 5};  // Replace
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(cache_, &update_data, 1);

  auto rlabels = lss_[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "o"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestPerShardRelabeler, ReplaceToNewLS3) {
  RelabelConfigTest rct{.separator = ";", .regex = ".*", .target_label = "replaced", .replacement = "blabla", .action = 5};  // Replace
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 1);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[0], relabeled_results_[0], &update_data);
  EXPECT_EQ(shards_inner_series_[0]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(cache_, &update_data, 1);

  auto rlabels = lss_[update_data[0].relabeled_ls_id];
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
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}}), make_samples({{1712567046855, 0.1}}));

  prs.input_relabeling(lss, lss, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(cache_, &update_data, 1);

  auto rlabels = lss[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "baj"}, {"replaced", "o"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestPerShardRelabeler, InputRelabelingWithStalenans) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  make_hashdex(hx_, make_label_set({{"__name__", "value"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));
  PromPP::Prometheus::Relabel::StaleNaNsState state{};

  prs.input_relabeling_with_stalenans(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_, state, 1712567046855);

  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 0);

  vector_shards_inner_series_[1] = std::make_unique<PromPP::Prometheus::Relabel::InnerSeries>();
  HashdexTest empty_hx;
  PromPP::Primitives::Timestamp stale_ts = 1712567047055;
  prs.input_relabeling_with_stalenans(lss_, lss_, cache_, empty_hx, o_, shards_inner_series_, relabeled_results_, state, stale_ts);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(PromPP::Primitives::Sample(stale_ts, PromPP::Prometheus::kStaleNan), shards_inner_series_[1]->data()[0].samples[0]);
}

TEST_F(TestPerShardRelabeler, TargetLabels_HappyPath) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 0);
  make_hashdex(hx_, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));
  std::vector<std::pair<std::string, std::string>> list_target_labels{{"a_name", "target_a_value"}, {"z_name", "target_z_value"}};
  add_target_labels(list_target_labels);

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 0);
  EXPECT_EQ(relabeled_results_[1]->size(), 1);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[1], relabeled_results_[1], &update_data);
  EXPECT_EQ(shards_inner_series_[1]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(cache_, &update_data, 1);

  auto rlabels = lss_[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels =
      make_label_set({{"__name__", "booom"}, {"a_name", "target_a_value"}, {"jab", "baj"}, {"job", "abc"}, {"z_name", "target_z_value"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestPerShardRelabeler, TargetLabels_ExportedLabel) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 0);
  make_hashdex(hx_, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));
  std::vector<std::pair<std::string, std::string>> list_target_labels{{"jab", "target_a_value"}, {"z_name", "target_z_value"}};
  add_target_labels(list_target_labels);

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 1);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[0], relabeled_results_[0], &update_data);
  EXPECT_EQ(shards_inner_series_[0]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(cache_, &update_data, 1);

  auto rlabels = lss_[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels =
      make_label_set({{"__name__", "booom"}, {"exported_jab", "baj"}, {"jab", "target_a_value"}, {"job", "abc"}, {"z_name", "target_z_value"}});

  EXPECT_EQ(rlabels, expected_labels);
}

TEST_F(TestPerShardRelabeler, TargetLabels_ExportedLabel_Honor) {
  RelabelConfigTest rct{.source_labels = std::vector<std::string_view>{"job"}, .regex = "abc", .action = 2};  // Keep
  Relabel::StatelessRelabeler sr(std::vector<RelabelConfigTest*>{&rct});
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 0);
  make_hashdex(hx_, make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}}), make_samples({{1712567046855, 0.1}}));
  std::vector<std::pair<std::string, std::string>> list_target_labels{{"jab", "target_a_value"}, {"z_name", "target_z_value"}};
  add_target_labels(list_target_labels);
  o_.honor_labels = true;

  prs.input_relabeling(lss_, lss_, cache_, hx_, o_, shards_inner_series_, relabeled_results_);
  // shard id 1
  EXPECT_EQ(relabeled_results_[0]->size(), 1);
  EXPECT_EQ(relabeled_results_[1]->size(), 0);
  EXPECT_EQ(shards_inner_series_[0]->size(), 0);
  EXPECT_EQ(shards_inner_series_[1]->size(), 0);

  PromPP::Prometheus::Relabel::RelabelerStateUpdate update_data{};
  prs.append_relabeler_series(lss_, shards_inner_series_[0], relabeled_results_[0], &update_data);
  EXPECT_EQ(shards_inner_series_[0]->size(), 1);
  EXPECT_EQ(update_data.size(), 1);

  prs.update_relabeler_state(cache_, &update_data, 1);

  auto rlabels = lss_[update_data[0].relabeled_ls_id];
  LabelViewSetForTest expected_labels = make_label_set({{"__name__", "booom"}, {"jab", "baj"}, {"job", "abc"}, {"z_name", "target_z_value"}});

  EXPECT_EQ(rlabels, expected_labels);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
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
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);

  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  prs.process_external_labels(builder);

  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}, {"b_name", "b_value"}, {"c_name", "c1_value"}});

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

struct TestTargetLabels : public testing::Test {
  // target_labels
  std::vector<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> vector_target_labels_;
  // PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> target_labels_;

  // Options
  PromPP::Prometheus::Relabel::RelabelerOptions o_;

  // for init PerShardRelabeler
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss_;
  std::vector<RelabelConfigTest*> rcts_;
  std::vector<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> vector_external_labels_;
  PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels_;

  void SetUp() final {
    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size());
    external_labels_.reset_to(vector_external_labels_.data(), vector_external_labels_.size());
  }

  void add_target_labels(std::vector<std::pair<std::string, std::string>>& list_target_labels) {
    vector_target_labels_.resize(list_target_labels.size());
    for (size_t i = 0; i < vector_target_labels_.size(); i++) {
      vector_target_labels_[i].first.reset_to(list_target_labels[i].first.data(), list_target_labels[i].first.size());
      vector_target_labels_[i].second.reset_to(list_target_labels[i].second.data(), list_target_labels[i].second.size());
    }
    o_.target_labels.reset_to(vector_target_labels_.data(), vector_target_labels_.size());
  }
};

TEST_F(TestTargetLabels, ResolveConflictingExposedLabels_EmptyConflictingLabels) {
  auto labels = make_label_set({{"c_name", "c_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  std::vector<PromPP::Primitives::Label> conflicting_exposed_labels{};
  LabelViewSetForTest expected_labels = make_label_set({{"c_name", "c_value"}});

  prs.resolve_conflicting_exposed_labels(builder, conflicting_exposed_labels);

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestTargetLabels, ResolveConflictingExposedLabels) {
  auto labels = make_label_set({{"c_name", "c_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  std::vector<PromPP::Primitives::Label> conflicting_exposed_labels{{"c_name", "a_value"}};
  LabelViewSetForTest expected_labels = make_label_set({{"c_name", "c_value"}, {"exported_c_name", "a_value"}});

  prs.resolve_conflicting_exposed_labels(builder, conflicting_exposed_labels);

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestTargetLabels, ResolveConflictingExposedLabels_ExportedLabel) {
  auto labels = make_label_set({{"c_name", "c_value"}});
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  std::vector<PromPP::Primitives::Label> conflicting_exposed_labels{{"exported_c_name", "a_value"}};
  LabelViewSetForTest expected_labels = make_label_set({{"c_name", "c_value"}, {"exported_exported_c_name", "a_value"}});

  prs.resolve_conflicting_exposed_labels(builder, conflicting_exposed_labels);

  EXPECT_EQ(builder.label_view_set(), expected_labels);
  EXPECT_EQ(builder.label_set(), expected_labels);
}

TEST_F(TestTargetLabels, InjectTargetLabels_EmptyLabels) {
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  auto labels = make_label_set({{"c_name", "c_value"}});
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  LabelViewSetForTest expected_labels = make_label_set({{"c_name", "c_value"}});

  bool changed = prs.inject_target_labels(builder, o_);
  const PromPP::Primitives::LabelViewSet& target_label_view_set = builder.label_view_set();

  EXPECT_FALSE(changed);
  EXPECT_EQ(target_label_view_set, expected_labels);
}

TEST_F(TestTargetLabels, InjectTargetLabels_HappyPath) {
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  auto labels = make_label_set({{"c_name", "c_value"}});
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  std::vector<std::pair<std::string, std::string>> list_target_labels{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}};
  add_target_labels(list_target_labels);
  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "target_a_value"}, {"b_name", "target_b_value"}, {"c_name", "c_value"}});

  bool changed = prs.inject_target_labels(builder, o_);
  const PromPP::Primitives::LabelViewSet& target_label_view_set = builder.label_view_set();

  EXPECT_TRUE(changed);
  EXPECT_EQ(target_label_view_set, expected_labels);
}

TEST_F(TestTargetLabels, InjectTargetLabels_ConflictingLabels) {
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  auto labels = make_label_set({{"a_name", "a_value"}, {"c_name", "c_value"}});
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  std::vector<std::pair<std::string, std::string>> list_target_labels{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}};
  add_target_labels(list_target_labels);
  LabelViewSetForTest expected_labels =
      make_label_set({{"a_name", "target_a_value"}, {"b_name", "target_b_value"}, {"c_name", "c_value"}, {"exported_a_name", "a_value"}});

  bool changed = prs.inject_target_labels(builder, o_);
  const PromPP::Primitives::LabelViewSet& target_label_view_set = builder.label_view_set();

  EXPECT_TRUE(changed);
  EXPECT_EQ(target_label_view_set, expected_labels);
}

TEST_F(TestTargetLabels, InjectTargetLabels_ConflictingLabels_ExportedLabel) {
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  auto labels = make_label_set({{"a_name", "a_value"}, {"c_name", "c_value"}});
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  std::vector<std::pair<std::string, std::string>> list_target_labels{{"a_name", "target_a_value"}, {"exported_a_name", "exported_target_a_value"}};
  add_target_labels(list_target_labels);
  LabelViewSetForTest expected_labels = make_label_set(
      {{"a_name", "target_a_value"}, {"c_name", "c_value"}, {"exported_a_name", "exported_target_a_value"}, {"exported_exported_a_name", "a_value"}});

  bool changed = prs.inject_target_labels(builder, o_);
  const PromPP::Primitives::LabelViewSet& target_label_view_set = builder.label_view_set();

  EXPECT_TRUE(changed);
  EXPECT_EQ(target_label_view_set, expected_labels);
}

TEST_F(TestTargetLabels, InjectTargetLabels_ConflictingLabels_Honor) {
  PromPP::Primitives::LabelsBuilderStateMap builder_state;
  PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state};
  auto labels = make_label_set({{"a_name", "a_value"}, {"c_name", "c_value"}});
  builder.reset(labels);
  Relabel::StatelessRelabeler sr(rcts_);
  PromPP::Prometheus::Relabel::PerShardRelabeler prs(external_labels_, &sr, 2, 1);
  std::vector<std::pair<std::string, std::string>> list_target_labels{{"a_name", "target_a_value"}, {"b_name", "target_b_value"}};
  add_target_labels(list_target_labels);
  o_.honor_labels = true;
  LabelViewSetForTest expected_labels = make_label_set({{"a_name", "a_value"}, {"b_name", "target_b_value"}, {"c_name", "c_value"}});

  bool changed = prs.inject_target_labels(builder, o_);
  const PromPP::Primitives::LabelViewSet& target_label_view_set = builder.label_view_set();

  EXPECT_TRUE(changed);
  EXPECT_EQ(target_label_view_set, expected_labels);
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

TEST_F(TestStaleNaNsState, Swap) {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap lss;
  PromPP::Prometheus::Relabel::StaleNaNsState* result = new PromPP::Prometheus::Relabel::StaleNaNsState();

  uint32_t current_ls_id{42};
  result->add_input(current_ls_id);

  auto fn = [&](const uint32_t ls_id) { EXPECT_EQ(ls_id, current_ls_id); };
  result->swap(fn, [&](const uint32_t ls_id [[maybe_unused]]) {});
  result->swap(fn, [&](const uint32_t ls_id [[maybe_unused]]) {});
  delete result;
}

}  // namespace
#pragma GCC diagnostic pop
