#include "prometheus_relabeler.h"
#include "_helpers.hpp"
#include "head/lss.h"

#include "primitives/go_slice.h"
#include "prometheus/relabeler.h"

using entrypoint::head::LssVariantPtr;

//
// StatelessRelabeler
//

extern "C" void prompp_prometheus_stateless_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::GORelabelConfig*> go_rcfgs;
  };
  struct Result {
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    out->stateless_relabeler = new PromPP::Prometheus::Relabel::StatelessRelabeler(in->go_rcfgs);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_stateless_relabeler_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->stateless_relabeler;
}

extern "C" void prompp_prometheus_stateless_relabeler_reset_to(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::GORelabelConfig*> go_rcfgs;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->stateless_relabeler->reset_to(in->go_rcfgs);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

//
// InnerSeries
//

extern "C" void prompp_prometheus_inner_series_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  new (in->inner_series) PromPP::Prometheus::Relabel::InnerSeries();
}

extern "C" void prompp_prometheus_inner_series_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  in->inner_series->~InnerSeries();
}

//
// RelabeledSeries
//

extern "C" void prompp_prometheus_relabeled_series_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  new (in->relabeled_series) PromPP::Prometheus::Relabel::RelabeledSeries();
}

extern "C" void prompp_prometheus_relabeled_series_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  in->relabeled_series->~RelabeledSeries();
}

//
// RelabelerStateUpdate
//

extern "C" void prompp_prometheus_relabeler_state_update_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  new (in->relabeler_state_update) PromPP::Prometheus::Relabel::RelabelerStateUpdate();
}

extern "C" void prompp_prometheus_relabeler_state_update_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  in->relabeler_state_update->~RelabelerStateUpdate();
}

//
// PerShardRelabeler
//

extern "C" void prompp_prometheus_per_shard_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    uint16_t number_of_shards;
    uint16_t shard_id;
  };
  struct Result {
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    out->per_shard_relabeler =
        new PromPP::Prometheus::Relabel::PerShardRelabeler(in->external_labels, in->stateless_relabeler, in->number_of_shards, in->shard_id);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->per_shard_relabeler;
}

extern "C" void prompp_prometheus_per_shard_relabeler_cache_allocated_memory(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
  };
  struct Result {
    size_t allocated_memory;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  out->allocated_memory = in->per_shard_relabeler->cache_allocated_memory();
}

extern "C" void prompp_prometheus_per_shard_relabeler_input_relabeling(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> shards_relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    HashdexVariant* hashdex;
    PromPP::Prometheus::Relabel::Cache* cache;
    LssVariantPtr input_lss;
    LssVariantPtr target_lss;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& input_lss = std::get<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);
          in->per_shard_relabeler->input_relabeling(input_lss, target_lss, *in->cache, hashdex, in->options, *out, in->shards_inner_series,
                                                    in->shards_relabeled_series);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_relabel_stalenans_state_ctor(void* res) {
  struct Result {
    PromPP::Prometheus::Relabel::StaleNaNsState* state;
  };
  auto out = new (res) Result();
  out->state = new PromPP::Prometheus::Relabel::StaleNaNsState();
}

extern "C" void prompp_prometheus_relabel_stalenans_state_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::StaleNaNsState* state;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  delete in->state;
}

extern "C" void prompp_prometheus_relabel_stalenans_state_reset(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::StaleNaNsState* state;
  };
  auto in = static_cast<Arguments*>(args);
  in->state->reset();
}

extern "C" void prompp_prometheus_per_shard_relabeler_input_relabeling_with_stalenans(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> shards_relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerOptions options;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    HashdexVariant* hashdex;
    PromPP::Prometheus::Relabel::Cache* cache;
    LssVariantPtr input_lss;
    LssVariantPtr target_lss;
    PromPP::Prometheus::Relabel::StaleNaNsState* state;
    PromPP::Primitives::Timestamp def_timestamp;
  };
  struct Result {
    uint32_t samples_added{0};
    uint32_t series_added{0};
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) {
          auto& input_lss = std::get<PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap>(*in->input_lss);
          auto& target_lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->target_lss);
          in->per_shard_relabeler->input_relabeling_with_stalenans(input_lss, target_lss, *in->cache, hashdex, in->options, *out, in->shards_inner_series,
                                                                   in->shards_relabeled_series, *in->state, in->def_timestamp);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_input_collect_stalenans(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    PromPP::Prometheus::Relabel::Cache* cache;
    PromPP::Prometheus::Relabel::StaleNaNsState* state;
    PromPP::Primitives::Timestamp stale_ts;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    in->per_shard_relabeler->input_collect_stalenans(*in->cache, in->shards_inner_series, *in->state, in->stale_ts);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_append_relabeler_series(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    LssVariantPtr lss;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);
    in->per_shard_relabeler->append_relabeler_series(lss, in->inner_series, in->relabeled_series, in->relabeler_state_update);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_update_relabeler_state(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    PromPP::Prometheus::Relabel::Cache* cache;
    uint16_t relabeled_shard_id;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->per_shard_relabeler->update_relabeler_state(*in->cache, in->relabeler_state_update, in->relabeled_shard_id);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> incoming_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> encoders_inner_series;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    LssVariantPtr lss;
    PromPP::Prometheus::Relabel::Cache* cache;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    auto& lss = std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss);
    in->per_shard_relabeler->output_relabeling(lss, *in->cache, in->relabeled_series, in->incoming_inner_series, in->encoders_inner_series);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_reset_to(void* args) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    uint16_t number_of_shards;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  in->per_shard_relabeler->reset_to(in->external_labels, in->number_of_shards);
}

//
// Relabeler cache
//

extern "C" void prompp_prometheus_cache_ctor(void* res) {
  struct Result {
    PromPP::Prometheus::Relabel::Cache* cache;
  };

  Result* out = new (res) Result();

  out->cache = new PromPP::Prometheus::Relabel::Cache();
}

extern "C" void prompp_prometheus_cache_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::Cache* cache;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  delete in->cache;
}

extern "C" void prompp_prometheus_cache_allocated_memory(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::Cache* cache;
  };
  struct Result {
    size_t allocated_memory;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  out->allocated_memory = in->cache->allocated_memory();
}

extern "C" void prompp_prometheus_cache_reset_to(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::Cache* cache;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  in->cache->reset();
}
