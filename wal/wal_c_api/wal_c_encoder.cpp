#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "prometheus/remote_write.h"
#include "third_party/protozero/pbf_reader.hpp"
#include "wal/wal.h"

#include "wal_c_encoder.h"

#include <limits>
#include <new>
#include <span>
#include <sstream>

namespace Wrapper {
class Hashdex {
 private:
  BareBones::Vector<PromPP::Prometheus::RemoteWrite::TimeseriesProtobufHashdexRecord> hashdex_;
  std::string_view replica_;
  std::string_view cluster_;
  const hashdex_label_set_limits limits_{};  // no limits on default.

 public:
  inline __attribute__((always_inline)) Hashdex() noexcept {}
  inline __attribute__((always_inline)) Hashdex(const hashdex_label_set_limits& limits) noexcept : limits_(limits) {}

  constexpr const hashdex_label_set_limits& get_limits() const noexcept { return limits_; }
  PromPP::Prometheus::RemoteWrite::PbLabelSetMemoryLimits get_pb_limits() const noexcept {
    return {
        limits_.max_label_name_length,
        limits_.max_label_value_length,
        limits_.max_label_names_per_timeseries,
        limits_.max_timeseries_count,
    };
  };

  // presharding - from protobuf make presharding slice with hash end proto.
  inline __attribute__((always_inline)) void presharding(c_slice proto_data, c_slice* cluster, c_slice* replica) {
    if (limits_.max_pb_size_in_bytes && proto_data.len > limits_.max_pb_size_in_bytes) {
      throw BareBones::Exception(0x1d979f3023b86c48, "Protobuf message's size (%zd) exceeds the maximum protobuf message size (%zd)", proto_data.len,
                                 limits_.max_pb_size_in_bytes);
    }

    auto pb_limits = get_pb_limits();

    protozero::pbf_reader pb(std::string_view{static_cast<const char*>(proto_data.array), proto_data.len});
    PromPP::Prometheus::RemoteWrite::read_many_timeseries_in_hashdex<PromPP::Primitives::TimeseriesSemiview,
                                                                   BareBones::Vector<PromPP::Prometheus::RemoteWrite::TimeseriesProtobufHashdexRecord>>(
        pb, hashdex_, pb_limits);
    if (!hashdex_.empty()) {
      PromPP::Primitives::TimeseriesSemiview timeseries;
      PromPP::Prometheus::RemoteWrite::read_timeseries_without_samples(protozero::pbf_reader{hashdex_.begin()->timeseries_protobuf_message}, timeseries,
                                                                     pb_limits);
      for (const auto& [name, value] : timeseries.label_set()) {
        if (name == "__replica__") {
          replica_ = value;
        }
        if (name == "cluster") {
          cluster_ = value;
        }
      }
      timeseries.clear();
      cluster->array = cluster_.begin();
      cluster->len = cluster_.size();
      cluster->cap = cluster_.size();
      replica->array = replica_.begin();
      replica->len = replica_.size();
      replica->cap = replica_.size();
    }
  };
  inline __attribute__((always_inline)) BareBones::Vector<PromPP::Prometheus::RemoteWrite::TimeseriesProtobufHashdexRecord> data() { return hashdex_; };
  inline __attribute__((always_inline)) ~Hashdex(){};
};

class Encoder {
 private:
  uint16_t shard_id_;
  uint16_t number_of_shards_;
  PromPP::Primitives::TimeseriesSemiview timeseries_;
  PromPP::WAL::Writer writer_;

 public:
  inline __attribute__((always_inline)) Encoder(uint16_t shard_id, uint16_t number_of_shards) noexcept
      : shard_id_(shard_id), number_of_shards_(number_of_shards) {}

  // encode - encoding data from Hashdex and make segment, redundant.
  inline __attribute__((always_inline)) void encode(c_hashdex c_hx, c_segment* c_seg, c_redundant* c_rt) {
    auto hashdex_data = static_cast<Hashdex*>(c_hx)->data();

    for (const auto& [chksm, pb_view] : hashdex_data) {
      if ((chksm % number_of_shards_) == shard_id_) {
        PromPP::Prometheus::RemoteWrite::read_timeseries(protozero::pbf_reader{pb_view}, timeseries_);
        writer_.add(timeseries_, chksm);
        timeseries_.clear();
      }
    }

    fill_in_segment_with_stats(c_seg);

    auto segment_buffer = new std::stringstream;
    c_rt->data = writer_.write(*segment_buffer).release();

    std::string_view outcome = segment_buffer->view();
    c_seg->data.array = outcome.begin();
    c_seg->data.len = outcome.size();
    c_seg->data.cap = outcome.size();
    c_seg->buf = segment_buffer;
  }

  // add - add to encode incoming data(ShardedData) through C++ encoder.
  inline __attribute__((always_inline)) void add(c_hashdex c_hx, c_segment* c_seg) {
    auto hashdex_data = static_cast<Hashdex*>(c_hx)->data();
    for (const auto& [chksm, pb_view] : hashdex_data) {
      if ((chksm % number_of_shards_) == shard_id_) {
        PromPP::Prometheus::RemoteWrite::read_timeseries(protozero::pbf_reader{pb_view}, timeseries_);
        writer_.add(timeseries_, chksm);
        timeseries_.clear();
      }
    }

    auto vec = BareBones::Vector<uint64_t>();
    vec.emplace_back(1);
    const auto& vec2 = vec;
    std::cout << vec2[2] << std::endl;

    fill_in_segment_with_stats(c_seg);
  }

  // finalize - finalize the encoded data in the C++ encoder to Segment.
  inline __attribute__((always_inline)) void finalize(c_segment* c_seg, c_redundant* c_rt) {
    fill_in_segment_with_stats(c_seg);

    auto segment_buffer = new std::stringstream;
    c_rt->data = writer_.write(*segment_buffer).release();

    std::string_view outcome = segment_buffer->view();
    c_seg->data.array = outcome.begin();
    c_seg->data.len = outcome.size();
    c_seg->data.cap = outcome.size();
    c_seg->buf = segment_buffer;
  }

  // snapshot - from redundants make snapshot.
  inline __attribute__((always_inline)) void snapshot(c_slice c_rts, c_snapshot* c_snap) {
    std::span<PromPP::WAL::Writer::Redundant*> span_redundants{(PromPP::WAL::Writer::Redundant**)(c_rts.array), c_rts.len};
    auto snapshot_buffer = new std::stringstream;
    writer_.snapshot(span_redundants, *snapshot_buffer);

    std::string_view outcome = snapshot_buffer->view();
    c_snap->data.array = outcome.begin();
    c_snap->data.len = outcome.size();
    c_snap->data.cap = outcome.size();
    c_snap->buf = snapshot_buffer;
  }

  inline __attribute__((always_inline)) ~Encoder() = default;

 private:
  void fill_in_segment_with_stats(c_segment* c_seg) const {
    size_t remaining_cap = std::numeric_limits<uint32_t>::max();

    c_seg->samples = writer_.buffer().samples_count();
    c_seg->series = writer_.buffer().series_count();
    c_seg->earliest_timestamp = writer_.buffer().earliest_sample();
    c_seg->latest_timestamp = writer_.buffer().latest_sample();
    c_seg->remainder_size = std::min(remaining_cap, writer_.remainder_size());
  }
};
}  // namespace Wrapper

extern "C" {
/**
 * Factory for encoder and types
 */

// Redundant
// okdb_wal_c_redundant_destroy - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_redundant_destroy)(c_redundant* c_rt) {
  delete static_cast<PromPP::WAL::BasicEncoder<>::Redundant*>(c_rt->data);
}

// Hashdex
// okdb_wal_c_hashdex_ctor - constructor, C wrapper C++, init C++ class Hashdex.
c_hashdex OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_ctor)(hashdex_label_set_limits* limits) {
  return new (std::nothrow) Wrapper::Hashdex(*limits);
}

// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_presharding)(c_hashdex c_hx, c_slice_ptr proto_data, c_slice_ptr cluster, c_slice_ptr replica) {
  return static_cast<Wrapper::Hashdex*>(c_hx)->presharding(*proto_data, cluster, replica);
}

// okdb_wal_c_hashdex_dtor - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_dtor)(c_hashdex c_hx) {
  delete static_cast<Wrapper::Hashdex*>(c_hx);
}

// Encoder
// okdb_wal_c_encoder_ctor - constructor, C wrapper C++, init C++ class Encoder.
c_encoder OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_ctor)(uint16_t shard_id, uint16_t number_of_shards) {
  return new (std::nothrow) Wrapper::Encoder(shard_id, number_of_shards);
}

// okdb_wal_c_encoder_encode - C wrapper C++, calls C++ class Encoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_encode)(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg, c_redundant* c_rt) {
  return static_cast<Wrapper::Encoder*>(c_enc)->encode(c_hx, c_seg, c_rt);
}

// okdb_wal_c_encoder_add - C wrapper C++, calls C++ class Encoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_add)(c_encoder c_enc, c_hashdex c_hx, c_segment* c_seg) {
  return static_cast<Wrapper::Encoder*>(c_enc)->add(c_hx, c_seg);
}

// okdb_wal_c_encoder_finalize - C wrapper C++, calls C++ class Encoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_finalize)(c_encoder c_enc, c_segment* c_seg, c_redundant* c_rt) {
  return static_cast<Wrapper::Encoder*>(c_enc)->finalize(c_seg, c_rt);
}

// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_snapshot)(c_encoder c_enc, c_slice* c_rts, c_snapshot* c_snap) {
  return static_cast<Wrapper::Encoder*>(c_enc)->snapshot(*c_rts, c_snap);
}

// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_dtor)(c_encoder c_enc) {
  delete static_cast<Wrapper::Encoder*>(c_enc);
}

}  // extern "C"
