#include <variant>

#include "_helpers.hpp"
#include "wal_encoder.h"

#include "primitives/go_slice.h"
#include "wal/encoder.h"
#include "wal/hashdex.h"
#include "wal/wal.h"

extern "C" void prompp_wal_encoders_version(void* res) {
  struct Result {
    uint8_t encoders_version;
  };

  Result* out = new (res) Result();
  out->encoders_version = static_cast<uint8_t>(PromPP::WAL::Writer::version);
}

extern "C" void prompp_wal_encoder_ctor(void* args, void* res) {
  struct Arguments {
    uint16_t shard_id;
    uint8_t log_shards;
  };
  struct Result {
    PromPP::WAL::Encoder* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->encoder = new PromPP::WAL::Encoder(in->shard_id, in->log_shards);
}

extern "C" void prompp_wal_encoder_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->encoder;
}

extern "C" void prompp_wal_encoder_add(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
    HashdexVariant* hashdex;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto lmb = [in, out](auto& hashdex) __attribute__((always_inline)) { in->encoder->add(hashdex, out); };
    std::visit(lmb, *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_add_inner_series(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> incoming_inner_series;
    PromPP::WAL::Encoder* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->add_inner_series(in->incoming_inner_series, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_add_relabeled_series(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PromPP::Prometheus::Relabel::RelabeledSeries* incoming_relabeled_series;
    PromPP::WAL::Encoder* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->add_relabeled_series(in->incoming_relabeled_series, in->relabeler_state_update, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_add_with_stale_nans(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
    HashdexVariant* hashdex;
    int64_t stale_ts;
    PromPP::WAL::Writer::SourceState source_state;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::WAL::Writer::SourceState source_state;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto lmb = [in, out](auto& hashdex) __attribute__((always_inline)) { in->encoder->add(hashdex, out); };
    std::visit(lmb, *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_collect_source(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
    int64_t stale_ts;
    PromPP::WAL::Writer::SourceState source_state;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->collect_source(out, in->stale_ts, in->source_state);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_encoder_finalize(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::Encoder* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> segment;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  auto out_stream = PromPP::Primitives::Go::BytesStream(&out->segment);

  try {
    in->encoder->finalize(out, out_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

//
// EncoderLightweight
//

/**
 * @brief Construct a new WAL EncoderLightweight
 *
 * @param args {
 *     shardID            uint16  // shard number
 *     logShards          uint8   // logarithm to the base 2 of total shards count
 * }
 * @param res {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
extern "C" void prompp_wal_encoder_lightweight_ctor(void* args, void* res) {
  struct Arguments {
    uint16_t shard_id;
    uint8_t log_shards;
  };
  struct Result {
    PromPP::WAL::EncoderLightweight* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->encoder = new PromPP::WAL::EncoderLightweight(in->shard_id, in->log_shards);
}

/**
 * @brief Destroy EncoderLightweight
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 */
extern "C" void prompp_wal_encoder_lightweight_dtor(void* args) {
  struct Arguments {
    PromPP::WAL::EncoderLightweight* encoder;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->encoder;
}

extern "C" void prompp_wal_encoder_lightweight_add(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::EncoderLightweight* encoder;
    HashdexVariant* hashdex;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto lmb = [in, out](auto& hashdex) __attribute__((always_inline)) { in->encoder->add(hashdex, out); };
    std::visit(lmb, *in->hashdex);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

/**
 * @brief Add inner series to current segment
 *
 * @param args {
 *     incomingInnerSeries []*InnerSeries // go slice with incoming InnerSeries;
 *     encoderLightweight  uintptr        // pointer to constructed encoder;
 * }
 * @param res {
 *     earliestTimestamp   int64          // minimal sample timestamp in segment
 *     latestTimestamp     int64          // maximal sample timestamp in segment
 *     allocatedMemory     uint64         // size of allocated memory for label sets;
 *     samples             uint32         // number of samples in segment
 *     series              uint32         // number of series in segment
 *     remainderSize       uint32         // rest of internal buffers capacity
 *     error               []byte         // error string if thrown
 * }
 */
extern "C" void prompp_wal_encoder_lightweight_add_inner_series(void* args, void* res) {
  struct Arguments {
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> incoming_inner_series;
    PromPP::WAL::EncoderLightweight* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->add_inner_series(in->incoming_inner_series, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

/**
 * @brief Add relabeled series to current segment
 *
 * @param args {
 *     incomingRelabeledSeries []*RelabeledSeries // go slice with incoming RelabeledSeries;
 *     encoderLightweight      uintptr            // pointer to constructed encoder
 *     relabelerStateUpdate    uintptr            // pointer to constructed RelabelerStateUpdate;
 * }
 * @param res {
 *     earliestTimestamp       int64              // minimal sample timestamp in segment
 *     latestTimestamp         int64              // maximal sample timestamp in segment
 *     allocatedMemory         uint64             // size of allocated memory for label sets;
 *     samples                 uint32             // number of samples in segment
 *     series                  uint32             // number of series in segment
 *     remainderSize           uint32             // rest of internal buffers capacity
 *     error                   []byte             // error string if thrown
 * }
 */
extern "C" void prompp_wal_encoder_lightweight_add_relabeled_series(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::Relabel::RelabelerStateUpdate* relabeler_state_update;
    PromPP::Prometheus::Relabel::RelabeledSeries* incoming_relabeled_series;
    PromPP::WAL::EncoderLightweight* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    in->encoder->add_relabeled_series(in->incoming_relabeled_series, in->relabeler_state_update, out);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

/**
 * @brief Flush segment
 *
 * @param args {
 *     encoderLightweight uintptr // pointer to constructed encoder
 * }
 * @param res {
 *     earliestTimestamp  int64   // minimal sample timestamp in segment
 *     latestTimestamp    int64   // maximal sample timestamp in segment
 *     allocatedMemory    uint64  // size of allocated memory for label sets;
 *     samples            uint32  // number of samples in segment
 *     series             uint32  // number of series in segment
 *     remainderSize      uint32  // rest of internal buffers capacity
 *     error              []byte  // error string if thrown
 * }
 */
extern "C" void prompp_wal_encoder_lightweight_finalize(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::EncoderLightweight* encoder;
  };
  struct Result {
    int64_t earliest_timestamp;
    int64_t latest_timestamp;
    size_t allocated_memory;
    uint32_t samples;
    uint32_t series;
    uint32_t remainder_size;
    PromPP::Primitives::Go::Slice<char> segment;
    PromPP::Primitives::Go::Slice<char> error;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  auto out_stream = PromPP::Primitives::Go::BytesStream(&out->segment);

  try {
    in->encoder->finalize(out, out_stream);
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}
