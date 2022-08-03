setup() {
    load '../../../../bats_helpers'

    common_setup
}

@test "debian" {
    dagger "do" -p ./test.cue test
}
