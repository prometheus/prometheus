workspace(name = "grpc_ecosystem_grpc_gateway")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "f04d2373bcaf8aa09bccb08a98a57e721306c8f6043a2a0ee610fd6853dcde3d",
    urls = [
        "https://storage.googleapis.com/bazel-mirror/github.com/bazelbuild/rules_go/releases/download/0.18.6/rules_go-0.18.6.tar.gz",
        "https://github.com/bazelbuild/rules_go/releases/download/0.18.6/rules_go-0.18.6.tar.gz",
    ],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "3c681998538231a2d24d0c07ed5a7658cb72bfb5fd4bf9911157c0e9ac6a2687",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.17.0/bazel-gazelle-0.17.0.tar.gz"],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

# Also define in Gopkg.toml
go_repository(
    name = "org_golang_google_genproto",
    commit = "383e8b2c3b9e36c4076b235b32537292176bae20",
    importpath = "google.golang.org/genproto",
)

# Also define in Gopkg.toml
go_repository(
    name = "com_github_rogpeppe_fastuuid",
    commit = "6724a57986aff9bff1a1770e9347036def7c89f6",
    importpath = "github.com/rogpeppe/fastuuid",
)

# Also define in Gopkg.toml
go_repository(
    name = "in_gopkg_resty_v1",
    commit = "fa5875c0caa5c260ab78acec5a244215a730247f",
    importpath = "gopkg.in/resty.v1",
)

# Also define in Gopkg.toml
go_repository(
    name = "com_github_ghodss_yaml",
    commit = "0ca9ea5df5451ffdf184b4428c902747c2c11cd7",
    importpath = "github.com/ghodss/yaml",
)

# Also define in Gopkg.toml
go_repository(
    name = "in_gopkg_yaml_v2",
    commit = "eb3733d160e74a9c7e442f435eb3bea458e1d19f",
    importpath = "gopkg.in/yaml.v2",
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "86592d703ecbe0c5cbb5139333a63268cf58d7efd2c459c8be8e69e77d135e29",
    strip_prefix = "buildtools-0.26.0",
    urls = ["https://github.com/bazelbuild/buildtools/archive/0.26.0.tar.gz"],
)

load("@com_github_bazelbuild_buildtools//buildifier:deps.bzl", "buildifier_dependencies")

buildifier_dependencies()
