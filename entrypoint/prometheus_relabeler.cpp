#include "prometheus_relabeler.h"
#include "_helpers.hpp"

#include "primitives/go_slice.h"
#include "prometheus/relabeler.h"

//
// StatelessRelabeler
//

/**
 * @brief Construct a new prometheus StatelessRelabeler
 *
 * @param args {
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     error               []byte    // error string if thrown;
 * }
 */
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

/**
 * @brief Destroy prometheus StatelessRelabeler
 *
 * @param args {
 *     stateless_relabeler uintptr // pointer of StatelessRelabeler;
 * }
 */
extern "C" void prompp_prometheus_stateless_relabeler_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::StatelessRelabeler* stateless_relabeler;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->stateless_relabeler;
}

/**
 * @brief reset_to reset configs and replace on new converting go-config.
 *
 * @param args {
 *     stateless_relabeler uintptr   // pointer to constructed StatelessRelabeler;
 *     cfgs                []*Config // go slice with pointer RelabelConfig;
 * }
 *
 * @param res {
 *     error               []byte    // error string if thrown;
 * }
 */
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

/**
 * @brief filling InnerSeries pointer vector InnerSerie;
 *
 * @param args {
 *     innerSeries *InnerSeries // pointer to InnerSeries;
 * }
 */
extern "C" void prompp_prometheus_inner_series_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  new (in->inner_series) PromPP::Prometheus::Relabel::InnerSeries();
}

/**
 * @brief Destroy vector with InnerSerie in InnerSeries.
 *
 * @param args {
 *     innerSeries *InnerSeries // pointer to InnerSeries;
 * }
 */
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

/**
 * @brief filling RelabeledSeries pointer vector RelabeledSerie;
 *
 * @param args {
 *     relabeledSeries *RelabeledSeries // pointer to RelabeledSeries;
 * }
 */
extern "C" void prompp_prometheus_relabeled_series_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  new (in->relabeled_series) PromPP::Prometheus::Relabel::RelabeledSeries();
}

/**
 * @brief Destroy vector with RelabeledSerie in RelabeledSeries.
 *
 * @param args {
 *     relabeledSeries *RelabeledSeries // pointer to RelabeledSeries;
 * }
 */
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

/**
 * @brief init RelabelerStateUpdate(pointer to RelabelerStateUpdate).
 *
 * @param res {
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 *     generation             uint32                // current generation;
 * }
 */
extern "C" void prompp_prometheus_relabeler_state_update_ctor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    uint32_t generation;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  new (in->relabeler_state_update) PromPP::Prometheus::Relabel::RelabelerStateUpdate(in->generation);
}

/**
 * @brief Destroy vector in RelabelerStateUpdate.
 *
 * @param args {
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 * }
 */
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

/**
 * @brief Construct a new PerShardRelabeler
 *
 * @param args {
 *     external_labels     []Label // slice with external lables(pair string);
 *     stateless_relabeler uintptr // pointer to constructed stateless relabeler;
 *     generation          uint32  // generation of lss;
 *     number_of_shards    uint16  // total shards count;
 *     shard_id            uint16  // current shard id;
 * }
 *
 * @param res {
 *     per_shard_relabeler uintptr // pointer to constructed PerShardRelabeler;
 *     error               []byte  // error string if thrown;
 * }
 */
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

/**
 * @brief Destroy prometheus PerShardRelabeler
 *
 * @param args {
 *     per_shard_relabeler uintptr // pointer of PerShardRelabeler;
 * }
 */
extern "C" void prompp_prometheus_per_shard_relabeler_dtor(void* args) {
  struct Arguments {
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->per_shard_relabeler;
}

/**
 * @brief return size of allocated memory for cache map.
 *
 * @param args {
 *     per_shard_relabeler uintptr // pointer to constructed per shard relabeler;
 * }
 *
 * @param res {
 *     allocated_memory    uint64  // size of allocated memory for label sets;
 * }
 */
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

/**
 * @brief relabeling incomig hashdex(first stage).
 *
 * @param args {
 *     shards_inner_series     []*InnerSeries     // go slice with output InnerSeries;
 *     shards_relabeled_series []*RelabeledSeries // go slice with output RelabeledSeries;
 *     label_limits            *LabelLimits       // pointer to LabelLimits;
 *     per_shard_relabeler     uintptr            // pointer to constructed per shard relabeler;
 *     hashdex                 uintptr            // pointer to filled hashdex;
 *     lss                     uintptr            // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
extern "C" void prompp_prometheus_per_shard_relabeler_input_relabeling(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> shards_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::RelabeledSeries*> shards_relabeled_series;
    PromPP::Prometheus::Relabel::LabelLimits* label_limits;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    HashdexVariant* hashdex;
    PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap* lss;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto lmb = [in](auto& hashdex) __attribute__((always_inline)) {
      in->per_shard_relabeler->input_relabeling(in->lss, in->label_limits, hashdex, in->shards_inner_series, in->shards_relabeled_series);
    };
    std::visit(lmb, *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

/**
 * @brief add relabeled ls to lss, add to result and add to cache update(second stage).
 *
 * @param args {
 *     inner_series           *InnerSeries          // go InnerSeries per shard;
 *     relabeled_series       *RelabeledSeries      // go RelabeledSeries per shard;
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 *     per_shard_relabeler    uintptr               // pointer to constructed per shard relabeler;
 *     lss                    uintptr               // pointer to constructed label sets;
 * }
 *
 * @param res {
 *     error                  []byte           // error string if thrown
 * }
 */
extern "C" void prompp_prometheus_per_shard_relabeler_append_relabeler_series(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::InnerSeries* inner_series;
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap* lss;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->per_shard_relabeler->append_relabeler_series(in->lss, in->inner_series, in->relabeled_series, in->relabeler_state_update);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

/**
 * @brief add to cache relabled data(third stage).
 *
 * @param args {
 *     relabeler_state_update *RelabelerStateUpdate // pointer to RelabelerStateUpdate;
 *     per_shard_relabeler    uintptr               // pointer to constructed per shard relabeler;
 *     relabeled_shard_id     uint16                // relabeled shard id;
 * }
 *
 * @param res {
 *     error                  []byte  // error string if thrown;
 * }
 */
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

/**
 * @brief relabeling output series(fourth stage).
 *
 * @param args {
 *     incoming_inner_series     []*InnerSeries     // go slice with incoming InnerSeries;
 *     encoders_inner_series     []*InnerSeries     // go slice with output InnerSeries;
 *     shards_relabeled_series   []*RelabeledSeries // go slice with output RelabeledSeries;
 *     per_shard_relabeler       uintptr            // pointer to constructed per shard relabeler;
 *     lss                       uintptr            // pointer to constructed label sets;
 *     generation                uint32             // current encoders generation;
 * }
 *
 * @param res {
 *     error                   []byte             // error string if thrown;
 * }
 */
extern "C" void prompp_prometheus_per_shard_relabeler_output_relabeling(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabeledSeries* relabeled_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> incoming_inner_series;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> encoders_inner_series;
    PromPP::Prometheus::Relabel::PerShardRelabeler* per_shard_relabeler;
    PromPP::Primitives::SnugComposites::LabelSet::EncodingBimap* lss;
    uint32_t generation;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->per_shard_relabeler->output_relabeling(in->lss, in->relabeled_series, in->incoming_inner_series, in->encoders_inner_series, in->generation);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

/**
 * @brief reset cache and store lss generation.
 *
 * @param args {
 *     external_labels     []Label // slice with external lables(pair string);
 *     per_shard_relabeler uintptr // pointer to constructed per shard relabeler;
 *     generation          uint32  // generation of lss;
 *     number_of_shards    uint16  // total shards count;
 * }
 */
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
