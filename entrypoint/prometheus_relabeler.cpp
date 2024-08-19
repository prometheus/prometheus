#include "prometheus_relabeler.h"
#include "_helpers.hpp"
#include "lss.hpp"

#include "primitives/go_slice.h"
#include "prometheus/relabeler.h"

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
    uint32_t generation;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  new (in->relabeler_state_update) PromPP::Prometheus::Relabel::RelabelerStateUpdate(in->generation);
}

extern "C" void prompp_prometheus_relabeler_state_update_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  in->relabeler_state_update->~RelabelerStateUpdate();
}

extern "C" void prompp_prometheus_stalenans_state_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::StaleNaNsState* source_state;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->source_state;
}

//
// PerShardRelabeler
//

extern "C" void prompp_prometheus_per_shard_relabeler_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
    uint32_t generation;
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
        new PromPP::Prometheus::Relabel::PerShardRelabeler(in->external_labels, in->stateless_relabeler, in->generation, in->number_of_shards, in->shard_id);
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
    PromPP::Prometheus::Relabel::MetricLimits* metric_limits;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    HashdexVariant* hashdex;
    entrypoint::LssVariantPtr lss;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in](auto& hashdex) PROMPP_LAMBDA_INLINE {
          std::visit(
              [in, &hashdex](auto& lss) PROMPP_LAMBDA_INLINE {
                in->per_shard_relabeler->input_relabeling(lss, in->metric_limits, hashdex, in->shards_inner_series, in->shards_relabeled_series);
              },
              *in->lss);
        },
        *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_input_relabeling_with_stalenans(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> shards_relabeled_series;
    PromPP::Prometheus::Relabel::MetricLimits* metric_limits;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    HashdexVariant* hashdex;
    entrypoint::LssVariantPtr lss;
    PromPP::Prometheus::Relabel::SourceState source_state;
    PromPP::Primitives::Timestamp stale_ts;
  };
  struct Result {
    PromPP::Prometheus::Relabel::SourceState source_state;
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in, out](auto& hashdex) PROMPP_LAMBDA_INLINE {
          std::visit(
              [in, out, &hashdex](auto& lss) PROMPP_LAMBDA_INLINE {
                out->source_state = in->per_shard_relabeler->input_relabeling_with_stalenans(lss, hashdex, in->shards_inner_series, in->shards_relabeled_series,
                                                                                             in->metric_limits, in->source_state, in->stale_ts);
              },
              *in->lss);
        },
        *in->hashdex);
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
    entrypoint::LssVariantPtr lss;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in](auto& lss)
            PROMPP_LAMBDA_INLINE { in->per_shard_relabeler->append_relabeler_series(lss, in->inner_series, in->relabeled_series, in->relabeler_state_update); },
        *in->lss);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_update_relabeler_state(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    uint16_t relabeled_shard_id;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->per_shard_relabeler->update_relabeler_state(in->relabeler_state_update, in->relabeled_shard_id);
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
    entrypoint::LssVariantPtr lss;
    uint32_t generation;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result();

  try {
    std::visit(
        [in](const auto& lss) PROMPP_LAMBDA_INLINE {
          in->per_shard_relabeler->output_relabeling(lss, in->relabeled_series, in->incoming_inner_series, in->encoders_inner_series, in->generation);
        },
        *in->lss);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_prometheus_per_shard_relabeler_reset_to(void* args) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<std::pair<PromPP::Primitives::Go::String, PromPP::Primitives::Go::String>> external_labels;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    uint32_t generation;
    uint16_t number_of_shards;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);

  in->per_shard_relabeler->reset_to(in->external_labels, in->generation, in->number_of_shards);
}
