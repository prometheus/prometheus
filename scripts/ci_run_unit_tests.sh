if [ "$SANITIZERS" = "with_sanitizers" ]; then
	printf "Run unit test with sanitizers\n"
	SANITIZERS_MODE_FLAG='--asan --strip=never --platform_suffix=asan'
else
	printf "Run unit test without sanitizers\n"
	SANITIZERS_MODE_FLAG=''
fi

TEST_PACKAGES=(
  "//:*"
  "@lru_cache//:*"
)

QUERY_COMMAND=''
for TEST_PACKAGE in "${TEST_PACKAGES[@]}"; do
  if [ "${QUERY_COMMAND}" != "" ]; then
    QUERY_COMMAND+=" union "
  fi
  QUERY_COMMAND+=$(printf "tests(%s)" "${TEST_PACKAGE}")
done

bazel query "${QUERY_COMMAND}" | \
  xargs bazel test --compilation_mode="${OPT}" --local_resources=cpu=HOST_CPUS*0.2 --test_output=errors \
  --test_timeout=60 --flaky_test_attempts=5 ${SANITIZERS_MODE_FLAG}