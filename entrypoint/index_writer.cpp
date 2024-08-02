#include "index_writer.h"

#include <memory>

#include "lss.hpp"
#include "primitives/go_slice.h"
#include "series_index/prometheus/tsdb/index/index_writer.h"

using PromPP::Primitives::Go::SliceView;
using series_index::prometheus::tsdb::index::ChunkMetadata;
using ChunkMetadataList = SliceView<SliceView<ChunkMetadata>>;
using IndexWriter = series_index::prometheus::tsdb::index::IndexWriter<ChunkMetadataList>;
using IndexWriterPtr = std::unique_ptr<IndexWriter>;

static_assert(sizeof(IndexWriterPtr) == sizeof(void*));

static PromPP::Primitives::Go::BytesStream create_bytes_stream(PromPP::Primitives::Go::Slice<char>& buffer) {
  buffer.resize(0);
  return PromPP::Primitives::Go::BytesStream{&buffer};
}

extern "C" void prompp_index_writer_ctor(void* args, void* res) {
  struct Arguments {
    entrypoint::LssVariantPtr lss;
    const ChunkMetadataList* chunk_metadata_list;
  };
  struct Result {
    IndexWriterPtr writer;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = new (res) Result;
  if (auto lss = std::get_if<entrypoint::QueryableEncodingBimap>(in->lss.get()); lss) {
    if (lss->size() == 0) {
      lss->find_or_emplace(PromPP::Primitives::LabelViewSet{{"job", "cron"}, {"server", "localhost"}, {"process", "php"}});
      lss->find_or_emplace(PromPP::Primitives::LabelViewSet{{"job", "cro1"}, {"server", "127.0.0.1"}, {"process", "nodejs"}});
      lss->find_or_emplace(PromPP::Primitives::LabelViewSet{{"joa", "cron"}, {"server", "127.0.0.1"}, {"process", "nodejs"}});
    }

    out->writer = std::make_unique<IndexWriter>(*lss, *in->chunk_metadata_list);
  }
}

extern "C" void prompp_index_writer_dtor(void* args) {
  struct Arguments {
    IndexWriterPtr writer;
  };

  reinterpret_cast<Arguments*>(args)->~Arguments();
}

extern "C" void prompp_index_writer_write_header(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto stream = create_bytes_stream(*reinterpret_cast<Result*>(res)->buffer);
  reinterpret_cast<Arguments*>(args)->writer->write_header(stream);
}

extern "C" void prompp_index_writer_write_symbols(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto stream = create_bytes_stream(*reinterpret_cast<Result*>(res)->buffer);
  reinterpret_cast<Arguments*>(args)->writer->write_symbols(stream);
}

extern "C" void prompp_index_writer_write_next_series_batch(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
    uint32_t batch_size;
  };
  struct Result {
    bool has_more_data;
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = reinterpret_cast<Result*>(res);

  auto stream = create_bytes_stream(*out->buffer);
  in->writer->write_series(stream, in->batch_size);
  out->has_more_data = in->writer->has_more_series_data();
}

extern "C" void prompp_index_writer_write_label_indices(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto stream = create_bytes_stream(*reinterpret_cast<Result*>(res)->buffer);
  reinterpret_cast<Arguments*>(args)->writer->write_label_indices(stream);
}

extern "C" void prompp_index_writer_write_next_postings_batch(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
    uint32_t max_batch_size;
  };
  struct Result {
    bool has_more_data;
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto in = reinterpret_cast<Arguments*>(args);
  auto out = reinterpret_cast<Result*>(res);

  auto stream = create_bytes_stream(*out->buffer);
  in->writer->write_postings(stream, in->max_batch_size);
  out->has_more_data = in->writer->has_more_postings_data();
}

extern "C" void prompp_index_writer_write_label_indices_table(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto stream = create_bytes_stream(*reinterpret_cast<Result*>(res)->buffer);
  reinterpret_cast<Arguments*>(args)->writer->write_label_indices_table(stream);
}

extern "C" void prompp_index_writer_write_postings_table_offsets(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto stream = create_bytes_stream(*reinterpret_cast<Result*>(res)->buffer);
  reinterpret_cast<Arguments*>(args)->writer->write_postings_table_offsets(stream);
}

extern "C" void prompp_index_writer_write_table_of_contents(void* args, void* res) {
  struct Arguments {
    IndexWriterPtr writer;
  };
  struct Result {
    PromPP::Primitives::Go::Slice<char>* buffer;
  };

  auto stream = create_bytes_stream(*reinterpret_cast<Result*>(res)->buffer);
  reinterpret_cast<Arguments*>(args)->writer->write_toc(stream);
}
