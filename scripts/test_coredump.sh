# This script adjusts the kernel.core_pattern argument for the specified and use it as
# an parameter for the CoreDumpTest. As this test is disabled by default, we have to run
# the test runner with '--gtest_also_run_disabled_test`
# N.B. This test requires sudo for temporarily changing kernel.core_pattern (you may also disable it via --no-sudo).

usage() {
    echo "Usage:"
    echo " ${0} [options]"
    echo ""
    echo "Options:"
    echo "--check-test-fail  - set the PP_BARE_BONES_EXCEPTION_TEST_SHOULD_FAIL env variable for test and it will fail (to check the bad scenario);"
    echo "--no-build         - skip building test;"
    echo "--no-sudo          - skip changing kernel.core_pattern;"
    echo "--no-use-pid       - don't use PIDs in kernel.core_pattern (requires sudo for temporarily changing kernel.core_uses_pid to 0);"
    echo "--remove-coredump  - remove the test coredump, will echo their location;"
    echo "--help             - show this text and end work."
}

options=$(getopt -n "${0}" -o "" -l "check-test-fail,no-build,no-sudo,no-use-pid,remove-coredump,help" -- "${@}")
if [ $? -ne 0 ]; then
    usage
    exit 1
fi

AVOID_SYSCALLS=0
CHECK_TEST_FAIL=0
CUR_SH_PID=$$
BINARY_IS_GTEST=0
COREDUMP_NAME="/tmp/core_$CUR_SH_PID"
COREDUMP_USE_PID=1
OLD_COREDUMP_PATTERN=""
OLD_COREDUMP_USE_PID=0
SAVE_COREDUMP=1
SKIP_BUILD_TEST=0
TEST_TARGET_NAME=bare_bones_coredump_test
TEST_BUILD_COMMAND="bazel build --config=g++-12 $TEST_TARGET_NAME"
TEST_BINARY=./bazel-bin/$TEST_TARGET_NAME

eval set -- "${options}"
while true; do
    case "${1}" in
	--check-test-fail)
	    CHECK_TEST_FAIL=1
	    shift;;
    --no-build)
        SKIP_BUILD_TEST=1
        shift;;
	--no-sudo)
	    AVOID_SYSCALLS=1
        shift;;
    --no-use-pid)
        COREDUMP_USE_PID=0
        shift;;
	--save-coredump)
	    SAVE_COREDUMP=1
            shift;;
	--help)
	    usage
	    exit 0;;
	--) break;;
        *) echo "Unrecognized option '${1}'."; usage; exit 1;;
    esac
done

run_test() {
RUN_COMMAND="$TEST_BINARY"
if [ $BINARY_IS_GTEST -eq 1 ]; then
    RUN_COMMAND="PP_BARE_BONES_EXCEPTION_TEST_COREDUMP_USE_PID=\"$COREDUMP_USE_PID\" " \
     "PP_BARE_BONES_EXCEPTION_TEST_COREDUMP_NAME=\"$COREDUMP_NAME\" $TEST_BINARY " \
     "--gtest_filter=$TEST_NAME_FILTER --gtest_also_run_disabled_tests"
    if [ $CHECK_TEST_FAIL -eq 1 ]; then
        RUN_COMMAND="PP_BARE_BONES_EXCEPTION_TEST_SHOULD_FAIL=1 $RUN_COMMAND"
    fi
fi
echo "Run test \"${RUN_COMMAND}\"..."
eval "${RUN_COMMAND}"
}

if [ $SKIP_BUILD_TEST -eq 0 ]; then
    $TEST_BUILD_COMMAND
fi

if [ $AVOID_SYSCALLS -eq 0 ]; then
    echo "Set temporary coredump name (requires sudo password):"
    OLD_COREDUMP_PATTERN=$(sudo sysctl -n kernel.core_pattern)
    OLD_COREDUMP_USE_PID=$(sudo sysctl -n kernel.core_uses_pid)
    sudo sysctl -w kernel.core_pattern="$COREDUMP_NAME"
    sudo sysctl -w kernel.core_uses_pid="$COREDUMP_USE_PID"
fi

run_test

if [ $AVOID_SYSCALLS -eq 0 ]; then
    echo "Reset coredump name to previous value (requires sudo password):"
    sudo sysctl -w kernel.core_pattern="$OLD_COREDUMP_PATTERN"
    sudo sysctl -w kernel.core_uses_pid="$OLD_COREDUMP_USE_PID"
fi

if [ $SAVE_COREDUMP -eq 0 ]; then
    rm -f $COREDUMP_NAME
else
    if [ $COREDUMP_USE_PID -eq 1 ]; then
        echo "The result coredump may be found via 'ls $COREDUMP_NAME*'. It would have an additional extension '.<pid>'."
        echo "You may check it via 'gdb $TEST_BINARY --core=<filename>'."
    else
        echo "The result coredump located at $COREDUMP_NAME. You may check it via 'gdb $TEST_BINARY --core-$COREDUMP_NAME'."
    fi
fi
