setup() {
    load '../../bats_helpers'

    common_setup
}

@test "python" {
    dagger "do" -p ./test.cue test
}
