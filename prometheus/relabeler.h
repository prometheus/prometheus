#pragma once

#include <cstdint>
#include <cstring>
#include <ostream>
#include <sstream>
#include <string>
#include <string_view>

// #include <parallel_hashmap/btree.h>
#include <parallel_hashmap/phmap.h>
#include <simdutf/simdutf.h>
#include <roaring/roaring.hh>
#include "md5/md5.h"

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wold-style-cast"
#pragma GCC diagnostic ignored "-Wpedantic"
#pragma GCC diagnostic ignored "-Wswitch"
#include "re2/re2.h"
#pragma GCC diagnostic pop

#include "bare_bones/allocator.h"
#include "bare_bones/preprocess.h"
#include "bare_bones/vector.h"
#include "primitives/go_slice.h"
#include "primitives/primitives.h"
#include "primitives/snug_composites.h"
#include "prometheus/value.h"

namespace PromPP::Prometheus::Relabel {

// pPatternPartType - is the pattern part type.
enum pPatternPartType : uint8_t {
  // pNoType - unknown type, init state.
  pUnknownType = 0,
  // pGroup - regex id group.
  pGroup,
  // pSting - regex name group.
  pSting,
};

// PatternPart - dismantled pattern.
class PatternPart {
  pPatternPartType type_;
  union {
    std::string_view string_;
    int group_;
  } data_;

 public:
  PROMPP_ALWAYS_INLINE explicit PatternPart(std::string_view s) : type_(pSting), data_{.string_ = s} {}
  PROMPP_ALWAYS_INLINE explicit PatternPart(int g) : type_(pGroup), data_{.group_ = g} {}

  // write - convert parts to out.
  PROMPP_ALWAYS_INLINE void write(std::ostream& out, std::vector<std::string>& groups) {
    if (type_ == pGroup) {
      out << groups[data_.group_];
    } else {
      out << data_.string_;
    }
  }

  ~PatternPart() = default;
};

// Regexp - wrapper on re2.
class Regexp {
  // use ptr because re2::RE2 move constructor is delete.
  std::unique_ptr<re2::RE2> re_;

 public:
  // Regexp - work without ("^(?:" + std::string(s) + ")$").
  PROMPP_ALWAYS_INLINE explicit Regexp(const std::string_view& s) noexcept : re_(std::make_unique<re2::RE2>(std::string(s))) {}

  PROMPP_ALWAYS_INLINE Regexp(Regexp&&) noexcept = default;
  PROMPP_ALWAYS_INLINE ~Regexp() = default;

  // number_of_capturing_groups - return the number of capturing sub-patterns, or -1 if the regexp wasn't valid on construction. The overall match ($0) does not
  // count. Use in test.
  PROMPP_ALWAYS_INLINE int number_of_capturing_groups() { return re_->NumberOfCapturingGroups(); }

  // groups - get named capturing groups and number groups.
  PROMPP_ALWAYS_INLINE std::map<std::string, int> groups() {
    // get named capturing groups
    std::map<std::string, int> named_groups = re_->NamedCapturingGroups();
    // add number groups to named capturing groups
    for (int i = 0; i <= number_of_capturing_groups(); ++i) {
      named_groups.emplace(std::to_string(i), i);
    }

    return named_groups;
  }

  // match_to_args - match expression and return result args.
  PROMPP_ALWAYS_INLINE bool match_to_args(std::string_view src, std::vector<std::string>& res) {
    int n = number_of_capturing_groups();

    // search full match to args, where size - number of capturing groups
    res.resize(n + 1);
    res[0] = src;
    std::vector<RE2::Arg> re_args;
    re_args.reserve(n);
    std::vector<RE2::Arg*> re_args_ptr;
    re_args_ptr.reserve(n);
    for (int i = 1; i <= n; ++i) {
      re_args.emplace_back(&res[i]);
      re_args_ptr.emplace_back(&re_args[i - 1]);
    }
    bool ok = RE2::FullMatchN(src, *re_.get(), &re_args_ptr[0], n);
    return ok;
  }

  // replace_with_args - replace in template with incoming args.
  PROMPP_ALWAYS_INLINE std::string replace_with_args(std::stringstream& buf, std::vector<std::string>& args, std::vector<PatternPart>& tmpl) {
    if (tmpl.size() == 0) [[unlikely]] {
      // no template or source data
      return "";
    }

    buf.str("");
    for (auto& val : tmpl) {
      val.write(buf, args);
    }

    return buf.str();
  }

  // replace_full - find match for source and replace in template.
  PROMPP_ALWAYS_INLINE std::string replace_full(std::stringstream& out, std::string_view src, std::vector<PatternPart>& tmpl) {
    if (src.size() == 0 || tmpl.size() == 0) [[unlikely]] {
      // no template or source data
      return "";
    }

    std::vector<std::string> res_args;
    bool ok = match_to_args(src, res_args);
    if (!ok) {
      // no entries in regexp
      return "";
    }

    return replace_with_args(out, res_args, tmpl);
  }

  // full_match - check text for full match regexp.
  PROMPP_ALWAYS_INLINE bool full_match(std::string_view str) { return RE2::FullMatch(str, *re_.get()); }
};

struct GORelabelConfig {
  // source_labels - a list of labels from which values are taken and concatenated with the configured separator in order.
  PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::String> source_labels;
  // separator - is the string between concatenated values from the source labels.
  PromPP::Primitives::Go::String separator;
  // regex - against which the concatenation is matched.
  PromPP::Primitives::Go::String regex;
  // modulus - to take of the hash of concatenated values from the source labels.
  uint64_t modulus;
  // target_label - is the label to which the resulting string is written in a replacement.
  // Regexp interpolation is allowed for the replace action.
  PromPP::Primitives::Go::String target_label;
  // replacement - is the regex replacement pattern to be used.
  PromPP::Primitives::Go::String replacement;
  // action - is the action to be performed for the relabeling.
  uint8_t action;
};

// rAction - is the action to be performed on relabeling.
enum rAction : uint8_t {
  // NoAction - no action, init state.
  rNoAction = 0,
  // Drop - drops targets for which the input does match the regex.
  rDrop,
  // Keep - drops targets for which the input does not match the regex.
  rKeep,
  // DropEqual - drops targets for which the input does match the target.
  rDropEqual,
  // KeepEqual - drops targets for which the input does not match the target.
  rKeepEqual,
  // Replace - performs a regex replacement.
  rReplace,
  // Lowercase - maps input letters to their lower case.
  rLowercase,
  // Uppercase - maps input letters to their upper case.
  rUppercase,
  // HashMod - sets a label to the modulus of a hash of labels.
  rHashMod,
  // LabelMap - copies labels to other labelnames based on a regex.
  rLabelMap,
  // LabelDrop - drops any label matching the regex.
  rLabelDrop,
  // LabelKeep - drops any label not matching the regex.
  rLabelKeep,
};

// relabelStatus resulting relabeling status.
enum relabelStatus : uint8_t {
  // Drop the result should be dropped.
  rsDrop = 0,
  // Invalid the result was invalid.
  rsInvalid,
  // Keep the result should be keeped.
  rsKeep,
  // Relabel the result relabeled and should be keeped.
  rsRelabel,
};

// label_name_is_valid validate label name.
PROMPP_ALWAYS_INLINE bool label_name_is_valid(std::string_view name) {
  if (name.size() == 0) {
    return false;
  }

  if (!std::ranges::all_of(name.begin() + 1, name.end(), [](char c) PROMPP_LAMBDA_INLINE { return std::isalnum(c) || c == '_'; })) {
    return false;
  }

  if (!(std::isalpha(name[0]) || name[0] == '_')) {
    return false;
  }

  return true;
}

// label_value_is_valid validate label value.
PROMPP_ALWAYS_INLINE bool label_value_is_valid(std::string_view value) noexcept {
  return simdutf::validate_utf8(value.data(), value.length());
}

// metric_name_value_is_valid validate value for metric name(__name__).
PROMPP_ALWAYS_INLINE bool metric_name_value_is_valid(std::string_view value) {
  if (value.size() == 0) {
    return false;
  }

  if (!std::ranges::all_of(value.begin() + 1, value.end(), [](char c) PROMPP_LAMBDA_INLINE { return std::isalnum(c) || c == '_' || c == ':'; })) {
    return false;
  }

  if (!(std::isalpha(value[0]) || value[0] == '_' || value[0] == ':')) {
    return false;
  }

  return true;
}

// soft_validate on empty validate label set.
template <class LabelsBuilder>
PROMPP_ALWAYS_INLINE void soft_validate(relabelStatus& rstatus, LabelsBuilder& builder) {
  if (rstatus == rsDrop) {
    return;
  }

  if (builder.is_empty()) [[unlikely]] {
    rstatus = rsDrop;
    return;
  }
};

// MetricLimits limits on label set and samples.
struct MetricLimits {
  size_t label_limit{0};
  size_t label_name_length_limit{0};
  size_t label_value_length_limit{0};
  size_t sample_limit{0};

  PROMPP_ALWAYS_INLINE bool label_limit_exceeded(size_t labels_count) { return label_limit > 0 && labels_count > label_limit; }

  PROMPP_ALWAYS_INLINE bool samples_limit_exceeded(size_t samples_count) { return sample_limit > 0 && samples_count >= sample_limit; }
};

// hard_validate on empty, name label(__name__) mandatory, valid label name and value) validate label set.
template <class LabelsBuilder>
PROMPP_ALWAYS_INLINE void hard_validate(relabelStatus& rstatus, LabelsBuilder& builder, MetricLimits* limits) {
  if (rstatus == rsDrop) {
    return;
  }

  // check on empty labels set
  if (builder.is_empty()) [[unlikely]] {
    rstatus = rsDrop;
    return;
  }

  // check on contains metric name labels set
  if (!builder.contains(kMetricLabelName)) [[unlikely]] {
    rstatus = rsInvalid;
    return;
  }

  // validate labels
  builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    if (lname == kMetricLabelName && !metric_name_value_is_valid(lvalue)) {
      rstatus = rsInvalid;
      return false;
    }

    if (!label_name_is_valid(lname) || !label_value_is_valid(lvalue)) {
      rstatus = rsInvalid;
      return false;
    }

    return true;
  });
  if (rstatus == rsInvalid) [[unlikely]] {
    return;
  }

  if (limits == nullptr) {
    return;
  }

  // check limit len serie
  if (limits->label_limit_exceeded(builder.size())) {
    rstatus = rsInvalid;
    return;
  }

  if (limits->label_name_length_limit == 0 && limits->label_value_length_limit == 0) {
    return;
  }

  // check limit len label name and value
  builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
    if (limits->label_name_length_limit > 0 && lname.size() > limits->label_name_length_limit) {
      rstatus = rsInvalid;
      return false;
    }

    if (limits->label_value_length_limit > 0 && lvalue.size() > limits->label_value_length_limit) {
      rstatus = rsInvalid;
      return false;
    }

    return true;
  });
};

// RelabelConfig - config for relabeling.
class RelabelConfig {
  // source_labels - a list of labels from which values are taken and concatenated with the configured separator in order.
  std::vector<std::string_view> source_labels_;
  // separator - is the string between concatenated values from the source labels.
  std::string_view separator_;
  // regexp - against which the concatenation is matched.
  Regexp regexp_;
  // modulus - to take of the hash of concatenated values from the source labels.
  uint64_t modulus_;
  // target_label - is the label to which the resulting string is written in a replacement.
  // Regexp interpolation is allowed for the replace action.
  std::string_view target_label_;
  // replacement - is the regex replacement pattern to be used.
  std::string_view replacement_;
  // action - is the action to be performed for the relabeling.
  rAction action_;
  // target_label_parts - dismantled target_label.
  std::vector<PatternPart> target_label_parts_;
  // replacement_parts - dismantled replacement.
  std::vector<PatternPart> replacement_parts_;

  // extract - extract from source letter or digit value.
  PROMPP_ALWAYS_INLINE std::string extract(re2::RE2& rgx_validate, std::string_view src) {
    std::string name;
    RE2::PartialMatch(src, rgx_validate, &name);
    return name;
  }

  // is_valid_name - validate source on letter or digit value.
  PROMPP_ALWAYS_INLINE bool is_valid_name(re2::RE2& rgx_validate, std::string_view src) { return RE2::FullMatch(src, rgx_validate); }

  // parse - parse template on parts.
  PROMPP_ALWAYS_INLINE void parse(Regexp& regexp, re2::RE2& rgx_validate, std::string_view tmpl, std::vector<PatternPart>& src_parts) {
    std::map<std::string, int> groups = regexp.groups();
    std::string_view p = std::string_view(tmpl);
    while (true) {
      if (p.size() == 0) {
        break;
      }
      // search '$' and cut before
      size_t i = p.find('$');
      std::string_view substr_p = p.substr(0, i);
      if (substr_p.size() != 0) {
        src_parts.emplace_back(substr_p);
      }
      if (i == std::string_view::npos) {
        break;
      }
      p.remove_prefix(i + 1);
      switch (p[0]) {
        // if contains '$$'
        case '$': {
          // "$"
          src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 1, 1));
          p.remove_prefix(1);
          continue;
        }
        // if contains '{...}'
        case '{': {
          p.remove_prefix(1);
          size_t j = p.find('}');
          if (j == std::string_view::npos) {
            // if '}' not found cut - "${"
            src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 2, 2));
            continue;
          }

          std::string_view g_name = p.substr(0, j);
          auto rec = groups.find(std::string(g_name));
          if (rec != groups.end()) {
            // if g_name found in map add as group(int)
            src_parts.emplace_back(rec->second);
            p.remove_prefix(g_name.size() + 1);
            continue;
          }

          bool ok = is_valid_name(rgx_validate, g_name);
          if (!ok) {
            // if g_name invalid add as is - "${" + std::string{g_name} + "}"
            src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 2, g_name.size() + 3));
            p.remove_prefix(g_name.size() + 1);
            continue;
          }

          // if g_name not found in map and g_name valid - cut g_name
          p.remove_prefix(g_name.size() + 1);

          continue;
        }

        default: {
          // search '$' and extract g_name
          int j = p.find('$');
          std::string_view g_name = p.substr(0, j);
          std::string name = extract(rgx_validate, g_name);
          if (name.size() == 0) {
            // if name invalid add as is - "$"
            src_parts.emplace_back(tmpl.substr(tmpl.size() - p.size() - 1, 1));
            continue;
          }
          auto rec = groups.find(name);
          std::string_view substr_g_name = g_name.substr(name.size(), g_name.size());
          if (rec != groups.end()) {
            // if g_name found in map add as group(int)
            src_parts.emplace_back(rec->second);
            if (substr_g_name.size() != 0) {
              src_parts.emplace_back(substr_g_name);
            }
            p.remove_prefix(g_name.size());
            continue;
          }

          // if g_name not found in map cut g_name
          if (substr_g_name.size() != 0) {
            src_parts.emplace_back(substr_g_name);
          }
          p.remove_prefix(g_name.size());
        }
      }
    }
  }

  // make_hash_uint64 - make uint64 from md5 hash.
  PROMPP_ALWAYS_INLINE uint64_t make_hash_uint64(std::string& src) {
    MD5::MD5 hash;
    unsigned char rawHash[MD5::HashBytes];
    hash.add(src.c_str(), src.size());
    hash.get_hash(rawHash);
    // Use only the last 8 bytes of the hash to give the same result as earlier versions of prom code.
    int shift = 8;
    // need return BigEndian
    uint64_t le = *reinterpret_cast<uint64_t*>(&rawHash[shift]);
    return std::byteswap(le);
  }

 public:
  // RelabelConfig - constructor for RelabelConfig from go-config.
  template <class GORelabelConfig>
  PROMPP_ALWAYS_INLINE explicit RelabelConfig(GORelabelConfig* go_rc) noexcept
      : source_labels_{},
        separator_{static_cast<std::string_view>(go_rc->separator)},
        regexp_(static_cast<std::string_view>(go_rc->regex)),
        modulus_{go_rc->modulus},
        target_label_{static_cast<std::string_view>(go_rc->target_label)},
        replacement_{static_cast<std::string_view>(go_rc->replacement)},
        action_{static_cast<rAction>(go_rc->action)} {
    source_labels_.reserve(go_rc->source_labels.size());
    for (const auto& sl : go_rc->source_labels) {
      source_labels_.emplace_back(static_cast<std::string_view>(sl));
    }

    static re2::RE2 rgx_validate("(^[\\p{N}\\p{L}_]+)");
    parse(regexp_, rgx_validate, target_label_, target_label_parts_);
    parse(regexp_, rgx_validate, replacement_, replacement_parts_);
  }

  PROMPP_ALWAYS_INLINE RelabelConfig(RelabelConfig&&) noexcept = default;

  // source_labels - a list of labels from which values are taken and concatenated with the configured separator in order.
  PROMPP_ALWAYS_INLINE const std::vector<std::string_view>& source_labels() const noexcept { return source_labels_; }

  // separator - is the string between concatenated values from the source labels.
  PROMPP_ALWAYS_INLINE const std::string_view& separator() const noexcept { return separator_; }

  // regexp - against which the concatenation is matched.
  PROMPP_ALWAYS_INLINE const Regexp& regexp() const noexcept { return regexp_; }

  // modulus - to take of the hash of concatenated values from the source labels.
  PROMPP_ALWAYS_INLINE const uint64_t& modulus() const noexcept { return modulus_; }

  // target_label - is the label to which the resulting string is written in a replacement.
  // Regexp interpolation is allowed for the replace action.
  PROMPP_ALWAYS_INLINE const std::string_view& target_label() const noexcept { return target_label_; }

  // replacement - is the regex replacement pattern to be used.
  PROMPP_ALWAYS_INLINE const std::string_view& replacement() const noexcept { return replacement_; }

  // action - is the action to be performed for the relabeling.
  PROMPP_ALWAYS_INLINE const rAction& action() const noexcept { return action_; }

  // target_label_parts - dismantled target_label.
  PROMPP_ALWAYS_INLINE const std::vector<PatternPart>& target_label_parts() const noexcept { return target_label_parts_; }

  // replacement_parts - dismantled replacement.
  PROMPP_ALWAYS_INLINE const std::vector<PatternPart>& replacement_parts() const noexcept { return replacement_parts_; }

  // relabel - building relabeling labels.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabel(std::stringstream& buf, LabelsBuilder& builder) {
    std::string value;
    for (const auto& ln : source_labels_) {
      std::string_view lv = builder.get(ln);
      if (lv.length() == 0) {
        continue;
      }
      value += (value.length() > 0 ? std::string(separator_) : "") + std::string(lv);
    }

    switch (action_) {
      case rDrop: {
        if (regexp_.full_match(value)) {
          return rsDrop;
        }
        break;
      }

      case rKeep: {
        if (!regexp_.full_match(value)) {
          return rsDrop;
        }
        break;
      }

      case rDropEqual: {
        if (builder.get(target_label_) == value) {
          return rsDrop;
        }
        break;
      }

      case rKeepEqual: {
        if (builder.get(target_label_) != value) {
          return rsDrop;
        }
        break;
      }

      case rReplace: {
        std::vector<std::string> res_args;
        bool ok = regexp_.match_to_args(value, res_args);
        if (!ok) {
          break;
        }

        std::string lname = regexp_.replace_with_args(buf, res_args, target_label_parts_);
        if (!label_name_is_valid(lname)) {
          break;
        }
        std::string lvalue = regexp_.replace_with_args(buf, res_args, replacement_parts_);
        if (lvalue.size() == 0) {
          if (builder.contains(lname)) {
            builder.del(lname);
            return rsRelabel;
          }
          break;
        }
        builder.set(lname, lvalue);
        return rsRelabel;
      }

      case rLowercase: {
        std::string lvalue{value};
        std::ranges::transform(lvalue, lvalue.begin(), [](unsigned char c) { return std::tolower(c); });
        builder.set(target_label_, lvalue);
        return rsRelabel;
      }

      case rUppercase: {
        std::string lvalue{value};
        std::ranges::transform(lvalue, lvalue.begin(), [](unsigned char c) { return std::toupper(c); });
        builder.set(target_label_, lvalue);
        return rsRelabel;
      }

      case rHashMod: {
        std::string lvalue{std::to_string(make_hash_uint64(value) % modulus_)};
        builder.set(target_label_, lvalue);
        return rsRelabel;
      }

      case rLabelMap: {
        bool change{false};
        builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
          if (regexp_.full_match(lname)) {
            std::string rlname = regexp_.replace_full(buf, lname, replacement_parts_);
            builder.set(rlname, lvalue);
            change = true;
          }
          return true;
        });
        if (change) {
          return rsRelabel;
        }
        break;
      }

      case rLabelDrop: {
        bool change{false};
        builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, [[maybe_unused]] LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
          if (regexp_.full_match(lname)) {
            builder.del(lname);
            change = true;
          }
          return true;
        });
        if (change) {
          return rsRelabel;
        }
        break;
      }
      case rLabelKeep: {
        bool change{false};
        builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, [[maybe_unused]] LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
          if (!regexp_.full_match(lname)) {
            builder.del(lname);
            change = true;
          }
          return true;
        });
        if (change) {
          return rsRelabel;
        }
        break;
      }

      default: {
        throw BareBones::Exception(0x481dea53751b85c3, "unknown relabel action");
      }
    }

    return rsKeep;
  }

  // ~RelabelConfig - destructor for RelabelConfig from go-config.
  PROMPP_ALWAYS_INLINE ~RelabelConfig() = default;
};

// StatelessRelabeler - stateless relabeler with relabel configs.
//
// configs_ - incoming relabel configs;
class StatelessRelabeler {
  std::vector<RelabelConfig> configs_;

 public:
  // StatelessRelabeler - constructor for StatelessRelabeler, converting go-config.
  template <class GORelabelConfigs>
  PROMPP_ALWAYS_INLINE explicit StatelessRelabeler(const GORelabelConfigs& go_rcfgs) noexcept {
    configs_.reserve(go_rcfgs.size());
    for (const auto go_rcfg : go_rcfgs) {
      configs_.emplace_back(go_rcfg);
    }
  }

  // relabeling_process caller passes a LabelsBuilder containing the initial set of labels, which is mutated by the rules.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabeling_process(std::stringstream& buf, LabelsBuilder& builder) {
    relabelStatus rstatus{rsKeep};
    for (auto& rcfg : configs_) {
      relabelStatus status = rcfg.relabel(buf, builder);
      if (status == rsDrop) {
        return rsDrop;
      }
      if (status == rsRelabel) {
        rstatus = rsRelabel;
      }
    }

    return rstatus;
  }

  // relabeling_process_with_soft_validate caller passes a LabelsBuilder containing the initial set of labels, which is mutated by the rules with soft(on empty)
  // validate label set.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabeling_process_with_soft_validate(std::stringstream& buf, LabelsBuilder& builder) {
    relabelStatus rstatus = relabeling_process(buf, builder);

    if (rstatus == rsDrop) {
      return rsDrop;
    }

    if (builder.is_empty()) [[unlikely]] {
      return rsDrop;
    }

    return rstatus;
  }

  // reset_to reset configs and replace on new converting go-config.
  template <class GORelabelConfigs>
  PROMPP_ALWAYS_INLINE void reset_to(const GORelabelConfigs& go_rcfgs) noexcept {
    configs_.clear();
    configs_.reserve(go_rcfgs.size());
    for (const auto go_rcfg : go_rcfgs) {
      configs_.emplace_back(go_rcfg);
    }
  }

  PROMPP_ALWAYS_INLINE ~StatelessRelabeler() = default;
};

// InnerSerie - timeserie after relabeling.
//
// samples - incoming samples;
// ls_id   - relabeling ls id from lss;
struct InnerSerie {
  BareBones::Vector<PromPP::Primitives::Sample> samples;
  uint32_t ls_id;

  PROMPP_ALWAYS_INLINE bool operator==(const InnerSerie& rt) const noexcept = default;
};

// InnerSeries - vector with relabeled result.
//
// size - number of timeseries processed;
// data - vector with timeseries;
class InnerSeries {
  size_t size_{0};
  std::vector<InnerSerie> data_;

 public:
  PROMPP_ALWAYS_INLINE const std::vector<InnerSerie>& data() const { return data_; }

  PROMPP_ALWAYS_INLINE size_t size() const { return size_; }

  PROMPP_ALWAYS_INLINE void reserve(size_t n) { data_.reserve(n); }

  PROMPP_ALWAYS_INLINE void emplace_back(const BareBones::Vector<PromPP::Primitives::Sample>& samples, const uint32_t& ls_id) {
    data_.emplace_back(samples, ls_id);
    ++size_;
  }

  PROMPP_ALWAYS_INLINE void clear() noexcept {
    data_.clear();
    size_ = 0;
  }
};

// RelabeledSerie - element after relabeling with new ls(for next step).
//
// ls      - relabeling new label set;
// samples - incoming samples;
// hash    - hash sum from ls;
// ls_id   - incoming ls id from lss;
struct RelabeledSerie {
  PromPP::Primitives::LabelSet ls;
  BareBones::Vector<PromPP::Primitives::Sample> samples;
  size_t hash;
  uint32_t ls_id;
};

// RelabeledSeries - vector with relabeling elements.
//
// size - number of timeseries processed;
// data - vector with RelabelElement;
class RelabeledSeries {
  size_t size_{0};
  std::vector<RelabeledSerie> data_;

 public:
  PROMPP_ALWAYS_INLINE const std::vector<RelabeledSerie>& data() const { return data_; }

  PROMPP_ALWAYS_INLINE size_t size() const { return size_; }

  PROMPP_ALWAYS_INLINE void emplace_back(PromPP::Primitives::LabelSet& ls,
                                         const BareBones::Vector<PromPP::Primitives::Sample>& samples,
                                         const size_t hash,
                                         const uint32_t ls_id) {
    data_.emplace_back(ls, samples, hash, ls_id);
    ++size_;
  }
};

// CacheValue - value for cache map.
//
// ls_id    - relabeled ls id;
// shard_id - relabeled shard id;
struct PROMPP_ATTRIBUTE_PACKED CacheValue {
  uint32_t ls_id{};
  uint16_t shard_id{};
};

// IncomingAndRelabeledLsID - for update cache.
struct IncomingAndRelabeledLsID {
  uint32_t incoming_ls_id{};
  uint32_t relabeled_ls_id{};
};

// RelabelerStateUpdate - container for update states.
class RelabelerStateUpdate {
  std::vector<IncomingAndRelabeledLsID> data_;

 public:
  PROMPP_ALWAYS_INLINE explicit RelabelerStateUpdate() {}

  PROMPP_ALWAYS_INLINE const std::vector<IncomingAndRelabeledLsID>& data() const { return data_; }

  PROMPP_ALWAYS_INLINE void emplace_back(const uint32_t incoming_ls_id, uint32_t relabeled_ls_id) { data_.emplace_back(incoming_ls_id, relabeled_ls_id); }

  PROMPP_ALWAYS_INLINE size_t size() const { return data_.size(); }

  PROMPP_ALWAYS_INLINE const IncomingAndRelabeledLsID& operator[](uint32_t i) const {
    assert(i < data_.size());
    return data_[i];
  }
};

class NoOpStaleNaNsState {
 public:
  PROMPP_ALWAYS_INLINE void add_input([[maybe_unused]] uint32_t id) {}
  PROMPP_ALWAYS_INLINE void add_target([[maybe_unused]] uint32_t id) {}

  template <typename InputCallback, typename TargetCallback>
  PROMPP_ALWAYS_INLINE void swap([[maybe_unused]] InputCallback input_fn, [[maybe_unused]] TargetCallback target_fn) {}
};

// StaleNaNsState state for stale nans.
class StaleNaNsState {
  roaring::Roaring input_bitset_{};
  roaring::Roaring target_bitset_{};
  roaring::Roaring prev_input_bitset_{};
  roaring::Roaring prev_target_bitset_{};

 public:
  PROMPP_ALWAYS_INLINE explicit StaleNaNsState() {}

  PROMPP_ALWAYS_INLINE void add_input(uint32_t id) { input_bitset_.add(id); }

  PROMPP_ALWAYS_INLINE void add_target(uint32_t id) { target_bitset_.add(id); }

  template <typename InputCallback, typename TargetCallback>
  PROMPP_ALWAYS_INLINE void swap(InputCallback input_fn, TargetCallback target_fn) {
    prev_input_bitset_ -= input_bitset_;
    for (uint32_t ls_id : prev_input_bitset_) {
      input_fn(ls_id);
    }
    // drop old, store new..
    prev_input_bitset_ = std::move(input_bitset_);

    prev_target_bitset_ -= target_bitset_;
    for (uint32_t ls_id : prev_target_bitset_) {
      target_fn(ls_id);
    }
    // drop old, store new..
    prev_target_bitset_ = std::move(target_bitset_);
  }

  PROMPP_ALWAYS_INLINE void reset() {
    input_bitset_ = roaring::Roaring{};
    target_bitset_ = roaring::Roaring{};
    prev_input_bitset_ = roaring::Roaring{};
    prev_target_bitset_ = roaring::Roaring{};
  }
};

// Cache stateless cache for relabeler.
class Cache {
  size_t cache_allocated_memory_{0};
  // phmap::btree_map<uint32_t,
  //                  PromPP::Prometheus::Relabel::CacheValue,
  //                  std::less<>,
  //                  BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>>
  //     cache_relabel_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>{cache_allocated_memory_}};
  phmap::flat_hash_map<uint32_t,
                       PromPP::Prometheus::Relabel::CacheValue,
                       std::hash<uint32_t>,
                       std::equal_to<>,
                       BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>>
      cache_relabel_{{}, {}, BareBones::Allocator<std::pair<const uint32_t, PromPP::Prometheus::Relabel::CacheValue>>{cache_allocated_memory_}};
  roaring::Roaring cache_keep_{};
  roaring::Roaring cache_drop_{};

 public:
  PROMPP_ALWAYS_INLINE explicit Cache() {}

  // allocated_memory return size of allocated memory for caches.
  PROMPP_ALWAYS_INLINE size_t allocated_memory() const noexcept {
    return cache_allocated_memory_ + cache_keep_.getSizeInBytes() + cache_drop_.getSizeInBytes();
  }

  // add_drop add ls id to drop cache.
  PROMPP_ALWAYS_INLINE void add_drop(const uint32_t ls_id) { cache_drop_.add(ls_id); }

  // add_keep add ls id to keep cache.
  PROMPP_ALWAYS_INLINE void add_keep(const uint32_t ls_id) { cache_keep_.add(ls_id); }

  // add_relabel add ls id to relabel cache.
  PROMPP_ALWAYS_INLINE void add_relabel(const uint32_t ls_id, const uint32_t relabeled_ls_id, const uint16_t relabeled_shard_id) noexcept {
    cache_relabel_.emplace(ls_id, CacheValue{.ls_id = relabeled_ls_id, .shard_id = relabeled_shard_id});
  }

  // run optimization on bitset caches.
  PROMPP_ALWAYS_INLINE void optimize() {
    cache_keep_.runOptimize();
    cache_drop_.runOptimize();
  }

  PROMPP_ALWAYS_INLINE void reset() {
    cache_relabel_.clear();
    cache_keep_ = roaring::Roaring{};
    cache_drop_ = roaring::Roaring{};
  }

  struct CheckResult {
    enum Status : uint8_t {
      kNotFound = 0,
      kDrop = 1,
      kKeep = 2,
      kRelabel = 3,
    };
    Status status{Status::kNotFound};
    uint16_t shard_id{};  // used only for kRelabel status
    uint32_t ls_id{};
  };

  template <class InputLSS, class TargetLSS, class LabelSet>
  PROMPP_ALWAYS_INLINE CheckResult check(const InputLSS& input_lss, const TargetLSS& target_lss, LabelSet& label_set, size_t hash) {
    if (std::optional<uint32_t> ls_id = input_lss.find(label_set, hash); ls_id.has_value()) {
      auto res = check_input(ls_id.value());
      if (res.status != CheckResult::kNotFound) {
        return res;
      }
    }
    if (std::optional<uint32_t> ls_id = target_lss.find(label_set, hash); ls_id.has_value()) {
      return check_target(ls_id.value());
    }
    return {};
  }

  PROMPP_ALWAYS_INLINE CheckResult check_input(uint32_t ls_id) {
    if (cache_drop_.contains(ls_id)) {
      return {.status = CheckResult::Status::kDrop};
    }

    if (auto it = cache_relabel_.find(ls_id); it != cache_relabel_.end()) {
      return {.status = CheckResult::Status::kRelabel, .shard_id = it->second.shard_id, .ls_id = it->second.ls_id};
    }

    return {};
  }

  PROMPP_ALWAYS_INLINE CheckResult check_target(uint32_t ls_id) {
    if (cache_keep_.contains(ls_id)) {
      return {.status = CheckResult::Status::kKeep, .ls_id = ls_id};
    }

    return {};
  }
};

struct RelabelerOptions {
  PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> target_labels{};
  MetricLimits* metric_limits{nullptr};
  bool honor_labels{false};
};

// PerShardRelabeler - relabeler for shard.
//
// buf_                 - stringstream for construct pattern part;
// builder_state_       - state of label set builder;
// timeseries_buf_      - buffer for read incoming timeseries;
// stateless_relabeler_ - shared stateless relabeler, pointer;
// shard_id_            - current shard id;
// log_shards_          - logarithm to the base 2 of total shards count;
class PerShardRelabeler {
  std::stringstream buf_;
  PromPP::Primitives::LabelsBuilderStateMap builder_state_;
  std::vector<PromPP::Primitives::LabelView> external_labels_{};
  PromPP::Primitives::TimeseriesSemiview timeseries_buf_;
  StatelessRelabeler* stateless_relabeler_;
  uint16_t number_of_shards_;
  uint16_t shard_id_;

 public:
  // PerShardRelabeler - constructor. Init only with pre-initialized LSS* and StatelessRelabeler*.
  PROMPP_ALWAYS_INLINE PerShardRelabeler(
      PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>>& external_labels,
      StatelessRelabeler* stateless_relabeler,
      const uint16_t number_of_shards,
      const uint16_t shard_id)
      : stateless_relabeler_(stateless_relabeler), number_of_shards_(number_of_shards), shard_id_(shard_id) {
    if (stateless_relabeler_ == nullptr) [[unlikely]] {
      throw BareBones::Exception(0xabd6db40882fd6aa, "stateless relabeler is null pointer");
    }

    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  // TODO delete after rebuild metrics
  // cache_allocated_memory - return size of allocated memory for cache map.
  PROMPP_ALWAYS_INLINE size_t cache_allocated_memory() const noexcept { return 0; }

 private:
  template <class InputLSS, class TargetLSS, class Hashdex, class StNaNsState>
  PROMPP_ALWAYS_INLINE void input_relabeling_internal(InputLSS& input_lss,
                                                      TargetLSS& target_lss,
                                                      Cache& cache,
                                                      const Hashdex& hashdex,
                                                      const RelabelerOptions& o,
                                                      PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                                      PromPP::Primitives::Go::SliceView<RelabeledSeries*>& shards_relabeled_series,
                                                      StNaNsState& stale_nan_state,
                                                      PromPP::Primitives::Timestamp stale_ts) {
    assert(number_of_shards_ > 0);

    size_t n = (hashdex.size() * 1.2) / (number_of_shards_ * number_of_shards_);
    for (auto i = 0; i < number_of_shards_; ++i) {
      shards_inner_series[i]->reserve(n);
    }

    PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state_};
    size_t samples_count{0};

    for (const auto& item : hashdex) {
      if ((item.hash() % number_of_shards_) != shard_id_) {
        continue;
      }

      timeseries_buf_.clear();
      item.read(timeseries_buf_);

      Cache::CheckResult check_result = cache.check(input_lss, target_lss, timeseries_buf_.label_set(), item.hash());
      switch (check_result.status) {
        case Cache::CheckResult::kNotFound: {
          builder.reset(timeseries_buf_.label_set());
          auto rstatus = relabel(o, builder);
          switch (rstatus) {
            case rsDrop: {
              cache.add_drop(input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash()));
              continue;
            }
            case rsInvalid: {
              cache.add_drop(input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash()));
              continue;
            }
            case rsKeep: {
              auto ls_id = target_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash());
              cache.add_keep(ls_id);
              stale_nan_state.add_target(ls_id);
              shards_inner_series[shard_id_]->emplace_back(timeseries_buf_.samples(), ls_id);
            } break;
            case rsRelabel: {
              auto ls_id = input_lss.find_or_emplace(timeseries_buf_.label_set(), item.hash());
              PromPP::Primitives::LabelSet new_label_set = builder.label_set();
              size_t new_hash = hash_value(new_label_set);
              size_t new_shard_id = new_hash % number_of_shards_;
              stale_nan_state.add_input(ls_id);
              shards_relabeled_series[new_shard_id]->emplace_back(new_label_set, timeseries_buf_.samples(), new_hash, ls_id);
            } break;
          }
        } break;
        case Cache::CheckResult::kKeep: {
          stale_nan_state.add_target(check_result.ls_id);
          shards_inner_series[shard_id_]->emplace_back(timeseries_buf_.samples(), check_result.ls_id);
        } break;
        case Cache::CheckResult::kRelabel: {
          stale_nan_state.add_input(check_result.ls_id);
          shards_inner_series[check_result.shard_id]->emplace_back(timeseries_buf_.samples(), check_result.ls_id);
        } break;
        default:
          continue;
      }

      if (o.metric_limits == nullptr) {
        continue;
      }

      samples_count += calculate_samples(timeseries_buf_.samples());
      if (o.metric_limits->samples_limit_exceeded(samples_count)) {
        break;
      }
    }

    BareBones::Vector<PromPP::Primitives::Sample> smpl{{stale_ts, kStaleNan}};
    stale_nan_state.swap(
        [&](uint32_t ls_id) {
          if (auto res = cache.check_input(ls_id); res.status == Cache::CheckResult::kRelabel) {
            shards_inner_series[res.shard_id]->emplace_back(smpl, res.ls_id);
          }
        },
        [&](uint32_t ls_id) {
          if (auto res = cache.check_target(ls_id); res.status == Cache::CheckResult::kKeep) {
            shards_inner_series[shard_id_]->emplace_back(smpl, res.ls_id);
          }
        });
    cache.optimize();
  }

  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE relabelStatus relabel(const RelabelerOptions& o, LabelsBuilder& builder) {
    bool changed = inject_target_labels(builder, o);

    relabelStatus rstatus = stateless_relabeler_->relabeling_process(buf_, builder);
    hard_validate(rstatus, builder, o.metric_limits);
    if (changed && rstatus == rsKeep) {
      rstatus = rsRelabel;
    }

    return rstatus;
  }

  // calculate_samples counts the number of samples excluding stale_nan.
  PROMPP_ALWAYS_INLINE size_t calculate_samples(const BareBones::Vector<PromPP::Primitives::Sample>& samples) noexcept {
    size_t samples_count{0};
    for (const auto smpl : samples) {
      if (is_stale_nan(smpl.value())) {
        continue;
      }
      ++samples_count;
    }

    return samples_count;
  }

 public:
  // inject_target_labels add labels from target to builder.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE bool inject_target_labels(LabelsBuilder& target_builder, const RelabelerOptions& o) {
    if (o.target_labels.empty()) {
      return false;
    }

    bool changed{false};

    if (o.honor_labels) {
      for (const auto& [lname, lvalue] : o.target_labels) {
        if (target_builder.contains(static_cast<std::string_view>(lname))) [[unlikely]] {
          continue;
        }
        target_builder.set(static_cast<std::string_view>(lname), static_cast<std::string_view>(lvalue));
        changed = true;
      }
      return changed;
    }

    std::vector<PromPP::Primitives::Label> conflicting_exposed_labels;
    for (const auto& [lname, lvalue] : o.target_labels) {
      PromPP::Primitives::Label existing_label = target_builder.extract(static_cast<std::string_view>(lname));
      if (!existing_label.second.empty()) [[likely]] {
        conflicting_exposed_labels.emplace_back(std::move(existing_label));
      }

      // It is now safe to set the target label.
      target_builder.set(static_cast<std::string_view>(lname), static_cast<std::string_view>(lvalue));
      changed = true;
    }

    // resolve conflict
    if (!conflicting_exposed_labels.empty()) {
      resolve_conflicting_exposed_labels(target_builder, conflicting_exposed_labels);
    }

    return changed;
  }

  // resolve_conflicting_exposed_labels add prefix to conflicting label name.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE void resolve_conflicting_exposed_labels(LabelsBuilder& builder, std::vector<PromPP::Primitives::Label>& conflicting_exposed_labels) {
    std::stable_sort(conflicting_exposed_labels.begin(), conflicting_exposed_labels.end(),
                     [](PromPP::Primitives::LabelView a, PromPP::Primitives::LabelView b) { return a.first.size() < b.first.size(); });

    for (auto& [ln, lv] : conflicting_exposed_labels) {
      while (true) {
        ln.insert(0, "exported_");
        if (builder.get(ln).empty()) {
          builder.set(ln, lv);
          break;
        }
      }
    }
  }

  template <class InputLSS, class TargetLSS, class Hashdex>
  PROMPP_ALWAYS_INLINE void input_relabeling(InputLSS& input_lss,
                                             TargetLSS& target_lss,
                                             Cache& cache,
                                             const Hashdex& hashdex,
                                             const RelabelerOptions& o,
                                             PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                             PromPP::Primitives::Go::SliceView<RelabeledSeries*>& shards_relabeled_series) {
    NoOpStaleNaNsState state{};
    input_relabeling_internal(input_lss, target_lss, cache, hashdex, o, shards_inner_series, shards_relabeled_series, state, 0);
  }

  template <class InputLSS, class TargetLSS, class Hashdex>
  PROMPP_ALWAYS_INLINE void input_relabeling_with_stalenans(InputLSS& input_lss,
                                                            TargetLSS& target_lss,
                                                            Cache& cache,
                                                            const Hashdex& hashdex,
                                                            const RelabelerOptions& o,
                                                            PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                                            PromPP::Primitives::Go::SliceView<RelabeledSeries*>& shards_relabeled_series,
                                                            StaleNaNsState& state,
                                                            PromPP::Primitives::Timestamp stale_ts) {
    input_relabeling_internal(input_lss, target_lss, cache, hashdex, o, shards_inner_series, shards_relabeled_series, state, stale_ts);
  }

  PROMPP_ALWAYS_INLINE void input_collect_stalenans(Cache& cache,
                                                    PromPP::Primitives::Go::SliceView<InnerSeries*>& shards_inner_series,
                                                    StaleNaNsState& state,
                                                    PromPP::Primitives::Timestamp stale_ts) {
    BareBones::Vector<PromPP::Primitives::Sample> smpl{{stale_ts, kStaleNan}};
    state.swap(
        [&](uint32_t ls_id) {
          if (auto res = cache.check_input(ls_id); res.status == Cache::CheckResult::kRelabel) {
            shards_inner_series[res.shard_id]->emplace_back(smpl, res.ls_id);
          }
        },
        [&](uint32_t ls_id) {
          if (auto res = cache.check_target(ls_id); res.status == Cache::CheckResult::kKeep) {
            shards_inner_series[shard_id_]->emplace_back(smpl, res.ls_id);
          }
        });
    cache.optimize();
  }

  // append_relabeler_series add relabeled ls to lss, add to result and add to cache update(second stage).
  template <class LSS>
  PROMPP_ALWAYS_INLINE void append_relabeler_series(LSS& lss,
                                                    InnerSeries* inner_series,
                                                    const RelabeledSeries* relabeled_series,
                                                    RelabelerStateUpdate* relabeler_state_update) {
    for (const auto& relabeled_serie : relabeled_series->data()) {
      uint32_t ls_id = lss.find_or_emplace(relabeled_serie.ls, relabeled_serie.hash);

      inner_series->emplace_back(relabeled_serie.samples, ls_id);
      relabeler_state_update->emplace_back(relabeled_serie.ls_id, ls_id);
    }
  }

  // update_relabeler_state - add to cache relabled data(third stage).
  PROMPP_ALWAYS_INLINE void update_relabeler_state(Cache& cache, const RelabelerStateUpdate* relabeler_state_update, const uint16_t relabeled_shard_id) {
    for (const auto& update : relabeler_state_update->data()) {
      cache.add_relabel(update.incoming_ls_id, update.relabeled_ls_id, relabeled_shard_id);
    }
  }

  // processExternalLabels merges externalLabels into ls. If ls contains
  // a label in externalLabels, the value in ls wins.
  template <class LabelsBuilder>
  PROMPP_ALWAYS_INLINE void process_external_labels(LabelsBuilder& builder) {
    if (external_labels_.size() == 0) {
      return;
    }

    std::size_t j{0};
    builder.range([&]<typename LNameType, typename LValueType>(LNameType& lname, [[maybe_unused]] LValueType& lvalue) PROMPP_LAMBDA_INLINE -> bool {
      for (; j < external_labels_.size() && lname > external_labels_[j].first;) {
        builder.set(external_labels_[j].first, external_labels_[j].second);
        ++j;
      }

      if (j < external_labels_.size() && lname == external_labels_[j].first) {
        j++;
      }
      return true;
    });

    for (; j < external_labels_.size(); j++) {
      builder.set(external_labels_[j].first, external_labels_[j].second);
    }
  }

  // output_relabeling - relabeling output series(fourth stage).
  template <class LSS>
  PROMPP_ALWAYS_INLINE void output_relabeling(const LSS& lss,
                                              Cache& cache,
                                              RelabeledSeries* relabeled_series,
                                              PromPP::Primitives::Go::SliceView<InnerSeries*>& incoming_inner_series,
                                              PromPP::Primitives::Go::SliceView<InnerSeries*>& encoders_inner_series) {
    std::ranges::for_each(incoming_inner_series, [&](const InnerSeries* inner_series) PROMPP_LAMBDA_INLINE {
      if (inner_series == nullptr || inner_series->size() == 0) {
        return;
      }

      // TODO move ctor builder from ranges for;
      PromPP::Primitives::LabelsBuilder<PromPP::Primitives::LabelsBuilderStateMap> builder{builder_state_};

      std::ranges::for_each(inner_series->data(), [&](const InnerSerie& inner_serie) PROMPP_LAMBDA_INLINE {
        auto res = cache.check_input(inner_serie.ls_id);
        if (res.status == Cache::CheckResult::kDrop) {
          return;
        }

        if (res.status == Cache::CheckResult::kRelabel) {
          encoders_inner_series[res.shard_id]->emplace_back(inner_serie.samples, res.ls_id);
          return;
        }

        if (inner_serie.ls_id >= lss.size()) [[unlikely]] {
          throw BareBones::Exception(0x7763a97e1717e835, "ls_id out of range: %d size: %d shard_id: %d", inner_serie.ls_id, lss.size(), shard_id_);
        }
        typename LSS::value_type labels = lss[inner_serie.ls_id];
        builder.reset(labels);
        process_external_labels(builder);

        relabelStatus rstatus = stateless_relabeler_->relabeling_process(buf_, builder);
        soft_validate(rstatus, builder);
        if (rstatus == rsDrop) {
          // cache_drop_.add(inner_serie.ls_id);
          cache.add_drop(inner_serie.ls_id);
          return;
        }

        PromPP::Primitives::LabelSet new_label_set = builder.label_set();
        relabeled_series->emplace_back(new_label_set, inner_serie.samples, hash_value(new_label_set), inner_serie.ls_id);
      });
    });

    cache.optimize();
  }

  // reset set new number_of_shards and external_labels.
  PROMPP_ALWAYS_INLINE void reset_to(
      const PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>>& external_labels,
      const uint16_t number_of_shards) {
    number_of_shards_ = number_of_shards;
    external_labels_.clear();
    external_labels_.reserve(external_labels.size());
    for (const auto& [ln, lv] : external_labels) {
      external_labels_.emplace_back(static_cast<std::string_view>(ln), static_cast<std::string_view>(lv));
    }
  }

  PROMPP_ALWAYS_INLINE ~PerShardRelabeler() = default;
};

class WALOutputDecoder {
  PromPP::Primitives::SnugComposites::LabelSet::EncodingTable wal_lss_;
  StatelessRelabeler* stateless_relabeler_;
  PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap output_lss_;
  //
 public:
};

}  // namespace PromPP::Prometheus::Relabel
