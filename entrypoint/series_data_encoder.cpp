#include "series_data/data_storage.h"
#include "series_data_encoder.h"
#include "series_data/encoder.h"
#include "series_data/outdated_sample_encoder.h"
#include "prometheus/relabeler.h"
#include "primitives/primitives.h"

#include <chrono>

struct SeriesDataEncoderWrapper {
    std::chrono::system_clock clock{};
    series_data::OutdatedSampleEncoder<std::chrono::system_clock> outdated_sample_encoder;
    series_data::Encoder<decltype(outdated_sample_encoder)> encoder;

    explicit SeriesDataEncoderWrapper(series_data::DataStorage& data_storage) : outdated_sample_encoder{data_storage, clock}, encoder{data_storage, outdated_sample_encoder} {}
};

extern "C" void prompp_series_data_encoder_ctor(void* args, void* res) {
  struct Arguments {
    series_data::DataStorage* data_storage;
  };
  using Result = struct {
    SeriesDataEncoderWrapper* encoder_wrapper;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  new (res) Result{.encoder_wrapper = new SeriesDataEncoderWrapper(*in->data_storage)};
}

extern "C" void prompp_series_data_encoder_encode(void* args) {
  struct Arguments {
    SeriesDataEncoderWrapper* encoder_wrapper;
    uint32_t series_id;
    int64_t timestamp;
    double value;
  };

  auto* in = reinterpret_cast<Arguments*>(args);
  in->encoder_wrapper->encoder.encode(in->series_id, in->timestamp, in->value);
}

extern "C" void prompp_series_data_encoder_encode_inner_series_slice(void* args) {
  struct Arguments {
    SeriesDataEncoderWrapper* encoder_wrapper;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> inner_series_slice;
  };

  auto* in = reinterpret_cast<Arguments*>(args);

  std::ranges::for_each(in->inner_series_slice, [&](const PromPP::Prometheus::Relabel::InnerSeries* inner_series) {
    if (inner_series == nullptr || inner_series->size() == 0) {
      return;
    }

    std::ranges::for_each(inner_series->data(), [&](const PromPP::Prometheus::Relabel::InnerSerie& inner_serie) {
      std::ranges::for_each(inner_serie.samples, [&](const PromPP::Primitives::Sample& sample) {
        in->encoder_wrapper->encoder.encode(inner_serie.ls_id, sample.timestamp(), sample.value());
      });
    });
  });
}

extern "C" void prompp_series_data_encoder_dtor(void* args) {
  struct Arguments {
    SeriesDataEncoderWrapper* encoder_wrapper;
  };

  Arguments* in = reinterpret_cast<Arguments*>(args);
  delete in->encoder_wrapper;
}
