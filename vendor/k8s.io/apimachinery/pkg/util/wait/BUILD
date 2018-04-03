package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = ["wait_test.go"],
    embed = [":go_default_library"],
    deps = ["//vendor/k8s.io/apimachinery/pkg/util/runtime:go_default_library"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "wait.go",
    ],
    importpath = "k8s.io/apimachinery/pkg/util/wait",
    deps = ["//vendor/k8s.io/apimachinery/pkg/util/runtime:go_default_library"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
)
