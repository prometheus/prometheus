#include "primitives_lss.h"

#include "_helpers.hpp"
#include "head/lss.h"
#include "series_index/querier/label_names_querier.h"
#include "series_index/querier/label_values_querier.h"

using GoLabelMatchers = PromPP::Primitives::Go::SliceView<PromPP::Prometheus::LabelMatcherTrait<PromPP::Primitives::Go::String>>;
using GoSliceOfString = PromPP::Primitives::Go::Slice<PromPP::Primitives::Go::String>;
using entrypoint::head::LssType;
using entrypoint::head::LssVariantPtr;
using entrypoint::head::QueryableEncodingBimap;

extern "C" void prompp_primitives_lss_ctor(void* args, void* res) {
  struct Arguments {
    LssType lss_type;
  };
  struct Result {
    LssVariantPtr lss;
  };

  new (res) Result{.lss = create_lss(static_cast<Arguments*>(args)->lss_type)};
}

extern "C" void prompp_primitives_lss_dtor(void* args) {
  struct Arguments {
    LssVariantPtr lss;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_lss_allocated_memory(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
  };
  struct Result {
    uint64_t allocated_memory;
  };

  std::visit([res](const auto& lss) PROMPP_LAMBDA_INLINE { new (res) Result{.allocated_memory = lss.allocated_memory()}; },
             *static_cast<Arguments*>(args)->lss);
}

extern "C" void prompp_primitives_lss_find_or_emplace(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::LabelSet label_set;
  };
  struct Result {
    uint32_t ls_id;
  };

  auto in = static_cast<Arguments*>(args);
  new (res) Result{.ls_id = std::visit([in](auto& lss) PROMPP_LAMBDA_INLINE { return lss.find_or_emplace(in->label_set); }, *in->lss)};
}

extern "C" void prompp_primitives_lss_query(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    GoLabelMatchers label_matchers;
  };
  struct Result {
    uint32_t status;
    PromPP::Primitives::Go::Slice<uint32_t> matches;
  };
  using Querier = series_index::querier::Querier<QueryableEncodingBimap, PromPP::Primitives::Go::Slice>;

  const auto in = static_cast<Arguments*>(args);
  auto query_result = Querier{std::get<QueryableEncodingBimap>(*in->lss)}.query(in->label_matchers);

  new (res) Result{.status = static_cast<uint32_t>(query_result.status), .matches = std::move(query_result.series_ids)};
}

void prompp_primitives_lss_get_label_sets(void* args, void* res) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    LssVariantPtr lss;
    Slice<uint32_t> series_ids;
  };
  struct Result {
    Slice<Slice<Label>> label_sets;
  };

  auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();

  std::visit(
      [in, out](auto& lss) PROMPP_LAMBDA_INLINE {
        out->label_sets.resize(in->series_ids.size());

        for (size_t i = 0; i < in->series_ids.size(); ++i) {
          auto in_label_set = lss[in->series_ids[i]];
          auto& out_label_set = out->label_sets[i];
          out_label_set.reserve(in_label_set.size());
          std::ranges::transform(in_label_set, std::back_inserter(out_label_set),
                                 [](const auto& label) PROMPP_LAMBDA_INLINE { return Label({.name = String{label.first}, .value = String{label.second}}); });
        }
      },
      *in->lss);
}

extern "C" void prompp_primitives_lss_free_label_sets(void* args) {
  using PromPP::Primitives::Go::Slice;

  struct Arguments {
    Slice<Slice<PromPP::Primitives::Go::Label>> label_sets;
  };

  static_cast<Arguments*>(args)->label_sets.~Slice();
}

extern "C" void prompp_primitives_lss_query_label_names(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    GoLabelMatchers label_matchers;
  };
  struct Result {
    uint32_t status{};
    GoSliceOfString names;
  };

  using LabelNamesQuerier = series_index::querier::LabelNamesQuerier<QueryableEncodingBimap>;

  const auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();
  out->status = static_cast<uint32_t>(LabelNamesQuerier{std::get<QueryableEncodingBimap>(*in->lss)}.query(
      in->label_matchers, [out](std::string_view name) PROMPP_LAMBDA_INLINE { out->names.emplace_back(name); }));
}

extern "C" void prompp_primitives_lss_query_label_values(void* args, void* res) {
  struct Arguments {
    LssVariantPtr lss;
    PromPP::Primitives::Go::String label_name;
    GoLabelMatchers label_matchers;
  };
  struct Result {
    uint32_t status{};
    GoSliceOfString values;
  };

  using LabelValuesQuerier = series_index::querier::LabelValuesQuerier<QueryableEncodingBimap>;

  const auto in = static_cast<Arguments*>(args);
  auto out = new (res) Result();
  out->status = static_cast<uint32_t>(LabelValuesQuerier{std::get<QueryableEncodingBimap>(*in->lss)}.query(
      static_cast<std::string_view>(in->label_name), in->label_matchers,
      [out](std::string_view value) PROMPP_LAMBDA_INLINE { out->values.emplace_back(value); }));
}
