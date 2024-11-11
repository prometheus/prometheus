#pragma once

#include <cstdint>
#include <ostream>
#include <sstream>
#include <string>
#include <string_view>
#include <vector>

#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wold-style-cast"
#pragma GCC diagnostic ignored "-Wpedantic"
#pragma GCC diagnostic ignored "-Wswitch"
#include "re2/re2.h"
#pragma GCC diagnostic pop
#include <simdutf/simdutf.h>
#include "md5/md5.h"

#include "bare_bones/exception.h"
#include "bare_bones/preprocess.h"
#include "primitives/go_slice.h"

namespace PromPP::Prometheus::Relabel {

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
    for (size_t i = 0; i < source_labels_.size(); ++i) {
      std::string_view lv = builder.get(source_labels_[i]);
      if (i == 0) {
        value += std::string(lv);
        continue;
      }
      value += std::string(separator_) + std::string(lv);
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

}  // namespace PromPP::Prometheus::Relabel
