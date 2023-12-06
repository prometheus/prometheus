#include <gtest/gtest.h>

#include "bare_bones/lz4stream/compressor.h"
#include "bare_bones/lz4stream/decompressor.h"
#include "bare_bones/lz4stream/decompressor_buffer.h"

namespace {

using BareBones::lz4stream::Compressor;
using BareBones::lz4stream::Decompressor;
using BareBones::lz4stream::DecompressorBuffer;

class StreamCompressorDecompressorFixture : public testing::Test {
 public:
  void SetUp() override {
    std::ignore = compressor_.initialize();
    std::ignore = decompressor_.initialize();
  }

 protected:
  Compressor compressor_;
  DecompressorBuffer buffer_{{.shrink_ratio = 0.3, .threshold_size_shrink_ratio = 0.5}};
  Decompressor<DecompressorBuffer> decompressor_{buffer_, {.min_allocation_size = 1, .allocation_ratio = 2.0, .reallocation_ratio = 2.0}};
};

TEST_F(StreamCompressorDecompressorFixture, UsageTest) {
  // Arrange
  std::string_view first_frame = "123";
  std::string second_frame = std::string(1024, 'a');
  std::string buffer;
  std::string_view buffer_view;

  // Act
  buffer += compressor_.compress(first_frame);
  buffer += compressor_.compress(second_frame);
  buffer_view = buffer;
  auto [decompressed1_buffer, frame1_size] = decompressor_.decompress(buffer_view);
  std::string decompressed1 = std::string(decompressed1_buffer);
  buffer_view.remove_prefix(frame1_size);
  auto [decompressed2_buffer, frame2_size] = decompressor_.decompress(buffer_view);
  buffer_view.remove_prefix(frame2_size);

  // Assert
  EXPECT_EQ(first_frame, decompressed1);
  EXPECT_EQ(second_frame, decompressed2_buffer);
  EXPECT_TRUE(buffer_view.empty());
}

}  // namespace
