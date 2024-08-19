#include "primitives_lss.h"

#include "_helpers.hpp"
#include "lss.hpp"
#include "series_index/querier/querier.h"

extern "C" void prompp_primitives_lss_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::LssType lss_type;
  };
  struct Result {
    entrypoint::LssVariantPtr lss;
  };

  new (res) Result{.lss = create_lss(reinterpret_cast<Arguments*>(args)->lss_type)};
}

extern "C" void prompp_primitives_lss_dtor(void* args) {
  struct Arguments {
    entrypoint::LssVariantPtr lss;
  };

  reinterpret_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_primitives_lss_allocated_memory(void* args, void* res) {
  struct Arguments {
    entrypoint::LssVariantPtr lss;
  };
  struct Result {
    uint64_t allocated_memory;
  };

  std::visit([res](const auto& lss) PROMPP_LAMBDA_INLINE { new (res) Result{.allocated_memory = lss.allocated_memory()}; },
             *reinterpret_cast<Arguments*>(args)->lss);
}

extern "C" void prompp_primitives_lss_find_or_emplace(void* args, void* res) {
  struct Arguments {
    entrypoint::LssVariantPtr lss;
    PromPP::Primitives::Go::LabelSet label_set;
  };
  struct Result {
    uint32_t ls_id;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  new (res) Result{.ls_id = std::visit([in](auto& lss) PROMPP_LAMBDA_INLINE { return lss.find_or_emplace(in->label_set); }, *in->lss)};
}

extern "C" void prompp_primitives_lss_query(void* args, void* res) {
  struct Arguments {
    entrypoint::LssVariantPtr lss;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::LabelMatcherTrait<PromPP::Primitives::Go::String>> label_matchers;
  };
  struct Result {
    series_index::querier::QuerierStatus status;
    PromPP::Primitives::Go::Slice<uint32_t> matches;
  };
  using Querier = series_index::querier::Querier<entrypoint::QueryableEncodingBimap, PromPP::Primitives::Go::Slice>;

  auto in = reinterpret_cast<Arguments*>(args);
  auto query_result = Querier{std::get<entrypoint::QueryableEncodingBimap>(*in->lss)}.query(in->label_matchers);

  new (res) Result{.status = query_result.status, .matches = std::move(query_result.series_ids)};
}

void prompp_primitives_lss_get_label_sets(void* args, void* res) {
  using PromPP::Primitives::Go::Label;
  using PromPP::Primitives::Go::Slice;
  using PromPP::Primitives::Go::String;

  struct Arguments {
    entrypoint::LssVariantPtr lss;
    Slice<uint32_t> series_ids;
  };
  struct Result {
    Slice<Slice<Label>> label_sets;
  };

  auto in = reinterpret_cast<Arguments*>(args);
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

  reinterpret_cast<Arguments*>(args)->label_sets.~Slice();
}
