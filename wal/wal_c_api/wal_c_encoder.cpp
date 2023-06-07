#include "bare_bones/vector.h"
#include "primitives/primitives.h"
#include "prometheus/remote_write.h"
#include "third_party/protozero/pbf_reader.hpp"
#include "wal/wal.h"

#include "wal_c_encoder.h"

#include <new>
#include <span>
#include <sstream>

namespace Wrapper {
class Hashdex {
 private:
  BareBones::Vector<PromPP::Prometheus::RemoteWrite::TimeseriesProtobufHashdexRecord> hashdex_;

 public:
  inline __attribute__((always_inline)) Hashdex() noexcept {}

  // presharding - from protobuf make presharding slice with hash end proto.
  inline __attribute__((always_inline)) void presharding(c_slice proto_data) {
    protozero::pbf_reader pb(std::string_view{static_cast<const char*>(proto_data.array), proto_data.len});
    PromPP::Prometheus::RemoteWrite::read_many_timeseries_in_hashdex<PromPP::Primitives::TimeseriesSemiview,
                                                                   BareBones::Vector<PromPP::Prometheus::RemoteWrite::TimeseriesProtobufHashdexRecord>>(pb,
                                                                                                                                                      hashdex_);
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
  inline __attribute__((always_inline)) void encode(c_hashdex c_hx, c_slice_with_stream_buffer* c_seg, c_redundant* c_rt) {
    auto hashdex_data = static_cast<Hashdex*>(c_hx)->data();
    for (const auto& [chksm, pb_view] : hashdex_data) {
      if ((chksm % number_of_shards_) == shard_id_) {
        PromPP::Prometheus::RemoteWrite::read_timeseries(protozero::pbf_reader{pb_view}, timeseries_);
        writer_.add(timeseries_, chksm);
        timeseries_.clear();
      }
    }

    auto segment_buffer = new std::stringstream;
    c_rt->data = writer_.write(*segment_buffer);

    std::string_view outcome = segment_buffer->view();
    c_seg->data.array = outcome.begin();
    c_seg->data.len = outcome.size();
    c_seg->data.cap = outcome.size();
    c_seg->buf = segment_buffer;
  }

  // snapshot - from redundants make snapshot.
  inline __attribute__((always_inline)) void snapshot(c_slice c_rts, c_slice_with_stream_buffer* c_snap) {
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
c_hashdex OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_ctor)() {
  return new (std::nothrow) Wrapper::Hashdex();
}

// okdb_wal_c_hashdex_presharding - C wrapper C++, calls C++ class Hashdex methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_hashdex_presharding)(c_hashdex c_hx, c_slice proto_data) {
  return static_cast<Wrapper::Hashdex*>(c_hx)->presharding(proto_data);
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
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_encode)(c_encoder c_enc, c_hashdex c_hx, c_slice_with_stream_buffer* c_seg, c_redundant* c_rt) {
  return static_cast<Wrapper::Encoder*>(c_enc)->encode(c_hx, c_seg, c_rt);
}

// okdb_wal_c_encoder_snapshot - C wrapper C++, calls C++ class Encoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_snapshot)(c_encoder c_enc, c_slice c_rts, c_slice_with_stream_buffer* c_snap) {
  return static_cast<Wrapper::Encoder*>(c_enc)->snapshot(c_rts, c_snap);
}

// okdb_wal_c_encoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_encoder_dtor)(c_encoder c_enc) {
  delete static_cast<Wrapper::Encoder*>(c_enc);
}

}  // extern "C"
