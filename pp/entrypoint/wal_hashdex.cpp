#include "wal_hashdex.h"

#include "exception.hpp"
#include "hashdex.hpp"
#include "primitives/go_slice.h"
#include "wal/decoder.h"

using PromPP::WAL::hashdex::scraper::OpenMetricsScraper;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;
using ScraperError = PromPP::WAL::hashdex::scraper::Error;

struct GoMetadata {
  PromPP::Primitives::Go::String metric_name;
  PromPP::Primitives::Go::String text;
  uint32_t type;

  template <class Metadata>
  explicit GoMetadata(const Metadata& metadata) : metric_name(metadata.metric_name()), text(metadata.text()), type(static_cast<uint32_t>(metadata.type())) {}

  explicit GoMetadata(const PromPP::WAL::hashdex::Metadata& metadata)
      : metric_name(metadata.metric_name), text(metadata.text), type(static_cast<uint32_t>(metadata.type)) {}
};

template <class HashdexType>
void get_metadata(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<GoMetadata> metadata;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  const auto metadata = std::get<HashdexType>(*in->hashdex).metadata();
  out->metadata.reserve(metadata.size());
  for (auto& m : metadata) {
    out->metadata.emplace_back(m);
  }
}

//
// ProtobufHashdex
//

extern "C" void prompp_wal_protobuf_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::hashdex::Limits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::kProtobuf>, in->limits};
}

extern "C" void prompp_wal_hashdex_dtor(void* args) {
  struct Arguments {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->hashdex;
}

extern "C" void prompp_wal_protobuf_hashdex_snappy_presharding(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::SliceView<char> compressed_protobuf;
  };
  struct Result {
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };
  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto& hashdex = std::get<PromPP::WAL::hashdex::Protobuf>(*in->hashdex_variant);
    hashdex.snappy_presharding(static_cast<std::string_view>(in->compressed_protobuf));
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

extern "C" void prompp_wal_protobuf_hashdex_get_metadata(void* args, void* res) {
  get_metadata<PromPP::WAL::hashdex::Protobuf>(args, res);
}

//
// GoModelHashdex
//

extern "C" void prompp_wal_go_model_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::Prometheus::hashdex::Limits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::kGoModel>, in->limits};
}

extern "C" void prompp_wal_go_model_hashdex_presharding(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::SliceView<PromPP::Primitives::Go::TimeSeries> data;
  };
  struct Result {
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };
  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto& hashdex = std::get<PromPP::WAL::hashdex::GoModel>(*in->hashdex_variant);
    hashdex.presharding(in->data);
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    entrypoint::handle_current_exception(err_stream);
  }
}

template <size_t hashdex_type>
PROMPP_ALWAYS_INLINE void scraper_hashdex_ctor(void* res) {
  struct Result {
    HashdexVariant* hashdex;
  };

  new (res) Result{.hashdex = new HashdexVariant{std::in_place_index<hashdex_type>}};
}

template <class Scraper>
PROMPP_ALWAYS_INLINE void scraper_hashdex_parse(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex;
    PromPP::Primitives::Go::SliceView<char> buffer;
    PromPP::Primitives::Timestamp default_timestamp;
  };
  struct Result {
    ScraperError error{ScraperError::kNoError};
    uint32_t scraped{};
  };

  const auto in = static_cast<Arguments*>(args);
  new (res) Result{.error = std::get<Scraper>(*in->hashdex).parse({const_cast<char*>(in->buffer.data()), in->buffer.size()}, in->default_timestamp),
                   .scraped = static_cast<uint32_t>(std::get<Scraper>(*in->hashdex).size())};
}

template <class Scraper>
PROMPP_ALWAYS_INLINE void scraper_hashdex_get_metadata(void* args, void* res) {
  get_metadata<Scraper>(args, res);
}

extern "C" void prompp_wal_prometheus_scraper_hashdex_ctor(void* res) {
  scraper_hashdex_ctor<HashdexType::kPrometheusScraper>(res);
}

extern "C" void prompp_wal_prometheus_scraper_hashdex_parse(void* args, void* res) {
  scraper_hashdex_parse<PrometheusScraper>(args, res);
}

extern "C" void prompp_wal_prometheus_scraper_hashdex_get_metadata(void* args, void* res) {
  scraper_hashdex_get_metadata<PrometheusScraper>(args, res);
}

extern "C" void prompp_wal_open_metrics_scraper_hashdex_ctor(void* res) {
  scraper_hashdex_ctor<HashdexType::kOpenMetricsScraper>(res);
}

extern "C" void prompp_wal_open_metrics_scraper_hashdex_parse(void* args, void* res) {
  scraper_hashdex_parse<OpenMetricsScraper>(args, res);
}

extern "C" void prompp_wal_open_metrics_scraper_hashdex_get_metadata(void* args, void* res) {
  scraper_hashdex_get_metadata<OpenMetricsScraper>(args, res);
}
