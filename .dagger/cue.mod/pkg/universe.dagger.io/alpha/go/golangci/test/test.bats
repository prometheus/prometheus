setup() {
    load '../../../../bats_helpers'

    common_setup
}

@test "golangci-lint lint" {
    dagger "do" -p ./lint.cue test
}
