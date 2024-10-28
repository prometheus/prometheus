#include "index_writer.h"

#include <memory>

#include "head/lss.h"
#include "primitives/go_slice.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"

using PromPP::Primitives::Go::SliceView;
using series_index::prometheus::tsdb::index::ChunkMetadata;
using IndexWriter = series_index::prometheus::tsdb::index::IndexWriter<PromPP::Primitives::Go::BytesStream>;
using IndexWriterPtr = std::unique_ptr<IndexWriter>;

static_assert(sizeof(IndexWriterPtr) == sizeof(void*));

static PromPP::Primitives::Go::BytesStream create_bytes_stream(PromPP::Primitives::Go::Slice<char>& buffer) {
  buffer.resize(0);
  return PromPP::Primitives::Go::BytesStream{&buffer};
}

extern "C" void prompp_index_writer_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::head::LssVariantPtr lss;
  };
  struct Result {
    IndexWriterPtr writer;
  };

  const auto in = static_cast<Arguments*>(args);
  new (res) Result{.writer = std::make_unique<IndexWriter>(std::get<entrypoint::head::QueryableEncodingBimap>(*in->lss))};
}

extern "C" void prompp_index_writer_dtor(void* args) {
  struct Arguments {
    IndexWriterPtr writer;
  };

  static_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_index_writer_write_header(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
  };

  auto stream = create_bytes_stream(static_cast<Result*>(res)->buffer);
  static_cast<Arguments*>(args)->writer->write_header(stream);
}

extern "C" void prompp_index_writer_write_symbols(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
  };

  auto stream = create_bytes_stream(static_cast<Result*>(res)->buffer);
  static_cast<Arguments*>(args)->writer->write_symbols(stream);
}

extern "C" void prompp_index_writer_write_next_series_batch(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
    SliceView<ChunkMetadata> chunk_metadata_list;
    PromPP::Primitives::LabelSetID ls_id;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  auto stream = create_bytes_stream(out->buffer);
  in->writer->write_series(in->ls_id, in->chunk_metadata_list, stream);
}

extern "C" void prompp_index_writer_write_label_indices(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
  };

  auto stream = create_bytes_stream(static_cast<Result*>(res)->buffer);
  static_cast<Arguments*>(args)->writer->write_label_indices(stream);
}

extern "C" void prompp_index_writer_write_next_postings_batch(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
    uint32_t max_batch_size;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
    bool has_more_data;
  };

  const auto in = static_cast<Arguments*>(args);
  const auto out = static_cast<Result*>(res);

  auto stream = create_bytes_stream(out->buffer);
  in->writer->write_postings(stream, in->max_batch_size);
  out->has_more_data = in->writer->has_more_postings_data();
}

extern "C" void prompp_index_writer_write_label_indices_table(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
  };

  auto stream = create_bytes_stream(static_cast<Result*>(res)->buffer);
  static_cast<Arguments*>(args)->writer->write_label_indices_table(stream);
}

extern "C" void prompp_index_writer_write_postings_table_offsets(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
  };

  auto stream = create_bytes_stream(static_cast<Result*>(res)->buffer);
  static_cast<Arguments*>(args)->writer->write_postings_table_offsets(stream);
}

extern "C" void prompp_index_writer_write_table_of_contents(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char> buffer;
  };

  auto stream = create_bytes_stream(static_cast<Result*>(res)->buffer);
  static_cast<Arguments*>(args)->writer->write_toc(stream);
}
