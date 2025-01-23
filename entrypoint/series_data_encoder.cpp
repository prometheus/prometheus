#include "series_data_encoder.h"

#include "head/series_data.h"
#include "primitives/primitives.h"
#include "prometheus/relabeler.h"
#include "series_data/data_storage.h"

extern "C" void prompp_series_data_encoder_ctor(void* args, void* res) {
  struct Arguments {
    series_data::DataStorage* data_storage;
  };
  using Result = struct {
    entrypoint::head::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  new (res) Result{.encoder_wrapper = std::make_unique<entrypoint::head::SeriesDataEncoderWrapper>(*static_cast<Arguments*>(args)->data_storage)};
}

extern "C" void prompp_series_data_encoder_encode(void* args) {
  struct Arguments {
    entrypoint::head::SeriesDataEncoderWrapperPtr encoder_wrapper;
    uint32_t series_id;
    int64_t timestamp;
    double value;
  };

  const auto* in = static_cast<Arguments*>(args);
  in->encoder_wrapper->encoder.encode(in->series_id, in->timestamp, in->value);
}

extern "C" void prompp_series_data_encoder_encode_inner_series_slice(void* args) {
  struct Arguments {
    entrypoint::head::SeriesDataEncoderWrapperPtr encoder_wrapper;
    PromPP::Primitives::Go::SliceView<PromPP::Prometheus::Relabel::InnerSeries*> inner_series_slice;
  };

  auto* in = static_cast<Arguments*>(args);

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

extern "C" void prompp_series_data_encoder_merge_out_of_order_chunks(void* args) {
  struct Arguments {
    entrypoint::head::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  entrypoint::head::OutdatedChunkMerger{static_cast<Arguments*>(args)->encoder_wrapper->encoder}.merge();
}

extern "C" void prompp_series_data_encoder_dtor(void* args) {
  struct Arguments {
    entrypoint::head::SeriesDataEncoderWrapperPtr encoder_wrapper;
  };

  static_cast<Arguments*>(args)->~Arguments();
}
