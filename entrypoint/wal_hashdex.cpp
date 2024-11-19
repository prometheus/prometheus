#include "wal_hashdex.h"
#include "_helpers.hpp"

#include "primitives/go_slice.h"
#include "wal/decoder.h"
#include "wal/hashdex.h"

using PromPP::WAL::hashdex::scraper::OpenMetricsScraper;
using PromPP::WAL::hashdex::scraper::PrometheusScraper;
using ScraperError = PromPP::WAL::hashdex::scraper::Error;

extern "C" void prompp_wal_protobuf_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::HashdexLimits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::kProtobuf>, in->limits};
}

void prompp_wal_hashdex_dtor(void* args) {
  struct Arguments {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->hashdex;
}

extern "C" void prompp_wal_protobuf_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
}

extern "C" void prompp_wal_protobuf_hashdex_presharding(void* args, void* res) {
  struct Arguments {
    HashdexVariant* hashdex_variant;
    PromPP::Primitives::Go::SliceView<char> protobuf;
  };
  struct Result {
    PromPP::Primitives::Go::String cluster;
    PromPP::Primitives::Go::String replica;
    PromPP::Primitives::Go::Slice<char> error;
  };
  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();

  try {
    auto& hashdex = std::get<PromPP::WAL::ProtobufHashdex>(*in->hashdex_variant);
    hashdex.presharding(in->protobuf.data(), in->protobuf.size());
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_go_model_hashdex_ctor(void* args, void* res) {
  struct Arguments {
    PromPP::WAL::HashdexLimits limits;
  };
  struct Result {
    HashdexVariant* hashdex;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  Result* out = new (res) Result();
  out->hashdex = new HashdexVariant{std::in_place_index<HashdexType::kGoModel>, in->limits};
}

extern "C" void prompp_wal_go_model_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
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
    auto& hashdex = std::get<PromPP::WAL::GoModelHashdex>(*in->hashdex_variant);
    hashdex.presharding(in->data);
    auto cluster = hashdex.cluster();
    out->cluster.reset_to(cluster.data(), cluster.size());
    auto replica = hashdex.replica();
    out->replica.reset_to(replica.data(), replica.size());
  } catch (...) {
    auto err_stream = PromPP::Primitives::Go::BytesStream(&out->error);
    handle_current_exception(__func__, err_stream);
  }
}

extern "C" void prompp_wal_basic_decoder_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
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
  };

  const auto in = static_cast<Arguments*>(args);
  new (res) Result{.error = std::get<Scraper>(*in->hashdex).parse({const_cast<char*>(in->buffer.data()), in->buffer.size()}, in->default_timestamp)};
}

template <class Scraper>
PROMPP_ALWAYS_INLINE void scraper_hashdex_get_metadata(void* args, void* res) {
  struct Metadata {
    PromPP::Primitives::Go::String metric_name;
    PromPP::Primitives::Go::String text;
    uint32_t type;

    explicit Metadata(const typename Scraper::Metadata& metadata)
        : metric_name(metadata.metric_name()), text(metadata.text()), type(static_cast<uint32_t>(metadata.type())) {}
  };

  struct Arguments {
    HashdexVariant* hashdex;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<Metadata> metadata;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  const auto metadata = std::get<Scraper>(*in->hashdex).metadata();
  out->metadata.reserve(metadata.size());
  for (auto& m : metadata) {
    out->metadata.emplace_back(m);
  }
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

extern "C" void prompp_wal_prometheus_scraper_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
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

extern "C" void prompp_wal_open_metrics_scraper_hashdex_dtor(void* args) {
  prompp_wal_hashdex_dtor(args);
}
