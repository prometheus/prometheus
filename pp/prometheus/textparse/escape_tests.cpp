#include <gtest/gtest.h>

#include "escape.h"

namespace {

using PromPP::Prometheus::textparse::unescape_label_value;

struct UnescapeLabelValueCase {
  std::string_view label_value;
  std::vector<std::string_view> pieces;
};

class UnescapeLabelValueFixture : public ::testing::TestWithParam<UnescapeLabelValueCase> {
 protected:
  [[nodiscard]] static std::vector<std::string_view> unescape(std::string_view label_value) {
    std::vector<std::string_view> result;
    unescape_label_value(label_value, [&result](const std::string_view& piece) PROMPP_LAMBDA_INLINE { result.emplace_back(piece); });
    return result;
  }
};

TEST_P(UnescapeLabelValueFixture, Test) {
  // Arrange

  // Act
  const auto result = unescape(GetParam().label_value);

  // Assert
  EXPECT_EQ(GetParam().pieces, result);
}

INSTANTIATE_TEST_SUITE_P(NoReplaces,
                         UnescapeLabelValueFixture,
                         testing::Values(UnescapeLabelValueCase{.label_value = "", .pieces = {}},
                                         UnescapeLabelValueCase{.label_value = "12345", .pieces = {"12345"}},
                                         UnescapeLabelValueCase{.label_value = R"(\a)", .pieces = {R"(\a)"}},
                                         UnescapeLabelValueCase{.label_value = R"(\')", .pieces = {R"(\')"}}));

INSTANTIATE_TEST_SUITE_P(ReplaceQuote,
                         UnescapeLabelValueFixture,
                         testing::Values(UnescapeLabelValueCase{.label_value = R"(\")", .pieces = {"\""}},
                                         UnescapeLabelValueCase{.label_value = R"(a\")", .pieces = {"a", R"(")"}},
                                         UnescapeLabelValueCase{.label_value = R"(\"b)", .pieces = {R"("b)"}},
                                         UnescapeLabelValueCase{.label_value = R"(a\"b)", .pieces = {"a", R"("b)"}}));

INSTANTIATE_TEST_SUITE_P(ReplaceBackslash,
                         UnescapeLabelValueFixture,
                         testing::Values(UnescapeLabelValueCase{.label_value = R"(\\)", .pieces = {R"(\)"}},
                                         UnescapeLabelValueCase{.label_value = R"(a\\)", .pieces = {"a", R"(\)"}},
                                         UnescapeLabelValueCase{.label_value = R"(\\b)", .pieces = {R"(\b)"}},
                                         UnescapeLabelValueCase{.label_value = R"(a\\b)", .pieces = {"a", R"(\b)"}}));

INSTANTIATE_TEST_SUITE_P(ReplaceNewline,
                         UnescapeLabelValueFixture,
                         testing::Values(UnescapeLabelValueCase{.label_value = R"(\n)", .pieces = {"\n"}},
                                         UnescapeLabelValueCase{.label_value = R"(a\n)", .pieces = {"a", "\n"}},
                                         UnescapeLabelValueCase{.label_value = R"(\nb)", .pieces = {"\n", "b"}},
                                         UnescapeLabelValueCase{.label_value = R"(a\nb)", .pieces = {"a", "\n", "b"}},
                                         UnescapeLabelValueCase{.label_value = R"(\n\n\n)", .pieces = {"\n", "\n", "\n"}}));

INSTANTIATE_TEST_SUITE_P(CornerCases,
                         UnescapeLabelValueFixture,
                         testing::Values(UnescapeLabelValueCase{.label_value = R"(\"\n\\)", .pieces = {"\"", "\n", "\\"}},
                                         UnescapeLabelValueCase{.label_value = R"(\"\\n)", .pieces = {"\"", R"(\n)"}},
                                         UnescapeLabelValueCase{.label_value = R"(\)", .pieces = {R"(\)"}}));

}  // namespace