setup() {
    load '../../bats_helpers'

    common_setup
}

@test "bash" {
    dagger "do" -p ./test.cue test
}

