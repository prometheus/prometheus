#include "prometheus/remote_write.h"
#include "wal/wal.h"

#include "wal_c_decoder.h"

#include <new>
#include <sstream>

#if __has_include(<spanstream>)  // sanity checks..
#if __cplusplus <= 202002L
#error "Please set -std="c++2b" or similar flag for C++23 for your compiler."
#endif
#include <spanstream>
#else
#error "Your C++ Standard library doesn't implement the std::spanstream. Make sure that you use conformant Library (e.g., libstdc++ from GCC 12)"
#endif

namespace Wrapper {
class Decoder {
 private:
  PromPP::WAL::Reader reader_;

 public:
  inline __attribute__((always_inline)) Decoder() noexcept {}

  // decode - decoding incoming data and make protbuf.
  inline __attribute__((always_inline)) uint32_t decode(c_slice c_seg, c_decoded_segment* c_protobuf) {
    std::ispanstream income_buffer({(char*)(c_seg.array), c_seg.len});
    income_buffer >> reader_;

    auto protobuf_buffer = new std::string;
    protozero::pbf_writer pb_message(*protobuf_buffer);
    reader_.process_segment([&](PromPP::WAL::Reader::timeseries_type timeseries) { PromPP::Prometheus::RemoteWrite::write_timeseries(pb_message, timeseries); });

    c_protobuf->data.array = protobuf_buffer->c_str();
    c_protobuf->data.len = protobuf_buffer->size();
    c_protobuf->data.cap = protobuf_buffer->size();
    c_protobuf->buf = protobuf_buffer;
    c_protobuf->created_at = reader_.created_at_tsns();
    c_protobuf->encoded_at = reader_.encoded_at_tsns();

    return reader_.last_processed_segment();
  }

  // decode_dry - decoding incoming data without protbuf.
  inline __attribute__((always_inline)) uint32_t decode_dry(c_slice c_seg) {
    std::ispanstream income_buffer({(char*)(c_seg.array), c_seg.len});
    income_buffer >> reader_;
    reader_.process_segment([](uint32_t ls_id, uint64_t ts, double v) {});

    return reader_.last_processed_segment();
  }

  // snapshot - restore reader from snapshot.
  inline __attribute__((always_inline)) void snapshot(c_slice c_snap) {
    std::ispanstream income_buffer({(char*)(c_snap.array), c_snap.len});
    reader_.load_snapshot(income_buffer);
  }

  inline __attribute__((always_inline)) ~Decoder(){};
};
};  // namespace Wrapper

extern "C" {
/**
 * Factory for decoder
 */

// Decoder
// okdb_wal_c_decoder_ctor - constructor, C wrapper C++, init C++ class Decoder.
c_decoder OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_ctor)() {
  return new (std::nothrow) Wrapper::Decoder();
}

// okdb_wal_c_decoder_decode - C wrapper C++, calls C++ class Decoder methods.
uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_decode)(c_decoder c_dec, c_slice c_seg, c_decoded_segment* c_protobuf) {
  return static_cast<Wrapper::Decoder*>(c_dec)->decode(c_seg, c_protobuf);
}

// okdb_wal_c_decoder_decode_dry - C wrapper C++, calls C++ class Decoder methods.
uint32_t OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_decode_dry)(c_decoder c_dec, c_slice c_seg) {
  return static_cast<Wrapper::Decoder*>(c_dec)->decode_dry(c_seg);
}

// okdb_wal_c_decoder_snapshot - C wrapper C++, calls C++ class Decoder methods.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_snapshot)(c_decoder c_dec, c_slice c_snap) {
  return static_cast<Wrapper::Decoder*>(c_dec)->snapshot(c_snap);
}

// okdb_wal_c_decoder_dtor - calls the destructor, C wrapper C++ for clear memory.
void OKDB_WAL_PREFIXED_NAME(okdb_wal_c_decoder_dtor)(c_decoder c_dec) {
  delete static_cast<Wrapper::Decoder*>(c_dec);
}

}  // extern "C"
