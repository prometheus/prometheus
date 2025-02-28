#pragma once

#include <memory>
#include <ranges>

#include "re2/prog.h"
#include "re2/regexp.h"

#include "bare_bones/algorithm.h"
#include "bare_bones/preprocess.h"
#include "prometheus/label_matcher.h"
#include "series_index/trie/concepts.h"

namespace series_index::querier {

class RegexpParser {
 public:
  using RegexpPtr = std::unique_ptr<re2::Regexp, void (*)(re2::Regexp*)>;

  [[nodiscard]] PROMPP_ALWAYS_INLINE static re2::Regexp::ParseFlags regexp_parse_flags() {
    return re2::Regexp::NeverCapture | re2::Regexp::MatchNL | re2::Regexp::PerlClasses | re2::Regexp::OneLine | re2::Regexp::PerlX;
  }

  [[nodiscard]] static RegexpPtr parse(std::string_view regexp) {
    re2::RegexpStatus parse_status;
    RegexpPtr rgx(re2::Regexp::Parse(regexp, regexp_parse_flags(), &parse_status), [](re2::Regexp* regexp) { regexp->Decref(); });
    if (!rgx) {
      return rgx;
    }

    if (const auto simplified_rgx = rgx->Simplify(); simplified_rgx != nullptr) {
      rgx.reset(simplified_rgx);
    } else {
      rgx = nullptr;
    }

    return rgx;
  }
};

class RegexpCompiledProg {
 public:
  RegexpCompiledProg() = default;
  explicit RegexpCompiledProg(re2::Regexp* rgx) { compile(rgx); }

  PROMPP_ALWAYS_INLINE bool compile(re2::Regexp* rgx) {
    prog_.reset(rgx->CompileToProg(0));
    return prog_ != nullptr;
  }

  [[nodiscard]] bool full_match(std::string_view str) const {
    // Drastically simplified logic from RE2::Match
    // https://github.com/google/re2/blob/2021-09-01/re2/re2.cc#L616

    if (!prog_) {
      return false;
    }

    bool dfa_failed;

    if (prog_->SearchDFA(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, &dfa_failed, nullptr)) {
      return true;
    }

    if (dfa_failed) {
      if (prog_->IsOnePass()) {
        return prog_->SearchOnePass(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, 0);
      }

      if (prog_->CanBitState() && str.size() <= static_cast<size_t>(256 * 1024 / prog_->list_count())) {
        return prog_->SearchBitState(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, 0);
      }

      return prog_->SearchNFA(str, str, re2::Prog::Anchor::kAnchored, re2::Prog::MatchKind::kFullMatch, nullptr, 0);
    }

    return false;
  }

 private:
  std::unique_ptr<re2::Prog> prog_;
};

class RegexpMatchAnalyzer {
 public:
  enum class Status : uint8_t {
    kError = 0,
    kEmptyMatch,
    kAnythingMatch,
    kAllMatch,
    kAllMatchWithExcludes,
    kPartialMatch,
  };

  [[nodiscard]] static Status analyze(re2::Regexp* regexp) {
    if (!regexp) {
      return Status::kError;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpEmptyMatch) {
      return Status::kEmptyMatch;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpStar && regexp->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
      return Status::kAnythingMatch;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpPlus && regexp->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
      return Status::kAllMatch;
    }

    if (regexp->op() == re2::RegexpOp::kRegexpConcat) {
      if (const auto i = skip_begin_text_operation(regexp); i == skip_end_text_operation(regexp, i)) {
        if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpPlus) {
          if (regexp->sub()[i]->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
            return Status::kAllMatch;
          }
        } else if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpStar) {
          if (regexp->sub()[i]->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
            return Status::kAnythingMatch;
          }
        } else if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpEndText) {
          return Status::kEmptyMatch;
        } else if (regexp->sub()[i]->op() == re2::RegexpOp::kRegexpAlternate) {
          if (has_empty_alternative(regexp->sub()[i])) {
            return Status::kAllMatchWithExcludes;
          }
        }
      }
    } else if (regexp->op() == re2::RegexpOp::kRegexpAlternate) {
      if (has_empty_alternative(regexp)) {
        return Status::kAllMatchWithExcludes;
      }
    }

    return Status::kPartialMatch;
  }

  [[nodiscard]] static int skip_begin_text_operation(re2::Regexp* regexp) noexcept {
    int i = 0;
    while (i < regexp->nsub() && regexp->sub()[i]->op() == re2::RegexpOp::kRegexpBeginText) {
      ++i;
    }

    return i;
  }

  [[nodiscard]] static int skip_end_text_operation(re2::Regexp* regexp, int start) noexcept {
    int i = regexp->nsub() - 1;
    while (i > start && regexp->sub()[i]->op() == re2::RegexpOp::kRegexpEndText) {
      --i;
    }
    return i;
  }

 private:
  [[nodiscard]] PROMPP_ALWAYS_INLINE static bool has_empty_alternative(re2::Regexp* regexp) noexcept {
    using enum re2::RegexpOp;

    for (auto i = 0; i < regexp->nsub(); ++i) {
      if (const auto alternative = regexp->sub()[i]; alternative->nsub() == 0) {
        if (BareBones::is_in(alternative->op(), kRegexpEmptyMatch, kRegexpBeginText, kRegexpEndText)) {
          return true;
        }
      } else {
        if (const auto start = skip_begin_text_operation(alternative); alternative->sub()[start]->op() == kRegexpEndText) {
          return true;
        }
      }
    }

    return false;
  }
};

template <class Trie, trie::RegexpMatchesListInterface<typename Trie::Traversal> MatchesList>
class RegexpSearcher {
 public:
  explicit RegexpSearcher(MatchesList& matches) : matches_(matches) {}

  [[nodiscard]] PromPP::Prometheus::MatchStatus search(const Trie& trie, const RegexpParser::RegexpPtr& regexp) {
    auto matches_count_before = matches_.count();
    process_subtrie(kProcessSubTrieDepthLimit, trie.make_traversal(), regexp.get());
    if (matches_.count() == matches_count_before) {
      return PromPP::Prometheus::MatchStatus::kEmptyMatch;
    }

    return PromPP::Prometheus::MatchStatus::kPartialMatch;
  }

 private:
  static constexpr uint8_t kProcessSubTrieDepthLimit = 50;

  MatchesList& matches_;
  re2::Regexp* prepared_for_ = nullptr;
  RegexpCompiledProg prepared_prog_;

  void process_subtrie(uint8_t depth_limit, const typename Trie::Traversal& trv, re2::Regexp* rgx) {
    if (depth_limit == 0) {
      process_subtrie_by_regexp(trv, rgx);
      return;
    }

    switch (rgx->op()) {
      case re2::RegexpOp::kRegexpAlternate:
        for (std::size_t i = 0; i < static_cast<size_t>(rgx->nsub()); i++) {
          process_subtrie(depth_limit - 1, trv, rgx->sub()[i]);
        }
        break;

      case re2::RegexpOp::kRegexpConcat: {
        const auto i = RegexpMatchAnalyzer::skip_begin_text_operation(rgx);
        switch (rgx->sub()[i]->op()) {
          case re2::RegexpOp::kRegexpLiteral:
          case re2::RegexpOp::kRegexpLiteralString:
          case re2::RegexpOp::kRegexpCharClass: {
            if (rgx->nsub() - i > 2) {
              for (auto j = i + 1; j < rgx->nsub(); j++) {
                rgx->sub()[j]->Incref();
              }
              const auto rgx_tail = re2::Regexp::Concat(rgx->sub() + i + 1, rgx->nsub() - i - 1, rgx->parse_flags());
              process_exact_prefix(depth_limit, trv, rgx->sub()[i], rgx_tail);
              rgx_tail->Decref();
            } else {
              process_exact_prefix(depth_limit, trv, rgx->sub()[i], rgx->sub()[i + 1]);
            }
            break;
          }
          default: {
            process_subtrie_by_regexp(trv, rgx);
          }
        }

        break;
      }

      case re2::RegexpOp::kRegexpLiteral:
      case re2::RegexpOp::kRegexpLiteralString:
      case re2::RegexpOp::kRegexpCharClass: {
        process_exact_prefix(depth_limit, trv, rgx);
        break;
      }
      case re2::RegexpOp::kRegexpEmptyMatch: {
        process_one_exact_prefix(depth_limit, trv, "");
        break;
      }
      default: {
        process_subtrie_by_regexp(trv, rgx);
      }
    }
  }

  void process_exact_prefix(uint8_t depth_limit, const typename Trie::Traversal& trv, re2::Regexp* rgx, re2::Regexp* rgx_tail = nullptr) {
    char buf[re2::UTFmax + 1];

    // Do simple full scan if it's a case-insensitive regex
    if (rgx->parse_flags() & re2::Regexp::FoldCase) {
      if (rgx_tail) {
        std::array rgxs{rgx->Incref(), rgx_tail->Incref()};
        const auto concat_rgx = re2::Regexp::Concat(rgxs.data(), rgxs.size(), rgx->parse_flags());
        process_subtrie_by_regexp(trv, concat_rgx);
        concat_rgx->Decref();
      } else {
        process_subtrie_by_regexp(trv, rgx);
      }
      return;
    }

    switch (rgx->op()) {
      case re2::RegexpOp::kRegexpLiteral: {
        const auto& r = rgx->rune();
        process_one_exact_prefix(depth_limit, trv, std::string_view(buf, re2::runetochar(buf, &r)), rgx_tail);
        break;
      }
      case re2::RegexpOp::kRegexpLiteralString: {
        std::string literal;
        if (rgx->parse_flags() & re2::Regexp::Latin1) {
          literal.resize(rgx->nrunes());
          for (int i = 0; i < rgx->nrunes(); i++) {
            literal[i] = static_cast<char>(rgx->runes()[i]);
          }
        } else {
          literal.resize(static_cast<size_t>(rgx->nrunes()) * re2::UTFmax);
          char* p = &literal[0];
          for (int i = 0; i < rgx->nrunes(); i++)
            p += re2::runetochar(p, rgx->runes() + i);
          literal.resize(p - &literal[0]);
        }
        process_one_exact_prefix(depth_limit, trv, literal, rgx_tail);
        break;
      }
      case re2::RegexpOp::kRegexpCharClass: {
        if (rgx->cc()->size() < 100) {
          for (const auto& rr : *rgx->cc()) {
            for (auto r = rr.lo; r <= rr.hi; ++r) {
              process_one_exact_prefix(depth_limit, trv, std::string_view(buf, re2::runetochar(buf, &r)), rgx_tail);
            }
          }
        } else {
          if (!rgx_tail) {
            process_subtrie_by_regexp(trv, rgx);
          } else {
            std::array rgxs{rgx->Incref(), rgx_tail->Incref()};
            const auto concat_rgx = re2::Regexp::Concat(rgxs.data(), rgxs.size(), rgx->parse_flags());
            process_subtrie_by_regexp(trv, concat_rgx);
            concat_rgx->Decref();
          }
        }
        break;
      }

      default: {
        // can't get here
        assert(false);
      }
    }
  }

  void process_one_exact_prefix(uint8_t depth_limit, const typename Trie::Traversal& trv, const std::string_view& prefix, re2::Regexp* rgx_tail = nullptr) {
    if (!rgx_tail) {
      matches_.add_leaf(trv, prefix);
      return;
    }

    auto ntrv = trv;
    if (!ntrv.traverse(prefix)) {
      return;
    }

    if (!ntrv.is_leaf()) {
      process_subtrie(depth_limit - 1, ntrv, rgx_tail);
      return;
    }

    auto tail = ntrv.tail();
    switch (rgx_tail->op()) {
      case re2::RegexpOp::kRegexpEmptyMatch: {
        if (tail.empty()) {
          matches_.add_leaf(ntrv);
        }
        break;
      }
      case re2::RegexpOp::kRegexpPlus:
      case re2::RegexpOp::kRegexpStar: {
        if (rgx_tail->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
          if (rgx_tail->op() == re2::RegexpOp::kRegexpStar || tail.size() > 0) {
            matches_.add_leaf(ntrv);
          }
          break;
        }

        [[fallthrough]];
      }
      default: {
        if (prepare_regexp(rgx_tail)) {
          if (prepared_prog_.full_match(tail)) {
            matches_.add_leaf(ntrv);
          }
        }
      }
    }
  }

  void process_subtrie_by_regexp(const typename Trie::Traversal& trv, re2::Regexp* rgx) {
    switch (rgx->op()) {
      case re2::RegexpOp::kRegexpPlus: {
        if (rgx->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
          matches_.add_subnodes(trv);
          return;
        }

        break;
      }
      case re2::RegexpOp::kRegexpStar: {
        if (rgx->sub()[0]->op() == re2::RegexpOp::kRegexpAnyChar) {
          matches_.add_node(trv);
          return;
        }

        break;
      }
      case re2::RegexpOp::kRegexpConcat: {
        if (rgx->sub()[rgx->nsub() - 1]->op() == re2::RegexpOp::kRegexpEndText) {
          const auto j = RegexpMatchAnalyzer::skip_end_text_operation(rgx, 0);
          const auto unanchored_rgx = re2::Regexp::Concat(rgx->sub(), j, rgx->parse_flags());
          process_subtrie_by_regexp(trv, unanchored_rgx);
          unanchored_rgx->Decref();
          return;
        }

        break;
      }
      default: {
        break;
      };
    }

    if (prepare_regexp(rgx)) {
      matches_.add_node(trv, [this](std::string_view node_tail) PROMPP_LAMBDA_INLINE { return prepared_prog_.full_match(node_tail); });
    }
  }

  PROMPP_ALWAYS_INLINE bool prepare_regexp(re2::Regexp* rgx) {
    if (prepared_for_ == rgx) {
      return true;
    }

    if (!prepared_prog_.compile(rgx)) {
      return false;
    }

    prepared_for_ = rgx;
    return true;
  }
};

}  // namespace series_index::querier